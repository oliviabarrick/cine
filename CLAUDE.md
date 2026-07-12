# CLAUDE.md

## Application

**Cine** is a Costa Rica movie-showtimes aggregator. A Go backend scrapes the
local cinema chains, normalizes their listings into one structured feed, and
serves a filterable browser UI — filter by **movie**, **date**, **format**
(subtituladas / dobladas), **sala**, and **cadena**. It defaults to
*subtituladas* (the common local preference).

Scraping runs **server-side on a background schedule** into an in-memory cache
(`aggregator.go`); HTTP requests only ever read that cache, so a slow or broken
cinema site never blocks a page load. The frontend (`web/`, vanilla JS + a
`fetch` of `/api/showtimes`) does all filtering client-side.

### Layout

- `showtime.go` — the normalized `Showtime` model + `ParseFormat` (maps the
  chains' inconsistent Spanish labels onto sub/dub) + the fixed CR timezone.
- `provider.go` — the `Provider` interface every chain implements, the registry
  (`providers()`), and the shared HTTP fetch helper.
- `cinemark.go` — **live scraper.** Cinemark CA's Vista JSON API
  (`api.cinemarkca.com/api/vista/data`): filter `theatres` to Costa Rica, then
  `billboard?cinema_id=<id>` per cinema; sub/dub parsed from the version-title
  suffix. Fixture-tested (`testdata/cinemark_*.json`).
- `ccm.go` — **live scraper.** CCM's ASP.NET JSON endpoints, fanned out per
  cinema (401/402/403) → movie → date: `GetPeliculasPorComplejo`,
  `GetFechasDisponibles`, `GetCacheFuncionesComplejoPeliculaFecha` (accurate
  per-screening sub/dub). Fixture-tested (`testdata/ccm_*.json`).
- `salagarbo.go` — **live scraper.** Server-rendered WordPress cartelera parsed
  with a marker-split HTML parser; single art-house cinema, defaults to
  subtituladas. Fixture-tested (`testdata/salagarbo.html`).
- `cinepolis.go` — **live scraper.** The public catalog
  (`cinepolis.co.cr/wp-json/mapi/v1/sites-data`) gives cinemas + films; the
  BiggerPicture ticketing API (`pub-api-use1.biggerpicture.ai/ecomAPI`) supplies
  the sessions behind a POST-minted JWT. Fanned out per cinema × film over a date
  window with bounded concurrency; per-screening sub/dub. Fixture-tested
  (`testdata/cinepolis_*.json`).
- `providers_stub.go` — `stubProvider` scaffold (no chains use it now; kept as
  the template for wiring a new chain).
- `aggregator.go` — concurrent scrape of all providers, panic-isolated,
  per-chain error tracking, TTL/staleness, cached snapshot.
- `api.go` — `GET /api/showtimes` → normalized JSON + facets (chains, cinemas)
  + errors + `updatedAt`/`stale`.
- `server.go` / `gzip.go` — static-asset serving (versioned `?v=<hash>` URLs +
  gzip) reused from the **photo-editor** template; `/api/*` is routed alongside.

> **All four chains are now live** and tested against real endpoint shapes.
> Cinépolis is the odd one out: its browse site (`cinepolis.co.cr`) is a
> WordPress + React app whose only JSON endpoint (`/wp-json/mapi/v1/sites-data`)
> carries the movie catalog + cinemas + sub/dub flags but **no showtimes**. The
> times live in the BiggerPicture ticketing API
> (`pub-api-use1.biggerpicture.ai/ecomAPI`): `POST /sys/login` mints a bearer
> JWT (one token serves every cinema), then
> `GET /cus/eventMaster/{emid}/site/{siteId}/startDate/{d}/endDate/{d}` with
> `Authorization: Bearer <token>` returns a film's sessions at a cinema over a
> date window (`.list`, with per-screening `isSubtitled`/`isDubbed`). There's no
> site→movie map, so `cinepolis.go` fans out over every (cinema, film) pair —
> bounded by `cinepolisConcurrency` over a `cinepolisMaxDays` window. If the
> session field names shift, re-capture an authed response and update the
> `cineSession` struct.

```sh
go run .                     # serve on :8080 (scrapes on startup + every 30m)
go test ./...                # model, parser (fixture), aggregator, server/API
GOFLAGS= go vet ./...
helm lint charts/cine        # validate the chart
```

There is no client build step — `web/` is served as-is. The container image and
CI just compile the Go server and copy `web/` in (see `Dockerfile`).

## Deployment

This repo reuses the **photo-editor** deployment pipeline: CI → GHCR image →
Helm chart (`charts/cine`) → Flux, fronted by a Cloudflare Tunnel ingress.
Onboarding is a single entry in `skylartaylor/thinkpod`'s
`cluster/apps/fleet.yaml`: Flux merges it into the app-factory to deploy
(namespace / GitRepository / HelmRelease / ingress at `cine.taylor-barrick.com`),
and Terraform reads the same file to generate this repo's CI —
`.github/workflows/publish.yml`, `.github/workflows/lint.yml`, and the managed
`.github/CLAUDE.md`. **Those three are Terraform-owned — don't hand-author them
here; only `test.yml` (repo build + tests) is repo-owned.** The
**build → publish → deploy pipeline and MCP connectors live in `.github/CLAUDE.md`**
(generated once this repo joins the fleet). See thinkpod's
`docs/adding-an-app.md`.

Chart-authoring convention (same as photo-editor): keep `values.yaml` free of
anything cluster-specific — `pullSecretName` has no default; the deploying
`HelmRelease` supplies it, and templates guard it with
`{{- if .Values.pullSecretName }}`. Don't hand-edit `image.tag` — CI's publish
step sets it after each push to `main`.

## Contributing

Develop on a branch, push, and open a PR (never push straight to `main`). Adding
a new chain = writing one `Provider`; everything else already handles it.

## Managed deploy & ops guidance

The build → publish → deploy pipeline, `gh`/git usage, PR-watching, and MCP
connectors are documented in the Terraform-managed `.github/CLAUDE.md`.
Subdirectory `CLAUDE.md` files load only on demand (when a session opens files
under that directory), so import it here to pull it into context at session
start:

@.github/CLAUDE.md
