package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeBundle lays down a minimal web/ bundle for the server to serve.
func writeBundle(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"index.html": `<!doctype html><link rel="stylesheet" href="/app.css?v=__VERSION__"><script src="/app.js?v=__VERSION__"></script>`,
		"app.js":     `console.log("cine");`,
		"app.css":    `body{margin:0}`,
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	return dir
}

func testServer(t *testing.T) *server {
	t.Helper()
	agg := newAggregator([]Provider{
		fakeProvider{name: "Cinépolis", st: []Showtime{
			{Chain: "Cinépolis", Cinema: "San Pedro", Movie: "Dune", Start: at(18, 30), Format: FormatSubtitled, Language: "Inglés"},
		}},
	})
	agg.Refresh(context.Background())
	srv, err := newServer(writeBundle(t), agg)
	if err != nil {
		t.Fatalf("newServer: %v", err)
	}
	return srv
}

func TestNewServerInjectsVersion(t *testing.T) {
	srv := testServer(t)
	if srv.version == "" {
		t.Fatal("version is empty")
	}
	if strings.Contains(string(srv.index), "__VERSION__") {
		t.Errorf("placeholder not substituted: %s", srv.index)
	}
	if !strings.Contains(string(srv.index), "?v="+srv.version) {
		t.Errorf("index does not reference versioned assets: %s", srv.index)
	}
}

func TestServerRoutes(t *testing.T) {
	srv := testServer(t)

	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET / = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-cache" {
		t.Errorf("GET / Cache-Control = %q, want no-cache", got)
	}

	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/app.js?v="+srv.version, nil))
	if got := rec.Header().Get("Cache-Control"); !strings.Contains(got, "max-age=31536000") {
		t.Errorf("GET /app.js Cache-Control = %q, want immutable long cache", got)
	}
}

func TestAPIShowtimes(t *testing.T) {
	srv := testServer(t)

	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/showtimes", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/showtimes = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Errorf("Content-Type = %q, want json", ct)
	}

	var resp showtimesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Showtimes) != 1 {
		t.Fatalf("got %d showtimes, want 1", len(resp.Showtimes))
	}
	s := resp.Showtimes[0]
	if s.Movie != "Dune" || s.Format != FormatSubtitled || s.Time != "18:30" {
		t.Errorf("unexpected showtime DTO: %+v", s)
	}
	if want := []string{"Cinépolis"}; len(resp.Chains) != 1 || resp.Chains[0] != want[0] {
		t.Errorf("chains facet = %v, want %v", resp.Chains, want)
	}
	if len(resp.Cinemas) != 1 || resp.Cinemas[0] != "San Pedro" {
		t.Errorf("cinemas facet = %v", resp.Cinemas)
	}
	if resp.UpdatedAt == "" {
		t.Error("updatedAt empty after refresh")
	}
}

func TestGzip(t *testing.T) {
	srv := testServer(t)
	h := gzipMiddleware(srv)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if got := rec.Header().Get("Content-Encoding"); got != "gzip" {
		t.Errorf("Content-Encoding = %q, want gzip", got)
	}
}

func TestNewServerMissingFile(t *testing.T) {
	if _, err := newServer(t.TempDir(), newAggregator(nil)); err == nil {
		t.Fatal("expected error for missing index.html, got nil")
	}
}
