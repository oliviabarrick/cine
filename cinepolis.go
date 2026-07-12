package main

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Cinépolis Costa Rica is a WordPress + React SPA with no scrapable showtimes
// HTML. Two sources combine into a feed:
//
//   - CATALOG (public GET, no auth): /wp-json/mapi/v1/sites-data
//     the cinemas (site id → name) and the films now/soon showing (emid →
//     webName + sub/dub flags). It carries NO times.
//   - SHOWTIMES (BiggerPicture ticketing API): the actual sessions live behind
//     a POST-minted JWT. POST {apiBase}/sys/login mints a bearer token; then
//     GET {apiBase}/cus/eventMaster/{emid}/site/{siteId}/startDate/../endDate/..
//     (Authorization: Bearer <token>) returns that film's sessions at that
//     cinema over a date window in one call.
//
// There's no site→movie map and no bulk endpoint, so we fan out over every
// (cinema, film) pair — bounded by cinepolisConcurrency, over a cinepolisMaxDays
// window — the same shape as ccm.go.
const (
	cinepolisCatalogURL  = "https://cinepolis.co.cr/wp-json/mapi/v1/sites-data"
	cinepolisAPIBase     = "https://pub-api-use1.biggerpicture.ai/ecomAPI/public/api"
	cinepolisEnlineaBase = "https://enlinea.cinepolis.co.cr"
	// cinepolisLoginLang is the login `language`; the API accepts an es-cr /
	// es-mx / es-es locale and returns a bearer either way.
	cinepolisLoginLang = "es-cr"
)

// cinepolisMaxDays caps how many upcoming days of sessions we pull per
// (cinema, film). The sessions endpoint already only returns a handful of
// upcoming days in one call; this is a safety ceiling on the window we ask for.
const cinepolisMaxDays = 8

// cinepolisConcurrency bounds simultaneous (cinema, film) session fetches so a
// refresh stays gentle on the ticketing API.
const cinepolisConcurrency = 8

type cinepolis struct {
	catalogURL string
	apiBase    string
}

func newCinepolis() cinepolis {
	return cinepolis{catalogURL: cinepolisCatalogURL, apiBase: cinepolisAPIBase}
}

func (c cinepolis) Name() string { return "Cinépolis" }

// cinepolisCatalog is the slice of /sites-data we read: the real cinemas and the
// films, each keyed for the sessions fan-out.
type cinepolisCatalog struct {
	Sites  []cineSite  `json:"sites"`
	Movies []cineMovie `json:"movies"`
}

type cineSite struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// cineMovie carries the eventMasterId (emid) the sessions endpoint keys on, the
// clean display title, and the catalog-level sub/dub flags ("1"/"0") used only
// as a last-resort format fallback.
type cineMovie struct {
	EMID        int    `json:"emid"`
	WebName     string `json:"webName"`
	IsDubbed    string `json:"isDubbed"`
	IsSubtitled string `json:"isSubtitled"`
}

type cineLoginResp struct {
	Token string `json:"token"`
}

// cineSessionsResp is the ticketing API envelope; the sessions live in .list.
type cineSessionsResp struct {
	List []cineSession `json:"list"`
}

// cineSession is one screening. isSubtitled/isDubbed are the authoritative
// per-screening flags; eventAttributes is a JSON *string* of {id,name} tags
// (e.g. 3D / VIP / SUBTITLE) we mine for the screen format.
type cineSession struct {
	SiteID           int    `json:"siteId"`
	EventID          int    `json:"eventId"`
	EventMasterID    int    `json:"eventMasterId"`
	DateTimeOfEvent  string `json:"dateTimeOfEvent"`
	VenueDisplayName string `json:"venueDisplayName"`
	IsSubtitled      bool   `json:"isSubtitled"`
	IsDubbed         bool   `json:"isDubbed"`
	EventAttributes  string `json:"eventAttributes"`
}

