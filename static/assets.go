// Package assets embeds the public static files into the binary, so the
// server never touches disk at runtime and no asset-dir path is threaded
// through Config. Add new public files to the go:embed list below.
// Tailwind's input.css is deliberately excluded (build source, not served).
package assets

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
)

// FS holds css/app.css and js/htmax.min.js, served under /static/.
// Paths inside FS are relative to this directory, e.g. "css/app.css".
//
//go:embed css/app.css js
var FS embed.FS

// Version is a short content hash over the embedded files, used as the
// ?v= cache-buster in asset URLs. Content change → new URL → browsers fetch
// fresh; unchanged → the immutable Cache-Control lets them skip the request
// entirely. Recomputed at startup from the embedded bytes, so no build step.
var Version = func() string {
	h := sha256.New()
	for _, f := range []string{"css/app.css", "js/htmax.min.js", "js/realtime.js"} {
		b, err := FS.ReadFile(f)
		if err != nil {
			panic("assets: unreadable embedded file " + f)
		}
		h.Write(b)
	}
	return hex.EncodeToString(h.Sum(nil))[:12]
}()
