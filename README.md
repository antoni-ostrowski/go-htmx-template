# Go + HTMX + templ template

Minimal template for Server-rendered Go/HTMX/Templ web apps. Uses `net/http`, `templ`, `sqlc`, PostgreSQL, `scs` sessions, OpenTelemetry. Uses [mise](https://mise.jdx.dev/) for tooling.

## Run

Needs PostgreSQL. Default: `postgres://postgres:postgres@localhost:5432/todos?sslmode=disable`

```bash
mise run dev           # hot reload (starts dev db, applies schema)
```

`DATABASE_URL` overrides the default.

## Project structure

```text
main.go       env + signals, then server.Run
server/       mux + middleware + http.Server wiring
handlers/     one package per domain (todo/, auth/, static/), sharing Deps
db/           schema.sql + queries.sql -> sqlc/ generated code
templates/    layout + pages (todos, auth) + components/ (shadcn-templ)
static/       css input, vendored htmx, embedded assets
obs/          OTel setup + log handler
integration/  HTTP + DB tests against real Postgres
utils/        shadcn-templ helpers (cn, etc.)
```

New domain = new subpackage with `Register(mux, Deps)`, called from `server.NewHandler`. Handlers call `*db.Queries` directly, templates take rows directly.

## Auth

Username + password with bcrypt. Sessions in Postgres via `scs`. Sign-up and sign-in log the user in, sign-out destroys the session.

## Observability

OTel traces + logs via OTLP/HTTP, stderr JSON always on. No endpoint set → noop, app runs on stderr logs.

```bash
OTEL_EXPORTER_OTLP_ENDPOINT=   # set to enable
APP_ENV=production             # INFO+ export; otherwise DEBUG+
```

## Database

`db/schema.sql` is the source of truth for both sqldef and sqlc. Edit it, then `mise run generate` → `db-plan` → `db-apply`.

## Deployment

Single binary + Postgres. `static/` is compiled in via `embed.FS`, so the server never touches disk at runtime.

Tailwind via standalone CLI, no Node. Dev CSS is unminified (`mise run css-dev`, watched by air); prod CSS is minified (`mise run css-prod`, runs as part of `mise run build` → `tmp/app`).

Cache-busting is a content hash: `assets.Version` hashes the embedded `css/app.css` + `js/` at startup, templates emit `/static/...?v=<hash>`, and `?v=` responses carry `Cache-Control: immutable` (bare URLs get a short cache). New content → new URL → browsers refetch; unchanged → no re-request.

Static responses are precompressed once at startup (raw + gzip BestCompression, `handlers/static/static.go`), so there is no per-request CPU. Clients sending `Accept-Encoding: gzip` get `Content-Encoding: gzip` (~119KB resources → ~42KB transferred for HTML + `htmax.min.js` + `app.css`); others get raw. `ETag` equals the content hash and `If-None-Match` → `304`, with `Vary: Accept-Encoding`.

## Testing

Integration only: HTTP requests against real PostgreSQL, asserting response + DB state.

```bash
mise run test-int   # disposable Postgres, full suite
```

## Tasks

```bash
mise run generate | css-dev | css-prod | dev | build
mise run db | db-plan | db-apply
mise run test | test-race | test-int | lint | check
```
