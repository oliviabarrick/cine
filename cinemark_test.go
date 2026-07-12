package main

import (
	"os"
	"strings"
	"testing"
)

func TestParseCinemarkCRCinemas(t *testing.T) {
	body, err := os.ReadFile("testdata/cinemark_theatres.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	cinemas, err := parseCinemarkCRCinemas(body)
	if err != nil {
		t.Fatalf("parseCinemarkCRCinemas: %v", err)
	}
	// Only the two Costa Rica cinemas, with the "Costa Rica - " prefix stripped.
	if len(cinemas) != 2 {
		t.Fatalf("got %d cinemas, want 2 (CR only)", len(cinemas))
	}
	if cinemas[0].ID != "770" || cinemas[0].Name != "Multiplaza Escazu" {
		t.Errorf("cinema[0] = %+v", cinemas[0])
	}
	if cinemas[1].Name != "Oxigeno" {
		t.Errorf("cinema[1].Name = %q, want Oxigeno", cinemas[1].Name)
	}
}

func TestParseCinemarkBillboard(t *testing.T) {
	body, err := os.ReadFile("testdata/cinemark_billboard.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	st, err := parseCinemarkBillboard(body, "Multiplaza Escazu", "770")
	if err != nil {
		t.Fatalf("parseCinemarkBillboard: %v", err)
	}
	if len(st) != 2 {
		t.Fatalf("got %d showtimes, want 2", len(st))
	}

	// Dubbed version, with screen tags recovered and country tag dropped.
	dub := st[0]
	if dub.Chain != "Cinemark" || dub.Cinema != "Multiplaza Escazu" || dub.Movie != "Moana Live Action" {
		t.Errorf("dub = %+v", dub)
	}
	if dub.Format != FormatDubbed {
		t.Errorf("format = %q, want dub", dub.Format)
	}
	if dub.Screen != "2D DBOX" {
		t.Errorf("screen = %q, want 2D DBOX", dub.Screen)
	}
	if got := dub.Start.Format("2006-01-02T15:04"); got != "2026-07-12T15:30" {
		t.Errorf("start = %q, want 2026-07-12T15:30", got)
	}
	if !strings.Contains(dub.BuyURL, "movie_id=HO00069398") || !strings.Contains(dub.BuyURL, "tag=770") {
		t.Errorf("buyURL = %q", dub.BuyURL)
	}

	// Subtitled version.
	if st[1].Format != FormatSubtitled {
		t.Errorf("format = %q, want sub", st[1].Format)
	}
	if st[1].Screen != "2D" {
		t.Errorf("screen = %q, want 2D", st[1].Screen)
	}
}

func TestCinemarkFormatScreen(t *testing.T) {
	cases := []struct {
		title      string
		wantFormat Format
		wantScreen string
	}{
		{"Moana Live Action (2D DOB DBOX -CR)", FormatDubbed, "2D DBOX"},
		{"Moana Live Action (2D SUB -CR)", FormatSubtitled, "2D"},
		{"Alien (DOB DBOX 3D -CR)", FormatDubbed, "DBOX 3D"},
	}
	for _, c := range cases {
		gotF, gotS := cinemarkFormatScreen(c.title)
		if gotF != c.wantFormat || gotS != c.wantScreen {
			t.Errorf("cinemarkFormatScreen(%q) = %q/%q, want %q/%q", c.title, gotF, gotS, c.wantFormat, c.wantScreen)
		}
	}
}
