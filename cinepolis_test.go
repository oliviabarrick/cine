package main

import (
	"os"
	"strings"
	"testing"
)

func TestParseCinepolisCatalog(t *testing.T) {
	body, err := os.ReadFile("testdata/cinepolis_sites.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	sites, movies, err := parseCinepolisCatalog(body)
	if err != nil {
		t.Fatalf("parseCinepolisCatalog: %v", err)
	}
	// The synthetic id:0 "All" site is dropped; only real cinemas remain.
	if len(sites) != 2 {
		t.Fatalf("got %d sites, want 2 (id:0 dropped)", len(sites))
	}
	byID := map[int]string{}
	for _, s := range sites {
		if s.ID == 0 {
			t.Errorf("site id:0 should have been dropped")
		}
		byID[s.ID] = s.Name
	}
	if byID[1625] != "Cinépolis Lincoln" || byID[1627] != "Cinépolis TerraMall" {
		t.Errorf("site names = %+v", byID)
	}
	if len(movies) != 2 {
		t.Fatalf("got %d movies, want 2", len(movies))
	}
}

func TestParseCinepolisSessionsDubbed(t *testing.T) {
	body, err := os.ReadFile("testdata/cinepolis_sessions_dub.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	movie := cineMovie{EMID: 2175, WebName: "Moana", IsDubbed: "1", IsSubtitled: "0"}
	st, err := parseCinepolisSessions(body, movie, "Cinépolis Lincoln")
	if err != nil {
		t.Fatalf("parseCinepolisSessions: %v", err)
	}
	if len(st) != 3 {
		t.Fatalf("got %d showtimes, want 3", len(st))
	}
	s := st[0]
	if s.Chain != "Cinépolis" || s.Cinema != "Cinépolis Lincoln" || s.Movie != "Moana" {
		t.Errorf("showtime = %+v", s)
	}
	if s.Format != FormatDubbed {
		t.Errorf("format = %q, want dub", s.Format)
	}
	// eventAttributes "3D"+"DUB": the audio tag is dropped, the screen kept.
	if s.Screen != "3D" {
		t.Errorf("screen = %q, want 3D", s.Screen)
	}
	if got := s.Start.Format("2006-01-02T15:04"); got != "2026-07-12T13:30" {
		t.Errorf("start = %q, want 2026-07-12T13:30", got)
	}
	if !strings.Contains(s.BuyURL, "/site/1625") || !strings.Contains(s.BuyURL, "eventId=53802") {
		t.Errorf("buyURL = %q", s.BuyURL)
	}
}

func TestParseCinepolisSessionsSubtitled(t *testing.T) {
	body, err := os.ReadFile("testdata/cinepolis_sessions_sub.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	movie := cineMovie{EMID: 2194, WebName: "Backrooms: Sin Salida", IsDubbed: "0", IsSubtitled: "1"}
	st, err := parseCinepolisSessions(body, movie, "Cinépolis TerraMall")
	if err != nil {
		t.Fatalf("parseCinepolisSessions: %v", err)
	}
	if len(st) == 0 {
		t.Fatal("got no showtimes")
	}
	s := st[0]
	if s.Movie != "Backrooms: Sin Salida" || s.Cinema != "Cinépolis TerraMall" {
		t.Errorf("showtime = %+v", s)
	}
	if s.Format != FormatSubtitled {
		t.Errorf("format = %q, want sub", s.Format)
	}
	// eventAttributes "2D"+"SUBTITLE": SUBTITLE dropped from the screen label.
	if s.Screen != "2D" {
		t.Errorf("screen = %q, want 2D", s.Screen)
	}
}

func TestCinepolisFormatPrefersSessionFlags(t *testing.T) {
	dubMovie := cineMovie{IsDubbed: "1", IsSubtitled: "0"}
	// Session flags win over the movie's catalog flags.
	sub := cineSession{IsSubtitled: true, EventAttributes: `[{"id":15,"name":"SUBTITLE"}]`}
	if got := cinepolisFormat(sub, dubMovie); got != FormatSubtitled {
		t.Errorf("session-subtitled = %q, want sub", got)
	}
	// Ambiguous session flags fall back to attributes.
	amb := cineSession{EventAttributes: `[{"id":2,"name":"DUB"}]`}
	if got := cinepolisFormat(amb, cineMovie{}); got != FormatDubbed {
		t.Errorf("attr fallback = %q, want dub", got)
	}
	// No session flags and no attribute hints: fall back to the catalog flags.
	none := cineSession{EventAttributes: `[{"id":1,"name":"3D"}]`}
	if got := cinepolisFormat(none, dubMovie); got != FormatDubbed {
		t.Errorf("movie fallback = %q, want dub", got)
	}
}
