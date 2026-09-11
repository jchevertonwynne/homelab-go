// Package etag computes strong ETags over content compiled into the binary,
// and serves it so that a repeat request costs a 304.
//
// The apps had five copies of the same recipe. Truncating the digest to 16
// bytes is a convention rather than a rule, which is exactly why it belongs
// in one place: five copies of an arbitrary choice is five chances to pick a
// different length and quietly change cache behaviour on one app.
package etag

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"net/http"
	"time"
)

// Of returns a strong validator over content. Quoted and not weak: this is a
// byte-for-byte comparison. Content-derived, so a deploy that leaves a file
// alone still answers 304.
func Of(content []byte) string {
	sum := sha256.Sum256(content)
	return `"` + hex.EncodeToString(sum[:16]) + `"`
}

// Map walks root inside fsys and returns URL path ("/"+path) to ETag.
//
// An error here is a build problem, not a runtime one — these files are
// embedded in the binary — so callers are expected to treat it the way they
// treat template.Must: refuse to start rather than serve something subtly
// wrong.
func Map(fsys fs.FS, root string) (map[string]string, error) {
	etags := make(map[string]string)
	err := fs.WalkDir(fsys, root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		content, err := fs.ReadFile(fsys, path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		etags["/"+path] = Of(content)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return etags, nil
}

// Handler serves fsys with the ETag from etags and Cache-Control: no-cache.
//
// The header is set before delegating because that is where
// http.ServeContent looks for it: its If-None-Match check reads what is
// already on the ResponseWriter, so setting it here is what turns a repeat
// request into a 304 rather than the whole file again. http.FileServerFS
// cannot supply an ETag for an embed.FS on its own, which is why the map
// exists.
//
// no-cache rather than a max-age: "keep it, but ask me first". A max-age is a
// guess at how long a file stays the same, and a wrong guess cannot be
// corrected from the server — a client that cached it holds it for the full
// window. A conditional request that answers 304 costs almost nothing.
func Handler(fsys fs.FS, etags map[string]string) http.Handler {
	fileServer := http.FileServerFS(fsys)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if tag, ok := etags[r.URL.Path]; ok {
			w.Header().Set("ETag", tag)
		}
		w.Header().Set("Cache-Control", "no-cache")
		fileServer.ServeHTTP(w, r)
	})
}

// ServeBytes serves one embedded file with its ETag. For the single-asset
// case — an icon — where a map would be one entry long.
//
// The zero modtime means no Last-Modified: the bytes are compiled in and have
// no meaningful modification time, which is the whole reason the ETag is here.
func ServeBytes(w http.ResponseWriter, r *http.Request, name string, content []byte, tag string) {
	w.Header().Set("ETag", tag)
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(content))
}
