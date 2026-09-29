package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The portal is one binary with no Node runtime, and the built app is embedded
// in it. These tests assert the two things that could silently break that: the
// app is actually served, and a missing asset does not come back as HTML with a
// 200.
func TestTheBuiltAppIsServed(t *testing.T) {
	if !spaPresent() {
		t.Skip("no frontend build staged; run scripts/build_frontend.sh")
	}
	server, _, _ := newTestServer(t)

	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/dashboard", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /dashboard = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "<div id=\"root\">") {
		t.Error("the client route did not return the app shell")
	}
	// A client route is not a server route, so a hard refresh must work. The
	// shell is what gets served, and the router takes it from there.
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("the shell cache control is %q; a stale shell would outlive a deploy", got)
	}
}

func TestAMissingAssetIsNotAnsweredWithHTML(t *testing.T) {
	if !spaPresent() {
		t.Skip("no frontend build staged")
	}
	server, _, _ := newTestServer(t)

	// A broken bundle normally shows up as a 200 full of "<!doctype html" and a
	// console syntax error, which is very hard to trace. The extension test is
	// what keeps that from happening.
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/assets/index-NOTAREALHASH.js", nil))
	if rec.Code == http.StatusOK && strings.Contains(rec.Body.String(), "<!doctype") {
		t.Error("a missing script was answered with the HTML shell and a 200")
	}
}

// The API must not be swallowed by the SPA fallback. This is the assertion that
// keeps a change to the client-route rule from quietly taking the JSON API with
// it.
func TestTheAPIIsNotShadowedByTheApp(t *testing.T) {
	server, _, _ := newTestServer(t)
	for _, path := range []string{"/v1/permissions", "/v1/events", "/healthz"} {
		rec := httptest.NewRecorder()
		server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if strings.Contains(rec.Header().Get("Content-Type"), "text/html") {
			t.Errorf("GET %s returned the app shell; the API route was shadowed", path)
		}
	}
}

// The brand assets are served from the build output, so a fresh clone has the
// logo the sidebar expects.
func TestTheBrandAssetsAreServed(t *testing.T) {
	if !spaPresent() {
		t.Skip("no frontend build staged")
	}
	server, _, _ := newTestServer(t)
	for _, name := range []string{"logo.svg", "icon.svg"} {
		rec := httptest.NewRecorder()
		server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/brand/"+name, nil))
		if rec.Code != http.StatusOK {
			t.Errorf("GET /brand/%s = %d, want 200", name, rec.Code)
			continue
		}
		if got := rec.Header().Get("Content-Type"); got != "image/svg+xml" {
			t.Errorf("GET /brand/%s content type = %q", name, got)
		}
		// design.md 10.2: the vendored assets are transparent, so a white
		// rectangle would render as a box on the mist canvas.
		if strings.Contains(rec.Body.String(), `fill="rgb(255,255,255)"`) {
			t.Errorf("/brand/%s still has an opaque backdrop", name)
		}
	}
}
