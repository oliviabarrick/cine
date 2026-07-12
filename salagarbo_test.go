package main

import (
	"os"
	"testing"
)

func TestParseSalaGarbo(t *testing.T) {
	body, err := os.ReadFile("testdata/salagarbo.html")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	st, err := parseSalaGarbo(string(body))
	if err != nil {
		t.Fatalf("parseSalaGarbo: %v", err)
	}
	if len(st) != 3 {
		t.Fatalf("got %d showtimes, want 3", len(st))
	}

	// Single-film event: the <h2> is the movie, no per-screening name.
	giant := st[0]
	if giant.Movie != "Domingos Épicos presenta: Giant (1956)" {
		t.Errorf("movie = %q", giant.Movie)
	}
	if giant.Chain != "Sala Garbo" || giant.Cinema != "Sala Garbo" {
		t.Errorf("chain/cinema = %q/%q", giant.Chain, giant.Cinema)
	}
	if giant.Format != FormatSubtitled {
		t.Errorf("format = %q, want sub (Sala Garbo default)", giant.Format)
	}
	if got := giant.Start.Format("2006-01-02T15:04"); got != "2026-07-12T15:00" {
		t.Errorf("start = %q, want 2026-07-12T15:00", got)
	}
	if giant.BuyURL != "https://salagarbo.com/product/domingos-epicos-presenta-giant-1956/" {
		t.Errorf("buyURL = %q", giant.BuyURL)
	}

	// Film-club series: the per-screening film name (event_additional_name) wins
	// over the series <h2>.
	if st[1].Movie != "Bloodsport" {
		t.Errorf("movie = %q, want Bloodsport", st[1].Movie)
	}
	if got := st[1].Start.Format("2006-01-02T15:04"); got != "2026-07-16T19:30" {
		t.Errorf("start = %q, want 2026-07-16T19:30", got)
	}
	if st[2].Movie != "The Last Starfighter" {
		t.Errorf("movie = %q, want The Last Starfighter", st[2].Movie)
	}
}

func TestParseSalaGarboEmptyErrors(t *testing.T) {
	if _, err := parseSalaGarbo("<html><body>nada</body></html>"); err == nil {
		t.Fatal("expected error for unparseable page, got nil")
	}
}

func TestParseSGDateTime(t *testing.T) {
	tm, ok := parseSGDateTime("12", "Jul", "2026", "15", "00")
	if !ok || tm.Format("2006-01-02T15:04") != "2026-07-12T15:00" {
		t.Errorf("parseSGDateTime = %v ok=%v", tm, ok)
	}
	if _, ok := parseSGDateTime("12", "Xyz", "2026", "15", "00"); ok {
		t.Error("bogus month parsed")
	}
}
