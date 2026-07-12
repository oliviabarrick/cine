package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
)

// CCM (Costa Rica Multicines, ccmcinemas.com) is a small three-cinema chain on a
// plain ASP.NET backend that exposes JSON endpoints directly — no SPA shell to
// render. Its browse UI fans out over three of them, and so do we:
//
//   - /Funciones/GetPeliculasPorComplejo?codComplejo=ID
//     one sample screening per movie now showing at a cinema; we use it only to
//     learn which movies (codPelicula → título) are playing there.
//   - /Peliculas/GetFechasDisponibles?codComplejo=ID&codPelicula=P
//     the dates that movie screens at that cinema (ISO timestamps).
//   - /Peliculas/GetCacheFuncionesComplejoPeliculaFecha?complejoId=ID&codPelicula=P&fecha=YYYY-MM-DD
//     the actual screenings for one movie/cinema/day — each carries an accurate
//     subtitulada/doblada flag, the published start time, and the cinema name.
//
// There is no bulk "all showtimes" endpoint, so we bound the fan-out with
// ccmMaxDays and cap concurrency to stay gentle on CCM's small origin.
const ccmBase = "https://www.ccmcinemas.com"

// ccmMaxDays caps how many upcoming days of screenings we pull per movie so a
// refresh can't balloon into hundreds of requests. GetFechasDisponibles already
// only returns a handful of upcoming dates; this is a safety ceiling.
const ccmMaxDays = 6

// ccmComplejos are CCM's cinema ids (codComplejo): Plaza Mayor, San Ramón, San
// Carlos. The display name comes from each response's nombreComplejo, not here.
var ccmComplejos = []int{401, 402, 403}

// ccmConcurrency bounds simultaneous per-movie fetches across all cinemas.
const ccmConcurrency = 8

type ccm struct{ base string }

func newCCM() ccm { return ccm{base: ccmBase} }

func (c ccm) Name() string { return "CCM" }

// ccmMovieFunc is a row from GetPeliculasPorComplejo; we read only the nested
// movie id/title (the rest is seat counts, geo, trailers we don't need).
type ccmMovieFunc struct {
	CachePeliculas struct {
		CodPelicula int    `json:"codPelicula"`
		Titulo      string `json:"titulo"`
	} `json:"cachePeliculas"`
}

// ccmFuncGroup is one technology/idioma bucket from GetCacheFunciones; the
// per-screening detail (and the accurate sub/dub flags) live in Funciones.
type ccmFuncGroup struct {
	TecnologiaNombre string    `json:"tecnologiaNombre"`
	Funciones        []ccmFunc `json:"funciones"`
	NombreComplejo   string    `json:"nombreComplejo"`
}

type ccmFunc struct {
	HoraComienzoOriginal string `json:"horaComienzoOriginal"`
	Subtitulada          bool   `json:"subtitulada"`
	Doblada              bool   `json:"doblada"`
	CodFuncion           int    `json:"codFuncion"`
	CodTecnologia        int    `json:"codTecnologia"`
	CodPelicula          int    `json:"codPelicula"`
	IdiomaPel            string `json:"idiomaPel"`
	NomTipoSala          string `json:"nomTipoSala"`
}

func (c ccm) Fetch(ctx context.Context) ([]Showtime, error) {
	now := time.Now().In(crZone)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, crZone)
	horizon := today.AddDate(0, 0, ccmMaxDays)

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

	sem := make(chan struct{}, ccmConcurrency)
	var wg sync.WaitGroup

	for _, complejo := range ccmComplejos {
		body, err := fetchPage(ctx, ccmMoviesURL(c.base, complejo))
		if err != nil {
			note(err)
			continue
		}
		titles, err := parseCCMTitles(body)
		if err != nil {
			note(err)
			continue
		}
		for cod := range titles {
			cod, complejo, titles := cod, complejo, titles
			wg.Add(1)
			sem <- struct{}{}
			go func() {
				defer wg.Done()
				defer func() { <-sem }()
				sts, err := c.fetchMovie(ctx, complejo, cod, titles, today, horizon)
				note(err)
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
			return nil, fmt.Errorf("ccm: %w", firstErr)
		}
		return nil, fmt.Errorf("ccm: no showtimes parsed (endpoints likely changed)")
	}
	return out, nil
}

// fetchMovie pulls one movie's available dates at a cinema and then its
// screenings for each date inside [today, horizon).
func (c ccm) fetchMovie(ctx context.Context, complejo, cod int, titles map[int]string, today, horizon time.Time) ([]Showtime, error) {
	var dates []string
	if err := ccmGetJSON(ctx, ccmDatesURL(c.base, complejo, cod), &dates); err != nil {
		return nil, err
	}
	var (
		out      []Showtime
		firstErr error
	)
	for _, raw := range dates {
		day, ok := parseCCMDate(raw)
		if !ok || day.Before(today) || !day.Before(horizon) {
			continue
		}
		body, err := fetchPage(ctx, ccmFuncURL(c.base, complejo, cod, day))
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		sts, err := parseCCMFunctions(body, titles, complejo, day)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		out = append(out, sts...)
	}
	if len(out) == 0 && firstErr != nil {
		return nil, firstErr
	}
	return out, nil
}

