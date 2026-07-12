package main

import (
	"context"
	"fmt"
	"html"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// cinepolis is the reference scraper: it shows the full pipeline end to end —
// real HTTP fetch of the public listing page, HTML parse, normalization into
// []Showtime with subtitled/dubbed detection, ready for the aggregator's cache.
//
// The other three chains (Cinemark / CCM / Sala Garbo) are stubs that plug into
// this exact shape once their parsers are written (see providers_stub.go).
//
// ponytail: the regexes below target the cartelera DOM described in
// testdata/cinepolis.html. The build sandbox's egress policy blocks
// cinepolis.co.cr, so these selectors are modeled on the chain's typical markup
// rather than verified against the live page — when deployed where egress is
// open, confirm them against the real DOM and adjust these three patterns (they
// are the *only* thing that needs to change). Everything downstream is fixed.
const cinepolisURL = "https://cinepolis.co.cr/cartelera"

type cinepolis struct {
	url string
}

func newCinepolis() cinepolis { return cinepolis{url: cinepolisURL} }

func (c cinepolis) Name() string { return "Cinépolis" }

func (c cinepolis) Fetch(ctx context.Context) ([]Showtime, error) {
	body, err := fetchPage(ctx, c.url)
	if err != nil {
		return nil, err
	}
	return parseCinepolis(body, time.Now().In(crZone))
}

// A movie card carries data-title and a set of "function" blocks; each function
// declares data-format (+ optional data-lang) and clickable showtime chips
// carrying data-datetime / data-cinema / href. See testdata/cinepolis.html.
//
// Rather than match nested closing tags (brittle — the last child's </div> gets
// eaten by the parent's terminator), we split the document on the opening
// markers and read attributes/chips out of each chunk. Robust to whitespace and
// unknown nesting.
const (
	cineMovieMarker    = `<div class="movie"`
	cineFunctionMarker = `<div class="function"`
)

var (
	reCineTitle    = regexp.MustCompile(`data-title="([^"]*)"`)
	reCineFormat   = regexp.MustCompile(`data-format="([^"]*)"`)
	reCineLang     = regexp.MustCompile(`data-lang="([^"]*)"`)
	reCineShowtime = regexp.MustCompile(`(?is)data-datetime="([^"]*)"(?:[^>]*data-cinema="([^"]*)")?(?:[^>]*href="([^"]*)")?`)
)

// parseCinepolis extracts showtimes from cartelera HTML. now anchors any
// date-less times to the current CR day (some chains render only the clock).
func parseCinepolis(body []byte, now time.Time) ([]Showtime, error) {
	var out []Showtime
	for _, movie := range splitOnMarker(string(body), cineMovieMarker) {
		title := cleanText(firstSubmatch(reCineTitle, movie))
		if title == "" {
			continue
		}
		for _, fn := range splitOnMarker(movie, cineFunctionMarker) {
			lang := cleanText(firstSubmatch(reCineLang, fn))
			format := ParseFormat(firstSubmatch(reCineFormat, fn) + " " + lang)
			for _, s := range reCineShowtime.FindAllStringSubmatch(fn, -1) {
				start, ok := parseCineTime(s[1], now)
				if !ok {
					continue
				}
				out = append(out, Showtime{
					Chain:    "Cinépolis",
					Cinema:   cleanText(s[2]),
					Movie:    title,
					Start:    start,
					Format:   format,
					Language: lang,
					BuyURL:   cleanText(s[3]),
				})
			}
		}
	}
	if len(out) == 0 {
		// A structurally-valid page that yields nothing almost always means the
		// DOM moved out from under the selectors — surface it instead of
		// silently caching an empty chain.
		return nil, fmt.Errorf("cinepolis: no showtimes parsed (selectors likely stale)")
	}
	return out, nil
}

// parseCineTime accepts either a full ISO timestamp ("2026-07-12T18:30") or a
// bare "HH:MM" clock, which it pins to now's date in CR local time.
func parseCineTime(raw string, now time.Time) (time.Time, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{"2006-01-02T15:04", "2006-01-02T15:04:05"} {
		if t, err := time.ParseInLocation(layout, raw, crZone); err == nil {
			return t, true
		}
	}
	if hh, mm, ok := parseClock(raw); ok {
		return time.Date(now.Year(), now.Month(), now.Day(), hh, mm, 0, 0, crZone), true
	}
	return time.Time{}, false
}

// parseClock reads "18:30" or "6:30" into hour/minute.
func parseClock(s string) (int, int, bool) {
	parts := strings.SplitN(s, ":", 2)
	if len(parts) != 2 {
		return 0, 0, false
	}
	hh, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
	mm, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err1 != nil || err2 != nil || hh < 0 || hh > 23 || mm < 0 || mm > 59 {
		return 0, 0, false
	}
	return hh, mm, true
}

// cleanText unescapes HTML entities and collapses whitespace.
func cleanText(s string) string {
	return strings.Join(strings.Fields(html.UnescapeString(s)), " ")
}
