package httpapi

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/iSundram/codeceremony/backend/internal/domain"
)

// Concurrency for the resources that get overwritten.
//
// Two organizers editing the same submission is the ordinary case in this
// portal, not an exotic one. Last write wins silently means one of them loses
// work without being told, which is the worst outcome available: neither sees an
// error and the record shows only the survivor. A precondition turns that into a
// 412 and a re-read.
//
// The mechanism is HTTP's own: a weak-free strong ETag on read, and If-Match on
// write. It is opt-in per request, so a client that does not track versions
// keeps working exactly as before. What a client must never be able to do is
// claim a version it did not read, and that is what If-Match enforces.

// entityVersion is the version a resource exposes to clients.
//
// A bare integer is the right token for a row that already carries a Version,
// and it is what a client can reason about without parsing a hash. The submission
// and review paths use it directly; other resources fall back to a content hash.
type entityVersion struct {
	// version is the row's own version counter, when it has one.
	version int
	// digest is the content hash, used when there is no counter.
	digest string
}

func versionedVersion(version int) entityVersion {
	return entityVersion{version: version}
}

func contentVersion(parts ...string) entityVersion {
	digest := sha256.New()
	for _, part := range parts {
		_, _ = digest.Write([]byte(part))
		_, _ = digest.Write([]byte("\x00"))
	}
	return entityVersion{digest: hex.EncodeToString(digest.Sum(nil))[:32]}
}

// reviewVersion derives a version token for a review.
//
// A review has no version counter, and adding one would mean threading it
// through the snapshot and the judging read path for a row that only its own
// author ever writes. A content hash over the fields a concurrent writer would
// change is sufficient here and is honest about what it is.
//
// UpdatedAt is deliberately excluded. Every save moves it, so including it would
// change the token even when the judge saved the same scores twice, and a client
// that saves, then saves again without re-reading, would be refused against its
// own last write. Two clients can only collide by writing different content,
// which is precisely the case the check has to catch.
//
// Criteria are sorted before hashing. Map iteration order in Go is randomised,
// and a hash over unsorted map keys would produce a different ETag for an
// unchanged review, which would reject a client's own retry.
func reviewVersion(review domain.Review) entityVersion {
	keys := make([]string, 0, len(review.Criteria))
	for criterion := range review.Criteria {
		keys = append(keys, criterion)
	}
	sort.Strings(keys)
	parts := []string{
		review.ID, review.ProjectID, strconv.FormatBool(review.Submitted), review.Comment,
	}
	for _, criterion := range keys {
		parts = append(parts, criterion+"="+strconv.Itoa(review.Criteria[criterion]))
	}
	return contentVersion(parts...)
}

// etag renders the value as a strong ETag.
func (v entityVersion) etag() string {
	if v.version > 0 {
		return `"v` + strconv.Itoa(v.version) + `"`
	}
	return `"` + v.digest + `"`
}

// setETag writes the ETag on a response.
func setETag(w http.ResponseWriter, v entityVersion) {
	w.Header().Set("ETag", v.etag())
}

// checkPrecondition enforces If-Match and If-None-Match.
//
// The two are handled together because a client can send both and the rules for
// combining them are not intuitive. If-Match is the one that protects a write,
// and it is the only one that is allowed to fail a request here. If-None-Match is
// honoured on reads so a client polling for a change can ask cheaply.
//
// A request with no conditional header is always allowed. Making the check
// mandatory would break every client that does not track versions, and a
// concurrency control nobody opts into protects nothing.
func checkPrecondition(r *http.Request, current entityVersion) *preconditionError {
	ifMatch := strings.TrimSpace(r.Header.Get("If-Match"))
	ifNoneMatch := strings.TrimSpace(r.Header.Get("If-None-Match"))

	if ifMatch != "" {
		if !etagMatches(ifMatch, current) {
			return &preconditionError{
				status: http.StatusPreconditionFailed,
				code:   "version_conflict",
				message: fmt.Sprintf(
					"this resource has changed since it was read (current %s)", current.etag()),
			}
		}
	}
	if ifNoneMatch != "" && etagMatches(ifNoneMatch, current) {
		// Correct for a read that is asking "has it changed since I looked?".
		return &preconditionError{
			status:   http.StatusNotModified,
			code:     "not_modified",
			message:  "the resource has not changed",
			noChange: true,
		}
	}
	return nil
}

type preconditionError struct {
	status   int
	code     string
	message  string
	noChange bool
}

func (e *preconditionError) error() string { return e.message }

// writePreconditionError answers a failed precondition.
//
// A 412 carries the current ETag so a client can re-read and retry without a
// second round trip to discover what it now has to merge against.
func (s *Server) writePreconditionError(w http.ResponseWriter, current entityVersion, err *preconditionError) {
	if err.noChange {
		setETag(w, current)
		w.WriteHeader(http.StatusNotModified)
		return
	}
	setETag(w, current)
	writeError(w, err.status, err.code, err.message)
}

// etagMatches evaluates an If-Match or If-None-Match header against a value.
//
// * matches anything, and a list is an alternative rather than a conjunction, so
// any one matching tag is enough. A strong comparison is used deliberately: a
// weak tag promises byte equality, and a resource that changed and changed back
// is not the resource the client read.
func etagMatches(header string, current entityVersion) bool {
	header = strings.TrimSpace(header)
	if header == "" {
		return false
	}
	if header == "*" {
		return true
	}
	want := current.etag()
	for _, candidate := range strings.Split(header, ",") {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		// Accept a weak prefix on either side rather than failing a client that
		// round-tripped through a proxy that added one.
		candidate = strings.TrimPrefix(candidate, "W/")
		if candidate == want {
			return true
		}
	}
	return false
}