// parseCCMTitles reads a GetPeliculasPorComplejo payload into a codPelicula →
// title map (the endpoint lists one sample screening per movie now showing).
func parseCCMTitles(body []byte) (map[int]string, error) {
	var movies []ccmMovieFunc
	if err := json.Unmarshal(body, &movies); err != nil {
		return nil, fmt.Errorf("ccm: decode movies: %w", err)
	}
	titles := map[int]string{}
	for _, m := range movies {
		if m.CachePeliculas.CodPelicula != 0 && m.CachePeliculas.Titulo != "" {
			titles[m.CachePeliculas.CodPelicula] = cleanText(m.CachePeliculas.Titulo)
		}
	}
	return titles, nil
}

// parseCCMFunctions turns a GetCacheFunciones payload into showtimes. titles maps
// codPelicula → movie title (from GetPeliculasPorComplejo); complejo/day anchor
// each screening and build its buy link.
func parseCCMFunctions(body []byte, titles map[int]string, complejo int, day time.Time) ([]Showtime, error) {
	var groups []ccmFuncGroup
	if err := json.Unmarshal(body, &groups); err != nil {
		return nil, fmt.Errorf("ccm: decode functions: %w", err)
	}
	var out []Showtime
	for _, g := range groups {
		for _, f := range g.Funciones {
			hh, mm, ok := parseClock(f.HoraComienzoOriginal)
			if !ok {
				continue
			}
			title := titles[f.CodPelicula]
			if title == "" {
				continue
			}
			start := time.Date(day.Year(), day.Month(), day.Day(), hh, mm, 0, 0, crZone)
			out = append(out, Showtime{
				Chain:    "CCM",
				Cinema:   cleanText(g.NombreComplejo),
				Movie:    title,
				Start:    start,
				Format:   ccmFormat(f.Subtitulada, f.Doblada, f.IdiomaPel),
				Language: cleanText(f.IdiomaPel),
				Screen:   cleanText(g.TecnologiaNombre),
				BuyURL:   ccmBuyURL(complejo, day, f),
			})
		}
	}
	return out, nil
}

// ccmFormat prefers CCM's explicit per-screening flags; only when they're
// absent or contradictory does it fall back to the language label.
func ccmFormat(sub, dob bool, idioma string) Format {
	switch {
	case sub && !dob:
		return FormatSubtitled
	case dob && !sub:
		return FormatDubbed
	default:
		return ParseFormat(idioma)
	}
}

// ccmBuyURL mirrors the site's /carrito deep link:
// df=complejo-YYYYMMDD-codPelicula-codFuncion-codTecnologia.
func ccmBuyURL(complejo int, day time.Time, f ccmFunc) string {
	df := fmt.Sprintf("%d-%s-%d-%d-%d", complejo, day.Format("20060102"), f.CodPelicula, f.CodFuncion, f.CodTecnologia)
	return ccmBase + "/carrito?df=" + df
}

// parseCCMDate reads GetFechasDisponibles' "2026-07-12T00:00:00" into a CR-local
// midnight.
func parseCCMDate(raw string) (time.Time, bool) {
	raw = strings.TrimSpace(raw)
	for _, layout := range []string{"2006-01-02T15:04:05", "2006-01-02T15:04:05.999", "2006-01-02"} {
		if t, err := time.ParseInLocation(layout, raw, crZone); err == nil {
			return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, crZone), true
		}
	}
	return time.Time{}, false
}

func ccmMoviesURL(base string, complejo int) string {
	return fmt.Sprintf("%s/Funciones/GetPeliculasPorComplejo?codComplejo=%d", base, complejo)
}

func ccmDatesURL(base string, complejo, cod int) string {
	return fmt.Sprintf("%s/Peliculas/GetFechasDisponibles?codComplejo=%d&codPelicula=%d", base, complejo, cod)
}

func ccmFuncURL(base string, complejo, cod int, day time.Time) string {
	return fmt.Sprintf("%s/Peliculas/GetCacheFuncionesComplejoPeliculaFecha?complejoId=%d&codPelicula=%d&fecha=%s",
		base, complejo, cod, day.Format("2006-01-02"))
}

// ccmGetJSON fetches url and unmarshals the JSON body into v.
func ccmGetJSON(ctx context.Context, url string, v any) error {
	body, err := fetchPage(ctx, url)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, v)
}
