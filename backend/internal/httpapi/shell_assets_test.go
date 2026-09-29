package httpapi

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The shell links five stylesheets and two brand assets. If one is missing the
// page renders unstyled or without its mark, and neither failure is visible in
// a unit test that only checks the status code.
func TestEveryShellAssetIsServed(t *testing.T) {
	server, _, _ := newTestServer(t)
	want := map[string]string{
		"/brand/logo.svg":        "image/svg+xml",
		"/brand/icon.svg":        "image/svg+xml",
		"/static/tokens.css":     "text/css; charset=utf-8",
		"/static/base.css":       "text/css; charset=utf-8",
		"/static/components.css": "text/css; charset=utf-8",
		"/static/shell.css":      "text/css; charset=utf-8",
		"/static/apps.css":       "text/css; charset=utf-8",
	}
	for path, contentType := range want {
		rec := httptest.NewRecorder()
		server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusOK {
			t.Errorf("GET %s = %d, want 200", path, rec.Code)
			continue
		}
		if got := rec.Header().Get("Content-Type"); got != contentType {
			t.Errorf("GET %s content type = %q, want %q", path, got, contentType)
		}
		if rec.Body.Len() == 0 {
			t.Errorf("GET %s served an empty body", path)
		}
	}
}

// The embed holds the whole fixture seed, so the asset routes must be an
// allowlist rather than a path join. A name that is not one of the approved
// files has to 404 rather than resolve.
func TestAssetRoutesRefuseAnythingNotOnTheAllowlist(t *testing.T) {
	server, _, _ := newTestServer(t)
	for _, path := range []string{
		"/brand/secrets.svg",
		"/brand/logo.png",
		"/static/seed.go",
		"/static/components.css.map",
	} {
		rec := httptest.NewRecorder()
		server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code == http.StatusOK {
			t.Errorf("GET %s = 200; the allowlist is not being enforced", path)
		}
	}
}

// design.md 10.2: the brand assets are used at their authored colours and are
// exempt from the palette. The exemption covers the mark and nothing else, so
// the vendored files must contain no white backdrop left over from the source.
func TestBrandAssetsHaveNoOpaqueBackdrop(t *testing.T) {
	for _, name := range []string{"logo.svg", "icon.svg"} {
		data := embeddedAsset(t, "webassets/brand/"+name)
		if strings.Contains(data, `fill="rgb(255,255,255)"`) {
			t.Errorf("%s still contains a white fill; the backdrop was not removed", name)
		}
		// A path whose geometry is exactly the viewBox rectangle is the backdrop.
		if strings.Contains(data, "L 0 0 L") {
			t.Errorf("%s contains a full-canvas rectangle; the backdrop was not removed", name)
		}
	}
}

func embeddedAsset(t *testing.T, path string) string {
	t.Helper()
	file, err := webAssets.Open(path)
	if err != nil {
		t.Fatalf("Open(%q) error = %v", path, err)
	}
	defer file.Close()
	data, err := io.ReadAll(file)
	if err != nil {
		t.Fatalf("reading %q error = %v", path, err)
	}
	return string(data)
}
