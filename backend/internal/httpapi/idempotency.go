package httpapi

import (
	"bytes"
	"io"
	"net/http"
	"time"

	"github.com/iSundram/codeceremony/backend/internal/auth"
	"github.com/iSundram/codeceremony/backend/internal/store"
)

// IdempotencyHeader is the request header a client uses to make a retry safe.
const IdempotencyHeader = "Idempotency-Key"

// idempotent makes a mutating request safe to retry.
//
// A client on a flaky connection cannot tell whether a write that returned a
// dropped connection was applied. Retrying may then double it: a second ballot,
// a second submission version, a second round of mail to every judge. The key
// lets the server recognise the retry and answer with what the first attempt
// answered, instead of applying it again.
//
// The key is optional. A request without one behaves exactly as before, because
// requiring it would break every existing client to protect against a problem
// most requests do not have.
func (s *Server) idempotent(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := sanitizeIdempotencyKey(r.Header.Get(IdempotencyHeader))
		if !isIdempotencyEligible(r.Method, key) {
			next(w, r)
			return
		}

		// The body is read here and restored, because the fingerprint needs it and
		// the handler still has to see it. A body is read at most once per
		// request, so this has to be the read.
		body, err := readBody(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", "the request body could not be read")
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))

		principal, _ := auth.PrincipalFromContext(r.Context())
		// If-Match is part of the request's identity. It was left out, and a 412
		// is below 500 so it was memoised as the outcome: a client that did what
		// the 412 told it to do — re-read, take the ETag the 412 carried, retry —
		// got the same 412 back with Idempotency-Replayed on it, forever. The
		// only escape was a different key.
		fingerprint := store.FingerprintRequest(r.Method, r.URL.Path, body, r.Header.Get("If-Match"))

		outcome, record, done := s.store.BeginIdempotent(principal.UserID, key, fingerprint)
		switch outcome {
		case store.IdempotencyReplay:
			// Answer with the original response, byte for byte. A replay that
			// re-ran the handler would defeat the mechanism.
			writeIdempotencyHeaders(w, key, true)
			contentType := record.ContentType
			if contentType == "" {
				contentType = "application/json"
			}
			w.Header().Set("Content-Type", contentType)
			w.WriteHeader(record.Status)
			_, _ = w.Write(record.Body)
			return

		case store.IdempotencyConflict:
			// The same key on a different request is a client bug, and returning
			// the first response would be a lie about the second.
			writeIdempotencyHeaders(w, key, false)
			writeError(w, http.StatusUnprocessableEntity, "idempotency_key_reused",
				"this Idempotency-Key was already used for a different request")
			return

		case store.IdempotencyInFlight:
			// Wait for the first attempt rather than reporting a conflict. The
			// retry is not wrong, it is early, and the common cause is a client
			// that fired the same request twice.
			select {
			case <-done:
				_, record, _ := s.store.BeginIdempotent(principal.UserID, key, fingerprint)
				writeIdempotencyHeaders(w, key, true)
				contentType := record.ContentType
				if contentType == "" {
					contentType = "application/json"
				}
				w.Header().Set("Content-Type", contentType)
				if record.Status == 0 {
					// The first attempt failed before producing a response. Say so
					// rather than returning an empty 200 — and release the claim,
					// or the key is dead: expiry deliberately skips entries that
					// are still in flight, so an entry that is completed-never
					// and abandoned-never blocks that key for the lifetime of
					// the process.
					s.store.AbandonIdempotent(principal.UserID, key)
					writeError(w, http.StatusServiceUnavailable, "idempotent_request_failed",
						"the earlier request with this key did not complete")
					return
				}
				w.WriteHeader(record.Status)
				_, _ = w.Write(record.Body)
			case <-time.After(30 * time.Second):
				writeIdempotencyHeaders(w, key, false)
				writeError(w, http.StatusConflict, "idempotent_request_in_flight",
					"a request with this Idempotency-Key is still being processed")
			}
			return
		}

		// This attempt owns the key. Capture the response so a retry can be
		// answered from it.
		// The status starts at 0, not 200, and that is the fix for a bug that
		// handed a client a lie. A panicking handler unwinds through this defer
		// before the middleware's recover runs, so with a seeded 200 the claim
		// was completed as a successful empty response: the client got a 500,
		// and its retry was answered 200 with an empty body and
		// Idempotency-Replayed. Zero means "nothing was written", which is the
		// only honest value before the handler runs.
		recorder := &recordingWriter{ResponseWriter: w}
		defer func() {
			if recorder.status == 0 {
				// Nothing reached the client, so there is no outcome to remember.
				s.store.AbandonIdempotent(principal.UserID, key)
				return
			}
			if recorder.status == http.StatusPreconditionFailed {
				// Deliberately not memoised. A 412 is the server asking the
				// client to change its request, and the changed request must not
				// be answered with the refusal to the previous one.
				s.store.AbandonIdempotent(principal.UserID, key)
				return
			}
			if recorder.err != nil || recorder.status >= 500 {
				// A failed attempt is not remembered, so a corrected retry is not
				// blocked by a claim that will never complete.
				s.store.AbandonIdempotent(principal.UserID, key)
				return
			}
			s.store.CompleteIdempotent(principal.UserID, key, fingerprint,
				recorder.status, recorder.body.Bytes(), recorder.header("Content-Type"))
		}()

		writeIdempotencyHeaders(w, key, false)
		next(recorder, r)
	}
}

