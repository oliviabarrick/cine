package main

import (
	"os"
	"testing"
	"time"
)

func TestParseCinepolis(t *testing.T) {
	body, err := os.ReadFile("testdata/cinepolis.html")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	now := time.Date(2026, 7, 12, 12, 0, 0, 0, crZone)

	st, err := parseCinepolis(body, now)
	if err != nil {
		t.Fatalf("parseCinepolis: %v", err)
	}
	if len(st) != 4 {
		t.Fatalf("got %d showtimes, want 4", len(st))
	}

	// First (sorted only later; parse preserves document order): Dune subtitled.
	first := st[0]
	if first.Movie != "Dune: Parte Dos" {
		t.Errorf("movie = %q, want Dune: Parte Dos", first.Movie)
	}
	if first.Format != FormatSubtitled {
		t.Errorf("format = %q, want sub", first.Format)
	}
	if first.Language != "Inglés" {
		t.Errorf("language = %q, want Inglés", first.Language)
	}
	if first.Cinema != "Cinépolis San Pedro" {
		t.Errorf("cinema = %q", first.Cinema)
	}
	if got := first.Start.Format("2006-01-02T15:04"); got != "2026-07-12T18:30" {
		t.Errorf("start = %q, want 2026-07-12T18:30", got)
	}
	if first.BuyURL != "/comprar/1" {
		t.Errorf("buyURL = %q", first.BuyURL)
	}

	// The dubbed Dune function must be picked up as dubbed.
	var sawDubbed bool
	for _, s := range st {
		if s.Movie == "Dune: Parte Dos" && s.Format == FormatDubbed {
			sawDubbed = true
		}
	}
	if !sawDubbed {
		t.Error("expected a dubbed Dune function")
	}
}

func TestParseCinepolisEmptyErrors(t *testing.T) {
	// A page whose DOM no longer matches must error, not cache an empty chain.
	if _, err := parseCinepolis([]byte("<html><body>nada</body></html>"), time.Now()); err == nil {
		t.Fatal("expected error for unparseable page, got nil")
	}
}

func TestParseCineTime(t *testing.T) {
	now := time.Date(2026, 7, 12, 9, 0, 0, 0, crZone)
	if _, ok := parseCineTime("garbage", now); ok {
		t.Error("garbage parsed as time")
	}
	// Bare clock pins to now's CR date.
	tm, ok := parseCineTime("18:45", now)
	if !ok || tm.Format("2006-01-02T15:04") != "2026-07-12T18:45" {
		t.Errorf("clock parse = %v ok=%v", tm, ok)
	}
}
