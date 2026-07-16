package main

import (
	"context"
	"fmt"
	"html"
	"regexp"
	"strings"
	"time"
)

// Nova Cinemas uses Vista's server-rendered ticketing site. The cinema list
// discovers every Costa Rica location; each cinema detail page carries its full
// schedule, with per-session attributes such as IMAX, 3D, VIP, SUBTITULADA, and
// DOBLADA plus a direct ticket link.
const novaBaseURL = "https://web-ticketing.novacinemas.cr"

type novaCinemas struct{ baseURL string }

func newNovaCinemas() novaCinemas { return novaCinemas{baseURL: novaBaseURL} }

func (n novaCinemas) Name() string { return "Nova Cinemas" }

type novaCinema struct {
	ID   string
	Name string
}

func (n novaCinemas) Fetch(ctx context.Context) ([]Showtime, error) {
	body, err := fetchPage(ctx, n.baseURL+"/Browsing/Cinemas")
	if err != nil {
		return nil, err
	}
	cinemas, err := parseNovaCinemas(string(body))
	if err != nil {
		return nil, err
	}

	var (
		out      []Showtime
		firstErr error
	)
	for _, cinema := range cinemas {
		b, err := fetchPage(ctx, novaCinemaURL(n.baseURL, cinema.ID))
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		out = append(out, parseNovaShowtimes(string(b), cinema.Name, n.baseURL)...)
	}
	if len(out) == 0 {
		if firstErr != nil {
			return nil, fmt.Errorf("nova cinemas: %w", firstErr)
		}
		return nil, fmt.Errorf("nova cinemas: no showtimes parsed (page markup likely changed)")
	}
	return out, nil
}

const (
	novaCinemaMarker = `<a class="cinema-title"`
	novaFilmMarker   = `<div class="film-item`
)

var (
	reNovaCinemaID  = regexp.MustCompile(`(?i)/Browsing/Cinemas/Details/([0-9]+)`)
	reNovaItemTitle = regexp.MustCompile(`(?is)<h3[^>]*class="item-title"[^>]*>(.*?)</h3>`)
	reNovaFilmTitle = regexp.MustCompile(`(?is)<h3[^>]*class="film-title"[^>]*>(.*?)</h3>`)
	reNovaSession   = regexp.MustCompile(`(?is)<a\s+href="([^"]*visSelectTickets\.aspx[^"]*)"[^>]*class="session-time[^"]*"[^>]*>(.*?)</a>`)
	reNovaDateTime  = regexp.MustCompile(`(?is)<time[^>]*datetime="([^"]+)"`)
	reNovaAttribute = regexp.MustCompile(`(?is)<img[^>]*alt="([^"]*)"[^>]*>`)
)

func parseNovaCinemas(body string) ([]novaCinema, error) {
	var out []novaCinema
	for _, block := range splitOnMarker(body, novaCinemaMarker) {
		id := firstSubmatch(reNovaCinemaID, block)
		name := cleanText(firstSubmatch(reNovaItemTitle, block))
		if id != "" && name != "" {
			out = append(out, novaCinema{ID: id, Name: name})
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("nova cinemas: no cinemas parsed (page markup likely changed)")
	}
	return out, nil
}

// parseNovaShowtimes flattens one cinema's film -> session HTML into the shared
// model. A cinema with no scheduled sessions legitimately returns an empty list;
// Fetch only reports a markup failure when every discovered cinema is empty.
func parseNovaShowtimes(body, cinemaName, baseURL string) []Showtime {
	var out []Showtime
	for _, film := range splitOnMarker(body, novaFilmMarker) {
		movie := cleanText(firstSubmatch(reNovaFilmTitle, film))
		if movie == "" {
			continue
		}
		for _, match := range reNovaSession.FindAllStringSubmatch(film, -1) {
			start, ok := parseNovaTime(firstSubmatch(reNovaDateTime, match[2]))
			if !ok {
				continue
			}
			format, screen := novaFormatScreen(match[2])
			out = append(out, Showtime{
				Chain:  "Nova Cinemas",
				Cinema: cinemaName,
				Movie:  movie,
				Start:  start,
				Format: format,
				Screen: screen,
				BuyURL: novaAbsoluteURL(baseURL, match[1]),
			})
		}
	}
	return out
}

func parseNovaTime(raw string) (time.Time, bool) {
	raw = strings.TrimSpace(html.UnescapeString(raw))
	if t, err := time.ParseInLocation("2006-01-02T15:04:05", raw, crZone); err == nil {
		return t, true
	}
	return time.Time{}, false
}

// novaFormatScreen separates the audio/subtitle attribute from presentation
// attributes. IMAX remains in Screen and therefore reaches the API and UI.
func novaFormatScreen(sessionHTML string) (Format, string) {
	format := FormatUnknown
	var screen []string
	for _, match := range reNovaAttribute.FindAllStringSubmatch(sessionHTML, -1) {
		attribute := cleanText(match[1])
		if f := ParseFormat(attribute); f != FormatUnknown {
			if format == FormatUnknown {
				format = f
			}
			continue
		}
		if attribute != "" {
			screen = append(screen, strings.ToUpper(attribute))
		}
	}
	return format, strings.Join(screen, " ")
}

func novaCinemaURL(baseURL, id string) string {
	return strings.TrimRight(baseURL, "/") + "/Browsing/Cinemas/Details/" + id
}

func novaAbsoluteURL(baseURL, href string) string {
	href = html.UnescapeString(strings.TrimSpace(href))
	switch {
	case strings.HasPrefix(href, "//"):
		return "https:" + href
	case strings.HasPrefix(href, "/"):
		return strings.TrimRight(baseURL, "/") + href
	default:
		return href
	}
}
