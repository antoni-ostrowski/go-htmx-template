// Package static serves the embedded assets (vendored JS, built CSS) under
// /static/. Conditional requests (ETag, Last-Modified) come free with
// ServeContent; add immutable caching only for content-hashed filenames.
package static

import (
	"net/http"
	"strings"

	"go-htmx-template/handlers"
	assets "go-htmx-template/static"
)

// Register serves the embedded asset FS at GET /static/. Directory listings
// are 404; path traversal is rejected by the file server. Responses carry
// year-long immutable caching when the request has the ?v= content hash
// (the only URLs our HTML emits), so browsers never re-request unchanged
// assets; bare URLs get an hour so a stale copy can't stick around.
func Register(mux *http.ServeMux) {
	files := http.FileServerFS(assets.FS)
	handlers.Route(mux, "GET /static/", http.StripPrefix("/static/", http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "/") {
				http.NotFound(w, r)
				return
			}
			// all of req from our web app hit this immutable cache
			if r.URL.Query().Has("v") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			} else {
				// basically never hits
				w.Header().Set("Cache-Control", "public, max-age=300")
			}
			files.ServeHTTP(w, r)
		})))
}
