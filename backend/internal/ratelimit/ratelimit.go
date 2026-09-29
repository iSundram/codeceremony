// Package ratelimit caps how often a caller may use an endpoint.
//
// The abuse cases a judging platform actually has to survive are all
// high-frequency, low-effort ones: credential stuffing against the login
// endpoint, ballot stuffing on community voting, and comment flooding. None of
// them need a sophisticated defence; they need a number.
//
// # Design
//
// A fixed window per (policy, caller) pair, held in memory. That is the right
// shape for a single-process self-hosted portal: there is no second node to keep
// in sync with, and a counter that resets on restart is a counter an attacker
// can wait out, which is why the window is bounded by wall-clock time rather
// than by request count.
//
// A fixed window lets a caller send 2x the limit across a window boundary. That
// is accepted deliberately: halving it to remove the artefact would halve the
// real throughput for every legitimate user, and for abuse prevention the
// doubled burst at the boundary is not the interesting part.
//
// The caller key is supplied by the caller, not derived here, because only the
// HTTP layer knows whether to identify by address, by account, or by both.
package ratelimit

import (
	"net/http"
	"strconv"
	"sync"
	"time"
)

// Policy is a named request budget.
type Policy struct {
	// Limit is how many requests are allowed per window.
	Limit int
	// Window is the length of the fixed window.
	Window time.Duration
	// Name identifies the policy in responses and logs.
	Name string
}

// Policies. The numbers are chosen so that a person clicking through a judging
// console never notices them, and a script cannot make meaningful progress.
var (
	// Login is strict because it is the only endpoint where an unauthenticated
	// caller can spend unlimited attempts against a credential.
	Login = Policy{Name: "login", Limit: 10, Window: time.Minute}
	// Account is for anything that creates or authenticates an account.
	Account = Policy{Name: "account", Limit: 20, Window: time.Minute}
	// Ballot covers voting. The limit is per user and per campaign, so one
	// determined voter cannot spend the whole community's budget.
	Ballot = Policy{Name: "ballot", Limit: 10, Window: time.Minute}
	// Comment covers comment posting and reporting.
	Comment = Policy{Name: "comment", Limit: 15, Window: time.Minute}
	// Write covers authenticated state changes that are not abuse-prone enough
	// for their own budget: submissions, reviews, profile edits.
	Write = Policy{Name: "write", Limit: 120, Window: time.Minute}
	// Read is a backstop against scraping the gallery. It is generous.
	Read = Policy{Name: "read", Limit: 600, Window: time.Minute}
)

type window struct {
	started time.Time
	count   int
}

// Limiter tracks fixed windows per key.
type Limiter struct {
	mu      sync.Mutex
	windows map[string]window
	// now is injectable so tests do not have to sleep.
	now func() time.Time
}

// New returns a limiter with an empty window table.
func New() *Limiter {
	return &Limiter{windows: make(map[string]window), now: time.Now}
}

// Allow records a request against a policy and key, reporting whether it is
// permitted and, if not, how long until the window rolls over.
func (l *Limiter) Allow(policy Policy, key string) (bool, time.Duration) {
	if policy.Limit <= 0 || policy.Window <= 0 || key == "" {
		return true, 0
	}
	composite := policy.Name + "\x00" + key
	now := l.now()

	l.mu.Lock()
	defer l.mu.Unlock()
	current, ok := l.windows[composite]
	if !ok || now.Sub(current.started) >= policy.Window {
		l.windows[composite] = window{started: now, count: 1}
		l.sweepLocked(now)
		return true, 0
	}
	if current.count >= policy.Limit {
		return false, policy.Window - now.Sub(current.started)
	}
	current.count++
	l.windows[composite] = current
	return true, 0
}

// sweepLocked drops windows that can no longer have any effect, so a long run
// with many distinct keys does not grow the table without bound.
func (l *Limiter) sweepLocked(now time.Time) {
	// Sweeping on every new key is O(n); the table is small and a cheap guard
	// keeps it from becoming a cost on its own.
	if len(l.windows) < 4096 {
		return
	}
	longest := Read.Window
	for _, policy := range []Policy{Login, Account, Ballot, Comment, Write, Read} {
		if policy.Window > longest {
			longest = policy.Window
		}
	}
	for key, entry := range l.windows {
		if now.Sub(entry.started) > longest {
			delete(l.windows, key)
		}
	}
}

// Reset clears all state. Used on tests and on an operator-forced reset.
func (l *Limiter) Reset() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.windows = make(map[string]window)
}

// Middleware applies a policy to every request, keyed by the client address.
//
// It is the blunt instrument and is meant for the read path. Endpoints where
// the meaningful identity is the account rather than the address use Guard
// directly, because many judges and participants share one NAT egress and
// sharing a budget would be a self-inflicted denial of service.
func (l *Limiter) Middleware(policy Policy, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !l.Guard(w, policy, clientKey(r)) {
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Guard applies a policy and, on refusal, writes a 429 with a Retry-After
// header. It reports whether the request may proceed.
func (l *Limiter) Guard(w http.ResponseWriter, policy Policy, key string) bool {
	allowed, retry := l.Allow(policy, key)
	if allowed {
		return true
	}
	seconds := int(retry.Seconds())
	if seconds < 1 {
		seconds = 1
	}
	w.Header().Set("Retry-After", strconv.Itoa(seconds))
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusTooManyRequests)
	body := `{"error":{"code":"rate_limited","message":"too many ` + policy.Name + ` requests, slow down"}}`
	_, _ = w.Write([]byte(body))
	return false
}

// clientKey identifies the caller by remote address, ignoring the port.
func clientKey(r *http.Request) string {
	host := r.RemoteAddr
	for index := len(host) - 1; index >= 0; index-- {
		if host[index] == ':' {
			host = host[:index]
			break
		}
	}
	if host == "" {
		host = "unknown"
	}
	return host
}