func (c cinepolis) Fetch(ctx context.Context) ([]Showtime, error) {
	body, err := fetchPage(ctx, c.catalogURL)
	if err != nil {
		return nil, err
	}
	sites, movies, err := parseCinepolisCatalog(body)
	if err != nil {
		return nil, err
	}
	if len(sites) == 0 || len(movies) == 0 {
		return nil, fmt.Errorf("cinepolis: empty catalog (sites=%d movies=%d)", len(sites), len(movies))
	}

	// One token serves every cinema (verified cross-site), so mint it once.
	token, err := cinepolisLogin(ctx, c.apiBase, sites[0].ID)
	if err != nil {
		return nil, fmt.Errorf("cinepolis: login: %w", err)
	}
	authHeader := map[string]string{"Authorization": "Bearer " + token}

	now := time.Now().In(crZone)
	start := now.Format("2006-01-02")
	end := now.AddDate(0, 0, cinepolisMaxDays).Format("2006-01-02")

	var (
		mu       sync.Mutex
		out      []Showtime
		firstErr error
	)
	note := func(err error) {
		if err == nil {
			return
		}
		mu.Lock()
		if firstErr == nil {
			firstErr = err
		}
		mu.Unlock()
	}

	sem := make(chan struct{}, cinepolisConcurrency)
	var wg sync.WaitGroup
	for _, site := range sites {
		for _, movie := range movies {
			site, movie := site, movie
			wg.Add(1)
			sem <- struct{}{}
			go func() {
				defer wg.Done()
				defer func() { <-sem }()
				url := cinepolisSessionsURL(c.apiBase, movie.EMID, site.ID, start, end)
				b, err := fetchPageWithHeaders(ctx, url, authHeader)
				if err != nil {
					note(err)
					return
				}
				sts, err := parseCinepolisSessions(b, movie, site.Name)
				if err != nil {
					note(err)
					return
				}
				if len(sts) > 0 {
					mu.Lock()
					out = append(out, sts...)
					mu.Unlock()
				}
			}()
		}
	}
	wg.Wait()

	if len(out) == 0 {
		if firstErr != nil {
			return nil, fmt.Errorf("cinepolis: %w", firstErr)
		}
		return nil, fmt.Errorf("cinepolis: no showtimes parsed (API likely changed)")
	}
	return out, nil
}

// cinepolisLogin POSTs /sys/login and returns the bearer token the sessions
// endpoint requires.
func cinepolisLogin(ctx context.Context, apiBase string, siteID int) (string, error) {
	body, err := postJSON(ctx, apiBase+"/sys/login", map[string]any{
		"siteId":          siteID,
		"saleChannelCode": "web",
		"language":        cinepolisLoginLang,
	})
	if err != nil {
		return "", err
	}
	var r cineLoginResp
	if err := json.Unmarshal(body, &r); err != nil {
		return "", fmt.Errorf("decode login: %w", err)
	}
	if r.Token == "" {
		return "", fmt.Errorf("no token in login response")
	}
	return r.Token, nil
}

// parseCinepolisCatalog reads /sites-data into the real CR cinemas (dropping the
// synthetic id:0 "All" site) and the films to fan out over.
func parseCinepolisCatalog(body []byte) ([]cineSite, []cineMovie, error) {
	var cat cinepolisCatalog
	if err := json.Unmarshal(body, &cat); err != nil {
		return nil, nil, fmt.Errorf("cinepolis: decode catalog: %w", err)
	}
	var sites []cineSite
	for _, s := range cat.Sites {
		if s.ID == 0 || s.Name == "" { // id:0 is the aggregate "All", not a cinema
			continue
		}
		sites = append(sites, cineSite{ID: s.ID, Name: cleanText(s.Name)})
	}
	var movies []cineMovie
	for _, m := range cat.Movies {
		if m.EMID == 0 || m.WebName == "" {
			continue
		}
		movies = append(movies, m)
	}
	return sites, movies, nil
}

