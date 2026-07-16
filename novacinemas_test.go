package main

import (
	"os"
	"strings"
	"testing"
)

func TestParseNovaCinemas(t *testing.T) {
	body, err := os.ReadFile("testdata/nova_cinemas.html")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	cinemas, err := parseNovaCinemas(string(body))
	if err != nil {
		t.Fatalf("parseNovaCinemas: %v", err)
	}
	if len(cinemas) != 2 {
		t.Fatalf("got %d cinemas, want 2", len(cinemas))
	}
	if cinemas[0].ID != "9999" || cinemas[0].Name != "Nova Cinemas" {
		t.Errorf("cinema[0] = %+v", cinemas[0])
	}
	if cinemas[1].ID != "9995" || cinemas[1].Name != "Nova Cinemas Limón" {
		t.Errorf("cinema[1] = %+v", cinemas[1])
	}
}

func TestParseNovaShowtimes(t *testing.T) {
	body, err := os.ReadFile("testdata/nova_showtimes.html")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	st := parseNovaShowtimes(string(body), "Nova Cinemas", novaBaseURL)
	if len(st) != 3 {
		t.Fatalf("got %d showtimes, want 3", len(st))
	}

	imax := st[0]
	if imax.Chain != "Nova Cinemas" || imax.Cinema != "Nova Cinemas" || imax.Movie != "LA ODISEA" {
		t.Errorf("imax = %+v", imax)
	}
	if imax.Format != FormatSubtitled || imax.Screen != "3D IMAX" {
		t.Errorf("format/screen = %q/%q, want sub/3D IMAX", imax.Format, imax.Screen)
	}
	if got := imax.Start.Format("2006-01-02T15:04"); got != "2026-07-17T20:30" {
		t.Errorf("start = %q", got)
	}
	if !strings.Contains(imax.BuyURL, "cinemacode=9999&txtSessionId=82637") {
		t.Errorf("buyURL = %q", imax.BuyURL)
	}

	if st[1].Format != FormatDubbed || st[1].Screen != "2D" {
		t.Errorf("dub format/screen = %q/%q", st[1].Format, st[1].Screen)
	}
	if st[2].Format != FormatDubbed || st[2].Screen != "VIP" {
		t.Errorf("Spanish format/screen = %q/%q", st[2].Format, st[2].Screen)
	}
}

func TestParseNovaCinemasEmptyErrors(t *testing.T) {
	if _, err := parseNovaCinemas("<html><body>nada</body></html>"); err == nil {
		t.Fatal("expected error for unparseable page, got nil")
	}
}
