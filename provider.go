package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// httpError is a non-200 response from a cinema site, kept distinct so callers
// (and logs) can tell "the site said no" from a transport/parse failure.
type httpError struct {
	URL    string
	Status int
}

func (e *httpError) Error() string {
	return fmt.Sprintf("GET %s: HTTP %d", e.URL, e.Status)
}

// readAllLimited reads up to max bytes, erroring if the body is larger, so an
// unexpectedly huge page can't blow up memory during a refresh.
func readAllLimited(r io.Reader, max int64) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(r, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > max {
		return nil, fmt.Errorf("response exceeds %d bytes", max)
	}
	return b, nil
}

// errNotImplemented is returned by providers that are scaffolded but whose live
// scraper hasn't been wired up yet (see providers_stub.go). The aggregator
// tolerates it: a not-yet-implemented chain never breaks the others.
var errNotImplemented = errors.New("provider not implemented yet")

// Provider scrapes one cinema chain and returns normalized showtimes. Each chain
// is fully isolated behind this interface, so one site changing its markup — or
// being unreachable — only affects that chain's results, never the whole app.
type Provider interface {
	// Name is the human-facing chain name used in results and UI facets.
	Name() string
	// Fetch scrapes the chain's current listings. It must honor ctx for
	// cancellation/timeout and return a non-nil error rather than panicking on
	// unexpected markup.
	Fetch(ctx context.Context) ([]Showtime, error)
}

// providers is the registry of every chain the app aggregates. Cinépolis is a
// live reference implementation; the rest are stubs awaiting their scrapers.
func providers() []Provider {
	return []Provider{
		newCinepolis(),
		newCinemark(),
		newCCM(),
		newSalaGarbo(),
	}
}

// httpClient is the shared client for all scrapers: a bounded timeout so a slow
// or hanging cinema site can't wedge a refresh cycle.
var httpClient = &http.Client{Timeout: 20 * time.Second}

// fetchPage GETs url with a browser-like User-Agent (several of these sites 403
// bare clients) and returns the body, honoring ctx.
func fetchPage(ctx context.Context, url string) ([]byte, error) {
	return fetchPageWithHeaders(ctx, url, nil)
}

// fetchPageWithHeaders is fetchPage plus caller-supplied headers, used by chains
// (e.g. Cinépolis) whose showtimes endpoint needs an Authorization bearer.
func fetchPageWithHeaders(ctx context.Context, url string, headers map[string]string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125 Safari/537.36")
	req.Header.Set("Accept-Language", "es-CR,es;q=0.9,en;q=0.8")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, &httpError{URL: url, Status: resp.StatusCode}
	}
	// Cap the read so a misbehaving upstream can't exhaust memory.
	return readAllLimited(resp.Body, 8<<20)
}

// postJSON POSTs reqBody as JSON to url and returns the response body, honoring
// ctx. Some chains (Cinépolis) mint a short-lived auth token this way before
// their read endpoints will answer.
func postJSON(ctx context.Context, url string, reqBody any) ([]byte, error) {
	buf, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(buf))
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125 Safari/537.36")
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, &httpError{URL: url, Status: resp.StatusCode}
	}
	return readAllLimited(resp.Body, 8<<20)
}
