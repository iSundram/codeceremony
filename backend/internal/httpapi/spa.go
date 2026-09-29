package httpapi

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// The built single-page app.
//
// The frontend is a Vite build whose output is copied into this package's
// webassets/spa directory before the binary is compiled, then embedded with
// go:embed. The production container therefore carries no Node runtime and no
// second process: the one-command rule holds, and `docker compose up` needs a
// network only while the image is built, never to run the portal.
//
// When the directory is absent — which is the case for `go test ./...` on a
// checkout where the frontend has not been built — the portal still serves the
// JSON API and the legacy server-rendered pages. That is deliberate: the backend
// must be runnable and testable without a Node toolchain, and a build step that
// gates the API would couple them.
//
//go:embed all:webassets/spa
var spaAssets embed.FS

const spaRoot = "webassets/spa"

// spaBuilt reports whether a build is present.
func spaBuilt() bool {
	entries, err := spaAssets.ReadDir(spaRoot)
	return err == nil && len(entries) > 0
}

// registerSPA mounts the built app.
//
// Two rules matter here. First, unknown paths under the app's own namespaces fall
// through to index.html, because a client-side route is not a server route: a
// hard refresh on /organizer/audit must not 404. Second, that fallthrough is
// strictly limited to GET on a path with no file extension, so a request for a
// missing asset still 404s instead of being answered with HTML and a 200, which
// is the failure mode that makes a broken bundle very hard to diagnose.
func (s *Server) registerSPA(mux *routeMux) {
	if !spaBuilt() {
		return
	}

	assets := http.FileServer(http.FS(spaAssets))

	mux.Handle("GET /assets/{path...}", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Hashed filenames mean an immutable asset is safe, and index.html must
		// not be cached or a deploy would leave a stale shell in every browser.
		if strings.HasPrefix(r.PathValue("path"), "index-") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		assets.ServeHTTP(w, r)
	}))

	// design.md 10.2: the full lockup and the mark alone are not
	// interchangeable, so both are served under /brand/ and each caller picks.
	// The allowlist is deliberate, because the embed also holds the fixture seed.
	mux.Handle("GET /brand/{name}", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if name != "logo.svg" && name != "icon.svg" {
			http.NotFound(w, r)
			return
		}
		data, err := spaAssets.ReadFile(spaRoot + "/" + name)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "image/svg+xml")
		w.Header().Set("Cache-Control", "public, max-age=86400")
		_, _ = w.Write(data)
	}))

	// Sanity check that the expected files are actually present, because a
	// mis-staged build produces a portal with no shell and no obvious cause.
	for _, want := range []string{"index.html", "logo.svg", "icon.svg"} {
		if _, err := spaAssets.ReadFile(spaRoot + "/" + want); err != nil {
			s.logger.Warn("the staged frontend build is incomplete", "missing", want,
				"hint", "run scripts/build_frontend.sh")
		}
	}

	index, err := spaAssets.ReadFile(spaRoot + "/index.html")
	if err != nil {
		return
	}
	indexHTML := func(w http.ResponseWriter) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "same-origin")
		_, _ = w.Write(index)
	}

	// The app's own paths. Anything here that is not a real file is a client
	// route and gets the shell. The bare root is left to the server-rendered
	// home page, which still exists and is what a first-time visitor expects to
	// land on.
	mux.Handle("GET /{path...}", s.clientRoute(indexHTML))
}

// clientRoute serves index.html for a client-side route and 404s for a missing
// asset.
//
// The extension test is the whole point. /organizer/audit has no extension and
// is a client route; /assets/missing-BADHASH.js has one and is a broken build.
// Answering the second with HTML and a 200 produces a page that fails in the
// console with a syntax error on <!doctype, which is far harder to trace than a
// 404 in the network panel.
func (s *Server) clientRoute(index func(http.ResponseWriter)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rest := strings.TrimPrefix(r.URL.Path, "/")
		if rest == "" {
			index(w)
			return
		}
		if path.Ext(rest) != "" {
			// A real file request. Serve it if it exists, otherwise 404.
			if data, err := spaAssets.ReadFile(spaRoot + "/" + rest); err == nil {
				w.Header().Set("Content-Type", contentTypeFor(rest))
				w.Header().Set("Cache-Control", "public, max-age=3600")
				_, _ = w.Write(data)
				return
			}
			http.NotFound(w, r)
			return
		}
		// A path that resolves to a directory is still a client route.
		if sub, err := fs.Stat(spaAssets, spaRoot+"/"+rest); err == nil && sub.IsDir() {
			index(w)
			return
		}
		index(w)
	})
}

func contentTypeFor(name string) string {
	switch path.Ext(name) {
	case ".css":
		return "text/css; charset=utf-8"
	case ".js", ".mjs":
		return "text/javascript; charset=utf-8"
	case ".svg":
		return "image/svg+xml"
	case ".json":
		return "application/json"
	case ".woff2":
		return "font/woff2"
	case ".png":
		return "image/png"
	case ".ico":
		return "image/x-icon"
	case ".webmanifest":
		return "application/manifest+json"
	default:
		return "application/octet-stream"
	}
}

// spaPresent reports whether a build is present, for the test that asserts the
// two-build story: the API must be reachable either way.
func spaPresent() bool { return spaBuilt() }
