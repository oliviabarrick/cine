package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// Cinemark Central America is a Vue SPA over a Vista cinema API. The listing is
// served by two clean, unauthenticated JSON endpoints under api.cinemarkca.com:
//
//   - /api/vista/data/theatres
//     every Central-America cinema, grouped by city; Costa Rica ones are
//     identified by a "costa-rica-" Slug prefix.
//   - /api/vista/data/billboard?cinema_id=<ID>
//     one cinema's full schedule (all upcoming days in a single response),
//     nested date → movie → movie_version → session.
//
// Sub/dub isn't a field: it's encoded in the parenthesized suffix of each
// movie_version title, e.g. "Moana Live Action (2D SUB DBOX -CR)".
const (
	cinemarkAPIBase      = "https://api.cinemarkca.com/api/vista/data"
	cinemarkPurchaseBase = "https://www.cinemarkca.com/costa-rica/purchase"
	cinemarkCRSlugPrefix = "costa-rica-"
)

type cinemark struct{ apiBase string }

func newCinemark() cinemark { return cinemark{apiBase: cinemarkAPIBase} }

func (c cinemark) Name() string { return "Cinemark" }

type cinemarkTheatreGroup struct {
	Cinemas []cinemarkTheatre `json:"cinemas"`
}

type cinemarkTheatre struct {
	ID   string `json:"ID"`
	Name string `json:"Name"`
	Slug string `json:"Slug"`
}

type cinemarkDay struct {
	Movies []cinemarkMovie `json:"movies"`
}

type cinemarkMovie struct {
	Title    string            `json:"title"`
	Versions []cinemarkVersion `json:"movie_versions"`
}

type cinemarkVersion struct {
	FilmHOPK string            `json:"film_HOPK"`
	Title    string            `json:"title"`
	Sessions []cinemarkSession `json:"sessions"`
}

type cinemarkSession struct {
	ID       string `json:"id"`
	Showtime string `json:"showtime"`
	Day      string `json:"day"`
	Hour     string `json:"hour"`
}

func (c cinemark) Fetch(ctx context.Context) ([]Showtime, error) {
	body, err := fetchPage(ctx, c.apiBase+"/theatres")
	if err != nil {
		return nil, err
	}
	cinemas, err := parseCinemarkCRCinemas(body)
	if err != nil {
		return nil, err
	}
	if len(cinemas) == 0 {
		return nil, fmt.Errorf("cinemark: no Costa Rica cinemas in theatres feed")
	}

	var (
		out      []Showtime
		firstErr error
	)
	for _, cin := range cinemas {
		b, err := fetchPage(ctx, cinemarkBillboardURL(c.apiBase, cin.ID))
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		sts, err := parseCinemarkBillboard(b, cin.Name, cin.ID)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		out = append(out, sts...)
	}
	if len(out) == 0 {
		if firstErr != nil {
			return nil, fmt.Errorf("cinemark: %w", firstErr)
		}
		return nil, fmt.Errorf("cinemark: no showtimes parsed (API likely changed)")
	}
	return out, nil
}

// parseCinemarkCRCinemas keeps only the Costa Rica cinemas and trims the
// "Costa Rica - " prefix the feed puts on every name.
func parseCinemarkCRCinemas(body []byte) ([]cinemarkTheatre, error) {
	var groups []cinemarkTheatreGroup
	if err := json.Unmarshal(body, &groups); err != nil {
		return nil, fmt.Errorf("cinemark: decode theatres: %w", err)
	}
	var out []cinemarkTheatre
	for _, g := range groups {
		for _, t := range g.Cinemas {
			if !strings.HasPrefix(t.Slug, cinemarkCRSlugPrefix) || t.ID == "" {
				continue
			}
			t.Name = cleanText(strings.TrimPrefix(t.Name, "Costa Rica - "))
			out = append(out, t)
		}
	}
	return out, nil
}

// parseCinemarkBillboard flattens the date→movie→version→session tree into
// showtimes for one cinema.
func parseCinemarkBillboard(body []byte, cinemaName, cinemaID string) ([]Showtime, error) {
	var days []cinemarkDay
	if err := json.Unmarshal(body, &days); err != nil {
		return nil, fmt.Errorf("cinemark: decode billboard: %w", err)
	}
	var out []Showtime
	for _, d := range days {
		for _, m := range d.Movies {
			movie := cleanText(m.Title)
			if movie == "" {
				continue
			}
			for _, v := range m.Versions {
				format, screen := cinemarkFormatScreen(v.Title)
				for _, s := range v.Sessions {
					start, ok := parseCinemarkTime(s.Showtime)
					if !ok {
						continue
					}
					out = append(out, Showtime{
						Chain:  "Cinemark",
						Cinema: cinemaName,
						Movie:  movie,
						Start:  start,
						Format: format,
						Screen: screen,
						BuyURL: cinemarkBuyURL(cinemaID, v.FilmHOPK, s),
					})
				}
			}
		}
	}
	return out, nil
}

// reCinemarkVersion captures the parenthesized tag list of a version title, e.g.
// the "2D SUB DBOX -CR" in "Moana Live Action (2D SUB DBOX -CR)".
var reCinemarkVersion = regexp.MustCompile(`\(([^)]*)\)[^)]*$`)

// cinemarkFormatScreen reads sub/dub and the screen format out of a version
// title's tag list. SUB→subtitled, DOB→dubbed; other tags (2D/3D/DBOX/XD) become
// the Screen; the "-CR" country tag is dropped.
func cinemarkFormatScreen(versionTitle string) (Format, string) {
	format := FormatUnknown
	var screen []string
	for _, tok := range strings.Fields(firstSubmatch(reCinemarkVersion, versionTitle)) {
		switch up := strings.ToUpper(tok); {
		case up == "SUB" || up == "SUBT":
			format = FormatSubtitled
		case up == "DOB" || up == "DUB":
			format = FormatDubbed
		case strings.HasPrefix(tok, "-"): // country tag, e.g. -CR
		default:
			screen = append(screen, up)
		}
	}
	if format == FormatUnknown {
		format = ParseFormat(versionTitle)
	}
	return format, strings.Join(screen, " ")
}

// parseCinemarkTime reads a CR-local "2026-07-11T21:25:00" session timestamp
// (the feed carries no timezone offset).
func parseCinemarkTime(raw string) (time.Time, bool) {
	if t, err := time.ParseInLocation("2006-01-02T15:04:05", strings.TrimSpace(raw), crZone); err == nil {
		return t, true
	}
	return time.Time{}, false
}

func cinemarkBillboardURL(apiBase, cinemaID string) string {
	return fmt.Sprintf("%s/billboard?cinema_id=%s", apiBase, url.QueryEscape(cinemaID))
}

// cinemarkBuyURL rebuilds the deep link the SPA constructs client-side.
func cinemarkBuyURL(cinemaID, filmHOPK string, s cinemarkSession) string {
	q := url.Values{
		"tag":      {cinemaID},
		"movie_id": {filmHOPK},
		"showtime": {s.ID},
		"date":     {s.Day},
		"hour":     {s.Hour},
	}
	return cinemarkPurchaseBase + "?" + q.Encode()
}
