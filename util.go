package main

import (
	"regexp"
	"sort"
	"strings"
)

// splitOnMarker splits s into chunks that each begin with marker (an opening
// tag). Text before the first marker is dropped. Used to carve HTML into
// per-element regions without matching fragile nested closing tags.
func splitOnMarker(s, marker string) []string {
	parts := strings.Split(s, marker)
	if len(parts) <= 1 {
		return nil
	}
	out := make([]string, 0, len(parts)-1)
	for _, p := range parts[1:] {
		out = append(out, marker+p)
	}
	return out
}

// firstSubmatch returns the first capture group of re in s, or "" if no match.
func firstSubmatch(re *regexp.Regexp, s string) string {
	if m := re.FindStringSubmatch(s); m != nil {
		return m[1]
	}
	return ""
}

// stringSet collects distinct facet values (chains, cinemas) in insertion-blind
// order and returns them sorted for stable UI dropdowns.
type stringSet map[string]struct{}

func newStringSet() stringSet { return stringSet{} }

func (s stringSet) add(v string) { s[v] = struct{}{} }

func (s stringSet) sorted() []string {
	out := make([]string, 0, len(s))
	for v := range s {
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}
