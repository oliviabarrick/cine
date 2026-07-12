# cine

**Cartelera unificada de cines en Costa Rica.** A Go backend scrapes the local
cinema chains, normalizes everything into one structured feed, and serves a
browser UI you can filter by **película**, **fecha**, **formato**
(subtituladas / dobladas), **sala**, and **cadena** — defaulting to
*subtituladas*.

## How it works

- **Scrape server-side, on a schedule.** A background loop scrapes every chain
  concurrently into an in-memory cache (`aggregator.go`). Page loads only read
  the cache, so a slow or down cinema site never blocks the UI, and one chain
  failing never breaks the others.
- **Normalize.** Each chain's listing is mapped to a common `Showtime`
  (chain, cinema, movie, start time in CR local, sub/dub format, language, buy
  link). The chains' inconsistent Spanish labels ("Subtitulada", "SUBT",
  "Doblada", "ESP", …) are collapsed to `sub` / `dub` / `unknown`.
- **Serve + filter.** `GET /api/showtimes` returns the normalized feed plus
  facets and per-chain error/staleness metadata; the static frontend (`web/`)
  does the filtering client-side. No framework, no client build step.

## Chains

| Chain | Status |
|-------|--------|
| Cinépolis  | ✅ live reference scraper (`cinepolis.go`) |
| Cinemark   | 🚧 stub — parser TODO |
| CCM        | 🚧 stub — parser TODO |
| Sala Garbo | 🚧 stub — parser TODO |

Adding a chain is one file: implement the `Provider` interface (a `Fetch` that
calls `fetchPage` and runs a chain-specific parser) and register it in
`providers()`. The model, cache, API, and UI already handle the rest — see
`providers_stub.go` for the plug-in points.

> **Note on the reference scraper.** Cinépolis's selectors were modeled on the
> chain's typical markup, not verified against the live site (this project was
> built in a sandbox whose network policy blocks the CR cinema hosts). Confirm
> the three regexes in `cinepolis.go` against the real DOM when running with open
> egress; they're the only thing that changes.

## Development

```sh
go run .                 # serve on :8080 (scrapes on startup, then every 30m)
go test ./...            # model, parser (fixture), aggregator, server/API — no network
helm lint charts/cine    # validate the chart
```

Then visit http://localhost:8080. The client is plain static files under `web/`
(`index.html`, `app.js`, `app.css`) — edit and refresh, no build step. The
server fingerprints those files and appends `?v=<hash>` to the asset URLs so a
new build busts Cloudflare's edge cache automatically (`index.html` itself is
served `no-cache`).

## Stack & deploy

A small pure-standard-library Go server (no external modules) serving a
vanilla-JS frontend — the same shape and **deployment pipeline** as its sibling
`photo-editor`: GitHub Actions builds and pushes a container image to `ghcr.io`
and bumps the Helm chart's `image.tag`; a FluxCD `HelmRelease` in the `thinkpod`
cluster repo builds `charts/cine` straight from this repo and reconciles it. The
build/publish/deploy details live in `.github/CLAUDE.md` (generated fleet-wide by
`skylartaylor/thinkpod`'s Terraform).
