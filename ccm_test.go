package main

import (
	"os"
	"testing"
	"time"
)

func TestParseCCMTitles(t *testing.T) {
	body, err := os.ReadFile("testdata/ccm_movies.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	titles, err := parseCCMTitles(body)
	if err != nil {
		t.Fatalf("parseCCMTitles: %v", err)
	}
	if got := titles[24]; got != "Scary Movie 6" {
		t.Errorf("titles[24] = %q, want Scary Movie 6", got)
	}
	if got := titles[27]; got != "Toy Story 5" {
		t.Errorf("titles[27] = %q, want Toy Story 5", got)
	}
}

func TestParseCCMFunctions(t *testing.T) {
	body, err := os.ReadFile("testdata/ccm_functions.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	titles := map[int]string{24: "Scary Movie 6"}
	day := time.Date(2026, 7, 12, 0, 0, 0, 0, crZone)

	st, err := parseCCMFunctions(body, titles, 402, day)
	if err != nil {
		t.Fatalf("parseCCMFunctions: %v", err)
	}
	if len(st) != 2 {
		t.Fatalf("got %d showtimes, want 2", len(st))
	}

	// First: a dubbed screening at 16:00.
	dub := st[0]
	if dub.Movie != "Scary Movie 6" {
		t.Errorf("movie = %q", dub.Movie)
	}
	if dub.Chain != "CCM" || dub.Cinema != "San Ramon" {
		t.Errorf("chain/cinema = %q/%q", dub.Chain, dub.Cinema)
	}
	if dub.Format != FormatDubbed {
		t.Errorf("format = %q, want dub", dub.Format)
	}
	if got := dub.Start.Format("2006-01-02T15:04"); got != "2026-07-12T16:00" {
		t.Errorf("start = %q, want 2026-07-12T16:00", got)
	}
	want := ccmBase + "/carrito?df=402-20260712-24-1582-3"
	if dub.BuyURL != want {
		t.Errorf("buyURL = %q, want %q", dub.BuyURL, want)
	}

	// Second: the same film subtitled at 19:15 — the per-function flags must win
	// over the movie-level subtitulada, so both formats are represented.
	sub := st[1]
	if sub.Format != FormatSubtitled {
		t.Errorf("format = %q, want sub", sub.Format)
	}
	if sub.Language != "Inglés" {
		t.Errorf("language = %q, want Inglés", sub.Language)
	}
}

func TestCCMFormat(t *testing.T) {
	cases := []struct {
		sub, dob bool
		idioma   string
		want     Format
	}{
		{true, false, "Inglés", FormatSubtitled},
		{false, true, "Español", FormatDubbed},
		{false, false, "Español", FormatDubbed}, // fall back to language
		{false, false, "Inglés", FormatUnknown},
	}
	for _, c := range cases {
		if got := ccmFormat(c.sub, c.dob, c.idioma); got != c.want {
			t.Errorf("ccmFormat(%v,%v,%q) = %q, want %q", c.sub, c.dob, c.idioma, got, c.want)
		}
	}
}

func TestParseCCMDate(t *testing.T) {
	tm, ok := parseCCMDate("2026-07-12T00:00:00")
	if !ok || tm.Format("2006-01-02") != "2026-07-12" {
		t.Errorf("parseCCMDate = %v ok=%v", tm, ok)
	}
	if _, ok := parseCCMDate("garbage"); ok {
		t.Error("garbage parsed as date")
	}
}
