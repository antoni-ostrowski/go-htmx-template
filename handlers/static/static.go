// Package static serves the embedded assets (vendored JS, built CSS) under
// /static/. Assets are precompressed once at startup (raw + gzip), so no
// per-request CPU. Responses carry year-long immutable caching when the
// request has the ?v= content hash (the only URLs our HTML emits).
package static

import (
	"bytes"
	"compress/gzip"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"

	assets "go-htmx-template/static"

	"github.com/pocketbase/pocketbase/core"
)

type cachedAsset struct {
	raw, gz    []byte
	contentTyp string
	etag       string
}

var cache = loadAll()

func loadAll() map[string]cachedAsset {
	out := map[string]cachedAsset{}
	etag := `"` + assets.Version + `"`
	_ = fs.WalkDir(assets.FS, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		b, err := assets.FS.ReadFile(p)
		if err != nil {
			panic("static: unreadable embedded file " + p)
		}
		ct := mime.TypeByExtension(path.Ext(p))
		if ct == "" {
			ct = "application/octet-stream"
		}
		var gz []byte
		if compressible(ct) {
			var buf bytes.Buffer
			w, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
			if err != nil {
				panic(err)
			}
			if _, err := w.Write(b); err != nil {
				panic(err)
			}
			if err := w.Close(); err != nil {
				panic(err)
			}
			gz = buf.Bytes()
		}
		out[p] = cachedAsset{raw: b, gz: gz, contentTyp: ct, etag: etag}
		return nil
	})
	return out
}

func compressible(ct string) bool {
	return strings.Contains(ct, "text/") ||
		strings.Contains(ct, "javascript") ||
		strings.Contains(ct, "json") ||
		strings.Contains(ct, "css") ||
		strings.Contains(ct, "html") ||
		strings.Contains(ct, "xml") ||
		strings.Contains(ct, "svg")
}

// Register serves the embedded asset cache at GET /static/{path...}.
// Directory listings are 404; path traversal is rejected. Clients signalling
// gzip via Accept-Encoding get the precompressed bytes with
// Content-Encoding: gzip, others get raw. ETag matches assets.Version, so
// If-None-Match → 304, with Vary: Accept-Encoding.
func Register(se *core.ServeEvent) {
	se.Router.GET("/static/{path...}", func(e *core.RequestEvent) error {
		p := e.Request.PathValue("path")
		if p == "" || strings.HasSuffix(p, "/") || strings.Contains(p, "..") {
			return e.NotFoundError("not found", nil)
		}
		p = path.Clean(p)
		a, ok := cache[p]
		if !ok {
			return e.NotFoundError("not found", nil)
		}
		if e.Request.URL.Query().Has("v") {
			e.Response.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			e.Response.Header().Set("Cache-Control", "public, max-age=300")
		}
		e.Response.Header().Set("Content-Type", a.contentTyp)
		e.Response.Header().Set("ETag", a.etag)
		e.Response.Header().Set("Vary", "Accept-Encoding")
		if inm := e.Request.Header.Get("If-None-Match"); inm != "" && inm == a.etag {
			return e.NoContent(http.StatusNotModified)
		}
		if a.gz != nil && strings.Contains(strings.ToLower(e.Request.Header.Get("Accept-Encoding")), "gzip") {
			e.Response.Header().Set("Content-Encoding", "gzip")
			_, _ = e.Response.Write(a.gz)
			return nil
		}
		_, _ = e.Response.Write(a.raw)
		return nil
	})
}
