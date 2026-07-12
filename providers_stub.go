package main

import "context"

// stubProvider is a chain that's registered and wired into the aggregator/UI but
// whose live scraper isn't written yet. It returns errNotImplemented, which the
// aggregator records as a per-chain error without failing the others.
//
// To implement one, replace the stub in providers() with a real type shaped
// exactly like cinepolis: a Fetch that calls fetchPage(ctx, site), runs a
// chain-specific parser, and returns normalized []Showtime. The model, cache,
// API, and frontend already handle everything downstream — only the parser is
// per-chain work.
//
//   - Cinemark  — https://www.cinemarkca.com/costa-rica (per-cinema `tag`/`cine`
//     query params; listings load via XHR, so find the JSON endpoint in the
//     Network tab rather than parsing the SPA shell).
//   - CCM       — https://www.ccmcinemas.com/
//   - Sala Garbo— https://salagarbo.com/cartelera/ (single art-house cinema,
//     mostly subtitled — server-rendered HTML, so an HTML parser like
//     Cinépolis's fits best).
type stubProvider struct {
	name string
	site string
}

func (s stubProvider) Name() string { return s.name }

func (s stubProvider) Fetch(context.Context) ([]Showtime, error) {
	return nil, errNotImplemented
}