// sanitizeIdempotencyKey accepts a conservative shape or nothing. The key is
// echoed back in a response header and stored in the record, so a 4000-character
// or control-character value was both a response-header hazard and a row in the
// store that no client would ever look up again.
func sanitizeIdempotencyKey(raw string) string {
	const max = 255
	if raw == "" || len(raw) > max {
		return ""
	}
	for _, r := range raw {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '-', r == '_', r == '.':
		default:
			return ""
		}
	}
	return raw
}

// isIdempotencyEligible reports whether a request is worth tracking.
//
// Only unsafe methods qualify. A GET is already safe to repeat, and tracking one
// would fill the table with reads. The key has to be present, because a request
// without one has no way for the server to recognise the retry.
func isIdempotencyEligible(method, key string) bool {
	if key == "" {
		return false
	}
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

// writeIdempotencyHeaders echoes the key back on both a fresh response and a
// replay.
//
// Echoing it is what lets a client correlate a response it did not expect with
// the key it sent, and the replayed marker is how a caller can tell a retry from
// a first attempt without comparing bodies.
func writeIdempotencyHeaders(w http.ResponseWriter, key string, replayed bool) {
	if key == "" {
		return
	}
	w.Header().Set(IdempotencyHeader, key)
	if replayed {
		w.Header().Set("Idempotency-Replayed", "true")
	}
}

// maxIdempotentBody bounds the body that is read to build a fingerprint.
//
// The body is held in memory for the request either way, so this is not a new
// limit, but it keeps a keyed upload from being buffered twice.
const maxIdempotentBody = 8 << 20

func readBody(r *http.Request) ([]byte, error) {
	if r.Body == nil {
		return nil, nil
	}
	return io.ReadAll(io.LimitReader(r.Body, maxIdempotentBody))
}

// recordingWriter captures a response so it can be replayed verbatim.
type recordingWriter struct {
	http.ResponseWriter
	status   int
	body     bytes.Buffer
	err      error
	wroteHdr bool
}

// committed reports whether a response has already left. The middleware's
// recover needs it: once a status and part of a body are on the wire, a 500
// cannot be substituted, and appending one produces a body that is a valid
// prefix followed by an error object.
func (w *recordingWriter) committed() bool { return w.wroteHdr }

func (w *recordingWriter) WriteHeader(status int) {
	if w.wroteHdr {
		return
	}
	w.wroteHdr = true
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *recordingWriter) Write(b []byte) (int, error) {
	if !w.wroteHdr {
		w.WriteHeader(http.StatusOK)
	}
	n, err := w.body.Write(b)
	if err != nil {
		w.err = err
		return n, err
	}
	return w.ResponseWriter.Write(b)
}

// header reads a response header, defaulting to JSON.
func (w *recordingWriter) header(name string) string {
	value := w.ResponseWriter.Header().Get(name)
	if value == "" {
		return "application/json"
	}
	return value
}

// Unwrap lets the optional interfaces of the underlying writer, notably
// http.Flusher and http.Hijacker, stay reachable through this one.
func (w *recordingWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
