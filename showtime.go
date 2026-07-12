package main

import (
	"strings"
	"time"
)

// crZone is Costa Rica's timezone. CR has observed no daylight saving since
// 1992, so a fixed UTC-6 offset is correct and avoids shipping tzdata into the
// scratch container image.
// ponytail: fixed -6 offset (no DST in CR); revisit only if CR reintroduces DST.
var crZone = time.FixedZone("CST", -6*60*60)

// Format is how a showing's audio/subtitles are presented. In Costa Rica a film
// is shown either subtitled (original audio + Spanish subtitles, "SUB"/"SUBT")
// or dubbed into Spanish ("DOB"/"ESP"). Everything else is unknown.
type Format string

const (
	FormatUnknown   Format = "unknown"
	FormatSubtitled Format = "sub"
	FormatDubbed    Format = "dub"
)

// ParseFormat maps a chain's free-text format/language label onto a Format.
// The chains use inconsistent Spanish labels ("Subtitulada", "SUBT", "VOSE",
// "Doblada", "ESP", "Español") so we match on substrings, subtitled first.
func ParseFormat(s string) Format {
	l := strings.ToLower(s)
	switch {
	case strings.Contains(l, "subt"), strings.Contains(l, "sub "),
		strings.HasSuffix(l, "sub"), strings.Contains(l, "vose"),
		strings.Contains(l, "vos"):
		return FormatSubtitled
	case strings.Contains(l, "dobl"), strings.Contains(l, "dob"),
		strings.Contains(l, "español"), strings.Contains(l, "espanol"),
		strings.Contains(l, "esp"), strings.Contains(l, "cast"):
		return FormatDubbed
	default:
		return FormatUnknown
	}
}

// Showtime is a single normalized screening across every chain. Providers emit
// these; the aggregator merges them and the frontend filters over them.
type Showtime struct {
	Chain    string    // "Cinépolis", "Cinemark", "CCM", "Sala Garbo"
	Cinema   string    // theater / location, e.g. "Cinépolis San Pedro"
	Movie    string    // film title
	Start    time.Time // screening start, in CR local time
	Format   Format    // subtitled / dubbed / unknown
	Language string    // original language if the chain exposes it, e.g. "Inglés"
	Screen   string    // optional auxiliary tags: "2D", "3D", "IMAX", "VIP", "4DX"
	BuyURL   string    // deep link to buy tickets, if available
}
