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
- `cinepolis.go` — **the one live reference scraper.** Real HTTP fetch of the
  public cartelera + a marker-split HTML parser, fixture-tested
  (`testdata/cinepolis.html`).
- `providers_stub.go` — Cinemark, CCM, Sala Garbo as stubs returning
  `errNotImplemented`. Each documents where to plug its parser in; the model,
  cache, API, and UI already handle everything downstream.
- `aggregator.go` — concurrent scrape of all providers, panic-isolated,
  per-chain error tracking, TTL/staleness, cached snapshot.
- `api.go` — `GET /api/showtimes` → normalized JSON + facets (chains, cinemas)
  + errors + `updatedAt`/`stale`.
- `server.go` / `gzip.go` — static-asset serving (versioned `?v=<hash>` URLs +
  gzip) reused from the **photo-editor** template; `/api/*` is routed alongside.

> **Scraper accuracy caveat.** Only Cinépolis is wired end-to-end, and its
> selectors were modeled on the chain's typical markup, **not** verified against
> the live site (the dev sandbox's egress policy blocks the CR cinema hosts).
> When running where egress is open, confirm the three regexes in `cinepolis.go`
> against the real DOM. The other three chains need their parsers written — swap
> the stub in `providers()` for a real `Provider` shaped like `cinepolis`.

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
