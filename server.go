package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"os"
	"path/filepath"
)

// server serves the static frontend bundle and the /api/showtimes endpoint.
//
// The static-asset handling mirrors the photo-editor template this repo is based
// on: index.html is rendered once at startup with a content fingerprint
// substituted for __VERSION__, so asset URLs become /app.js?v=<hash> and
// /app.css?v=<hash>. Behind Cloudflare the fixed paths are a single cache key, so
// the ?v= query (part of Cloudflare's and the browser's default cache key) makes
// every changed build a guaranteed cache miss, while index.html is served
// no-cache so the new query is always seen.
type server struct {
	index   []byte
	version string
	files   http.Handler
	api     http.Handler
}

// versionedAssets carry the ?v= fingerprint and may be cached indefinitely (a
// content change moves them to a new URL). index.html is excluded — always
// revalidated.
var versionedAssets = map[string]bool{
	"/app.js":  true,
	"/app.css": true,
}

func newServer(dir string, agg *Aggregator) (*server, error) {
	read := func(name string) ([]byte, error) { return os.ReadFile(filepath.Join(dir, name)) }

	idx, err := read("index.html")
	if err != nil {
		return nil, err
	}
	js, err := read("app.js")
	if err != nil {
		return nil, err
	}
	css, err := read("app.css")
	if err != nil {
		return nil, err
	}

	// Fingerprint every file whose bytes affect the rendered page, so any edit
	// invalidates the cached asset URLs.
	h := sha256.New()
	h.Write(idx)
	h.Write(js)
	h.Write(css)
	version := hex.EncodeToString(h.Sum(nil))[:12]

	return &server{
		index:   bytes.ReplaceAll(idx, []byte("__VERSION__"), []byte(version)),
		version: version,
		files:   http.FileServer(http.Dir(dir)),
		api:     apiHandler(agg),
	}, nil
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/", "/index.html":
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		w.Write(s.index)
		return
	case "/api/showtimes":
		s.api.ServeHTTP(w, r)
		return
	}

	if versionedAssets[r.URL.Path] && r.URL.Query().Get("v") != "" {
		// Safe to cache hard: the URL is content-addressed by ?v=, so a stale
		// body is never served under a URL whose bytes have changed.
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	}
	s.files.ServeHTTP(w, r)
}