// parseCinepolisSessions turns one ticketing-API response into showtimes. movie
// supplies the clean title (the session's own name carries a format suffix) and
// the format fallback; cinemaName anchors each screening.
func parseCinepolisSessions(body []byte, movie cineMovie, cinemaName string) ([]Showtime, error) {
	var resp cineSessionsResp
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("cinepolis: decode sessions: %w", err)
	}
	title := cleanText(movie.WebName)
	var out []Showtime
	for _, s := range resp.List {
		start, ok := parseCinepolisTime(s.DateTimeOfEvent)
		if !ok {
			continue
		}
		out = append(out, Showtime{
			Chain:  "Cinépolis",
			Cinema: cinemaName,
			Movie:  title,
			Start:  start,
			Format: cinepolisFormat(s, movie),
			Screen: cinepolisScreen(s.EventAttributes),
			BuyURL: cinepolisBuyURL(s),
		})
	}
	return out, nil
}

// parseCinepolisTime reads a CR-local "2026-07-12T13:30:00" session timestamp
// (the feed carries no timezone offset).
func parseCinepolisTime(raw string) (time.Time, bool) {
	if t, err := time.ParseInLocation("2006-01-02T15:04:05", strings.TrimSpace(raw), crZone); err == nil {
		return t, true
	}
	return time.Time{}, false
}

// cinepolisFormat trusts the session's per-screening sub/dub booleans first,
// then the screen attributes, and only falls back to the movie's catalog flags
// when a screening claims neither or both.
func cinepolisFormat(s cineSession, movie cineMovie) Format {
	switch {
	case s.IsSubtitled && !s.IsDubbed:
		return FormatSubtitled
	case s.IsDubbed && !s.IsSubtitled:
		return FormatDubbed
	}
	if f := cinepolisAttrFormat(s.EventAttributes); f != FormatUnknown {
		return f
	}
	return cineMovieFormat(movie)
}

// cineMovieFormat maps the catalog "1"/"0" isSubtitled/isDubbed flags to a
// Format, used only when a screening's own flags are ambiguous.
func cineMovieFormat(m cineMovie) Format {
	sub, dub := m.IsSubtitled == "1", m.IsDubbed == "1"
	switch {
	case sub && !dub:
		return FormatSubtitled
	case dub && !sub:
		return FormatDubbed
	default:
		return FormatUnknown
	}
}

type cineAttr struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// cinepolisAttrs decodes the eventAttributes JSON *string* (e.g.
// `[{"id":1,"name":"3D"},{"id":2,"name":"DUB"}]`) into its tag list, tolerating
// an absent or malformed value.
func cinepolisAttrs(raw string) []cineAttr {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var attrs []cineAttr
	if err := json.Unmarshal([]byte(raw), &attrs); err != nil {
		return nil
	}
	return attrs
}

// cinepolisAttrFormat reads sub/dub out of the screen attributes.
func cinepolisAttrFormat(raw string) Format {
	for _, a := range cinepolisAttrs(raw) {
		switch strings.ToUpper(a.Name) {
		case "SUBTITLE", "SUBT", "SUB":
			return FormatSubtitled
		case "DUB", "DOB":
			return FormatDubbed
		}
	}
	return FormatUnknown
}

// cinepolisScreen joins the non-audio attribute tags (2D/3D/VIP/4DX/XE/…) into
// the Screen label, dropping the sub/dub tags that ParseFormat already covers.
func cinepolisScreen(raw string) string {
	var screen []string
	for _, a := range cinepolisAttrs(raw) {
		switch up := strings.ToUpper(a.Name); up {
		case "SUBTITLE", "SUBT", "SUB", "DUB", "DOB": // audio/subs, not a screen tag
		default:
			screen = append(screen, up)
		}
	}
	return strings.Join(screen, " ")
}

// cinepolisBuyURL deep-links into the enlinea checkout SPA at the cinema entry,
// carrying the event query params the app itself uses to open a screening.
func cinepolisBuyURL(s cineSession) string {
	return fmt.Sprintf("%s/site/%d?eventMasterId=%d&eventId=%d",
		cinepolisEnlineaBase, s.SiteID, s.EventMasterID, s.EventID)
}

func cinepolisSessionsURL(apiBase string, emid, siteID int, start, end string) string {
	return fmt.Sprintf("%s/cus/eventMaster/%d/site/%d/startDate/%s/endDate/%s",
		apiBase, emid, siteID, start, end)
}

// parseClock reads "18:30" or "6:30" into hour/minute. (Shared with other
// chains that publish a bare clock; kept here alongside cleanText.)
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
