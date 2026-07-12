package main

import "context"

// stubProvider is a chain that's registered and wired into the aggregator/UI but
// whose live scraper isn't written yet. It returns errNotImplemented, which the
// aggregator records as a per-chain error without failing the others.
//
// No chain currently uses it — Cinemark (cinemark.go), CCM (ccm.go), and Sala
// Garbo (salagarbo.go) are all live. It's kept as the template for adding a new
// chain: replace the stub in providers() with a real type whose Fetch calls
// fetchPage / the chain's JSON endpoint, runs a chain-specific parser, and
// returns normalized []Showtime. The model, cache, API, and frontend already
// handle everything downstream — only the parser is per-chain work.
type stubProvider struct {
	name string
	site string
}

func (s stubProvider) Name() string { return s.name }

func (s stubProvider) Fetch(context.Context) ([]Showtime, error) {
	return nil, errNotImplemented
}
