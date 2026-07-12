package main

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Sala Garbo is a single art-house cinema in San José whose cartelera is a plain
// server-rendered WordPress page (a bespoke "sg-events" plugin over WooCommerce).
// There is no JSON API — we parse the HTML directly.
//
// Each film is a `<div class="row sg-row ...">` block holding an `<h2>` title, a
// product permalink, and a `<ul class="event-purchase-list">` whose `<li>`s are
// the individual screenings ("12 Jul 2026, 15:00"). The page never labels
// sub/dub: Sala Garbo screens foreign-language films in their original audio with
// Spanish subtitles, so every screening is subtituladas (also the app default).
const (
	salaGarboURL    = "https://salagarbo.com/cartelera/"
	salaGarboCinema = "Sala Garbo"
)

type salaGarbo struct {
	url string
}

func newSalaGarbo() salaGarbo { return salaGarbo{url: salaGarboURL} }

func (s salaGarbo) Name() string { return "Sala Garbo" }

func (s salaGarbo) Fetch(ctx context.Context) ([]Showtime, error) {
	body, err := fetchPage(ctx, s.url)
	if err != nil {
		return nil, err
	}
	return parseSalaGarbo(string(body))
}

const sgMovieMarker = `<div class="row sg-row`

var (
	reSGTitle    = regexp.MustCompile(`(?is)<h2[^>]*>(.*?)</h2>`)
	reSGHref     = regexp.MustCompile(`href="(https://salagarbo\.com/product/[^"]+)"`)
	reSGAddName  = regexp.MustCompile(`(?is)event_additional_name[^>]*>(.*?)</div>`)
	reSGDateTime = regexp.MustCompile(`(\d{1,2})\s+([A-Za-zÁÉÍÓÚáéíóú]{3,})\s+(\d{4}),\s*(\d{1,2}):(\d{2})`)
)

// parseSalaGarbo extracts showtimes from the cartelera HTML.
func parseSalaGarbo(body string) ([]Showtime, error) {
	var out []Showtime
	for _, block := range splitOnMarker(body, sgMovieMarker) {
		title := cleanText(firstSubmatch(reSGTitle, block))
		if title == "" {
			continue
		}
		buyURL := firstSubmatch(reSGHref, block)

		// Each <li> is one screening. Splitting on the opening tag keeps every
		// screening's date + optional per-film name (used by film-club series
		// where the <h2> is the series, not the film).
		for _, li := range strings.Split(block, "<li>")[1:] {
			m := reSGDateTime.FindStringSubmatch(li)
			if m == nil {
				// No date (e.g. a "Pase Mensual" season pass) — not a screening.
				continue
			}
			start, ok := parseSGDateTime(m[1], m[2], m[3], m[4], m[5])
			if !ok {
				continue
			}
			movie := cleanText(firstSubmatch(reSGAddName, li))
			if movie == "" {
				movie = title
			}
			out = append(out, Showtime{
				Chain:  "Sala Garbo",
				Cinema: salaGarboCinema,
				Movie:  movie,
				Start:  start,
				Format: FormatSubtitled,
				BuyURL: buyURL,
			})
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("salagarbo: no showtimes parsed (page markup likely changed)")
	}
	return out, nil
}

// sgMonths maps the Spanish month abbreviations Sala Garbo prints ("Jul") onto
// months. September appears as both "Sep" and "Set" in CR usage.
var sgMonths = map[string]time.Month{
	"ene": time.January, "feb": time.February, "mar": time.March,
	"abr": time.April, "may": time.May, "jun": time.June,
	"jul": time.July, "ago": time.August, "sep": time.September,
	"set": time.September, "oct": time.October, "nov": time.November,
	"dic": time.December,
}

// parseSGDateTime builds a CR-local time from "12 Jul 2026" + "15:00" pieces.
func parseSGDateTime(dayStr, monStr, yearStr, hhStr, mmStr string) (time.Time, bool) {
	key := strings.ToLower(monStr)
	if len(key) > 3 {
		key = key[:3]
	}
	mon, ok := sgMonths[key]
	if !ok {
		return time.Time{}, false
	}
	day, err1 := strconv.Atoi(dayStr)
	year, err2 := strconv.Atoi(yearStr)
	hh, err3 := strconv.Atoi(hhStr)
	mm, err4 := strconv.Atoi(mmStr)
	if err1 != nil || err2 != nil || err3 != nil || err4 != nil {
		return time.Time{}, false
	}
	if day < 1 || day > 31 || hh > 23 || mm > 59 {
		return time.Time{}, false
	}
	return time.Date(year, mon, day, hh, mm, 0, 0, crZone), true
}
