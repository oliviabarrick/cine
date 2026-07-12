package main

import (
	"encoding/json"
	"net/http"
	"time"
)

// showtimeDTO is the wire shape for one screening. Start is split into an RFC3339
// timestamp (for sorting/filtering) plus pre-formatted CR date/time strings so
// the frontend needn't re-derive the timezone.
type showtimeDTO struct {
	Chain    string `json:"chain"`
	Cinema   string `json:"cinema"`
	Movie    string `json:"movie"`
	Start    string `json:"start"` // RFC3339, CR local
	Date     string `json:"date"`  // YYYY-MM-DD
	Time     string `json:"time"`  // HH:MM
	Format   Format `json:"format"`
	Language string `json:"language,omitempty"`
	Screen   string `json:"screen,omitempty"`
	BuyURL   string `json:"buyUrl,omitempty"`
}

// showtimesResponse is the full payload for GET /api/showtimes: the normalized
// screenings plus the metadata the UI needs to build filters and warn when a
// chain failed or data is stale.
type showtimesResponse struct {
	UpdatedAt string            `json:"updatedAt"`        // RFC3339, or "" before first refresh
	Stale     bool              `json:"stale"`            // last refresh older than staleAfter
	Errors    map[string]string `json:"errors,omitempty"` // chain -> last scrape error
	Chains    []string          `json:"chains"`           // facet: distinct chains present
	Cinemas   []string          `json:"cinemas"`          // facet: distinct cinemas present
	Showtimes []showtimeDTO     `json:"showtimes"`
}

// staleAfter is how old the cache may get before the API flags it stale (the
// refresh interval plus slack); the UI shows a "data may be outdated" notice.
const staleAfter = 45 * time.Minute

// apiHandler serves the cached, normalized showtimes as JSON. All filtering is
// done client-side in the browser for a snappy UI, so this is a plain dump plus
// facets — no query params to keep the cache a single, cacheable payload.
func apiHandler(agg *Aggregator) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		st, errs, updated := agg.Snapshot()

		resp := showtimesResponse{
			Errors:    errs,
			Showtimes: make([]showtimeDTO, 0, len(st)),
		}
		if !updated.IsZero() {
			resp.UpdatedAt = updated.Format(time.RFC3339)
			resp.Stale = time.Since(updated) > staleAfter
		}

		chains := newStringSet()
		cinemas := newStringSet()
		for _, s := range st {
			local := s.Start.In(crZone)
			resp.Showtimes = append(resp.Showtimes, showtimeDTO{
				Chain:    s.Chain,
				Cinema:   s.Cinema,
				Movie:    s.Movie,
				Start:    local.Format(time.RFC3339),
				Date:     local.Format("2006-01-02"),
				Time:     local.Format("15:04"),
				Format:   s.Format,
				Language: s.Language,
				Screen:   s.Screen,
				BuyURL:   s.BuyURL,
			})
			chains.add(s.Chain)
			if s.Cinema != "" {
				cinemas.add(s.Cinema)
			}
		}
		resp.Chains = chains.sorted()
		resp.Cinemas = cinemas.sorted()

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		// Short cache: the data refreshes server-side on a schedule; a minute of
		// edge/browser caching cuts load without serving badly stale times.
		w.Header().Set("Cache-Control", "public, max-age=60")
		_ = json.NewEncoder(w).Encode(resp)
	}
}
