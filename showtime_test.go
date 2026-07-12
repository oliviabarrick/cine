package main

import "testing"

func TestParseFormat(t *testing.T) {
	cases := map[string]Format{
		"Subtitulada": FormatSubtitled,
		"SUBT":        FormatSubtitled,
		"VOSE inglés": FormatSubtitled,
		"Idioma SUB":  FormatSubtitled,
		"Doblada":     FormatDubbed,
		"DOB":         FormatDubbed,
		"Español":     FormatDubbed,
		"Castellano":  FormatDubbed,
		"":            FormatUnknown,
		"IMAX 3D":     FormatUnknown,
	}
	for in, want := range cases {
		if got := ParseFormat(in); got != want {
			t.Errorf("ParseFormat(%q) = %q, want %q", in, got, want)
		}
	}
}
