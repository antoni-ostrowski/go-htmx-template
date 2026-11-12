# Go + HTMX + templ + PocketBase template

Minimal template for server-rendered Go/HTMX/Templ web apps. PocketBase is
used **as a Go library**: auth, SQLite storage, realtime, files, admin
dashboard, and migrations all live in the same binary next to your
handlers. OpenTelemetry traces + logs via OTLP/HTTP, stderr JSON always on.
Uses [mise](https://mise.jdx.dev/) for tooling.

> **Classic variant:** the pre-PocketBase setup (PostgreSQL + `sqlc` +
> `scs` sessions) lives on the
> [`pg-classic`](https://github.com/antoni-ostrowski/go-web-ref/tree/pg-classic)
> branch. Pick it when you need sustained heavy writes, horizontal
> scale-out, multi-TB data, or Postgres extensions — otherwise this branch
> ships more features with less code.

## Run

Needs nothing external. Data lives in `./pb_data` (SQLite + uploads).

```bash
mise run dev           # hot reload, serves on 127.0.0.1:8080
```

Dashboard (users, data, logs, backups, settings): `http://127.0.0.1:8080/_/`.
Dev boots auto-create a dummy superuser (`admin@admin.com` /
`admin`) when none exists — production (`APP_ENV=production`) never
does; there use `mise run superuser -- you@example.com`.

## Project structure

```text
main.go       slog default + OTel, then PocketBase app.Start
server/       BindAppHooks: OTel span middleware, PB-log forwarder, domains
handlers/     shared Deps + Render/WriteError/Redirect helpers (PB flavored)
handlers/todo one package per domain, Register(se, Deps) on the PB router
handlers/auth email+password on the users auth collection, pb_auth cookie
migrations/   EnsureCollections + generated files, compiled in, applied on serve
templates/    layout + pages (todos, auth) + components/ (shadcn-templ)
static/       css input, vendored htmx, realtime.js bridge, embedded assets
obs/          OTel setup + log handler
integration/  ApiScenario tests against isolated temp SQLite
utils/        shadcn-templ helpers (cn, etc.)
```

New domain = new subpackage with `Register(se, Deps)`, called from
`server.BindAppHooks`. Handlers use `e.App` (`FindRecordsByFilter`,
`Save`, `Delete`) directly, templates take plain view models directly.

## Auth

Email + password on the `users` auth collection (PB hashes + tokens).
Sign-up/sign-in issue a JWT into the readable `pb_auth` cookie and bounce
via `HX-Redirect`/303; sign-out clears it. Requests resolve identity from
the `Authorization` header first (API + realtime clients), then the cookie
(browser pages). Logout is stateless — forgetting the token is the logout.
OAuth2, OTP, MFA, and password reset are available as collection options.

## Realtime

PB streams record events over `/api/realtime` (SSE, governed by the
collection API rules: `owner = @request.auth.id`). `static/js/realtime.js`
subscribes to `todos/*` with the cookie token and fires
`pb-todos-changed`, which refetches the `#todo-list` fragment via
`GET /todos-fragment`. No Redis, no extra process.

## UI (htmx)

All client interactivity is htmx, vendored as `static/js/htmax.min.js`
(htmx v4 + htmax build). Fragments over XHR (`hx-post`, `hx-delete`,
`hx-target`, `hx-swap`), full pages on plain navigations, `HX-Redirect`
for post-mutation bounces. No build step, no Node — the file is compiled
into the binary and cache-busted like the CSS.

For the small scripting that remains (confirm dialogs, toggling bits of
DOM), use htmx's own `hx-on:` inline handlers instead of pulling in
Alpine.js — one less dependency for the same job.

## Observability

OTel traces (one span per route, named by pattern) + logs via OTLP/HTTP,
stderr JSON always on. No endpoint set → noop, app runs on stderr logs.
PB system logs are forwarded into the same pipeline; handler logs fan out
from the single `Deps.Logger` (built once in `main.go` via
`obs.NewLogHandler`): the server middleware stashes the PB app in the
request ctx, so `d.Logger.InfoContext(ctx, ...)` lands on stderr/OTLP
immediately and in the PB dashboard batched (tagged `via=handlers`, which
the forwarder skips instead of echoing back).

```bash
OTEL_EXPORTER_OTLP_ENDPOINT=   # set to enable
APP_ENV=production             # INFO+ export; otherwise DEBUG+
```

## Database

Collections are the schema. Edit them in the Dashboard during dev
(automigrate writes `migrations/*.go` for you), or run
`go run . migrate create "name"`. Files are compiled into the binary and
applied automatically on `serve` — code and schema ship together.

```bash
mise run superuser -- you@example.com   # dashboard access
```

`pb_data/` is gitignored: never commit it, back it up instead
(Dashboard → Backups, local or S3). Delete it to start dev fresh.

## Deployment

Single binary + `pb_data/` dir on a volume. No Postgres, no Redis.

```bash
mise run build             # minified CSS + ./tmp/app
./tmp/app serve --http=:8080 --dir=/path/to/pb_data
```

Behind a reverse proxy, forward proto/IP headers and allow long-lived SSE
(`read_timeout 360s` for nginx). Set SMTP, rate limits, and the superuser
IP whitelist in Settings before going live.

Tailwind via standalone CLI, no Node. Dev CSS is unminified (`mise run css-dev`, watched by air); prod CSS is minified (`mise run css-prod`, runs as part of `mise run build` → `tmp/app`).

Cache-busting is a content hash: `assets.Version` hashes the embedded
`css/app.css` + `js/` at startup, templates emit `/static/...?v=<hash>`,
and `?v=` responses carry `Cache-Control: immutable` (bare URLs get a
short cache). New content → new URL → browsers refetch; unchanged → no
re-request.

Static responses are precompressed once at startup (raw + gzip
BestCompression, `handlers/static/static.go`), so there is no per-request
CPU. Clients sending `Accept-Encoding: gzip` get `Content-Encoding: gzip`;
others get raw. `ETag` equals the content hash and `If-None-Match` → `304`,
with `Vary: Accept-Encoding`.

## Testing

Critical paths only, via PB's ApiScenario harness (real router + hooks,
each scenario on isolated temp SQLite — no Docker).

```bash
mise run test         # full suite
mise run test-race    # with race detector
```

## Tasks

```bash
mise run generate | css-dev | css-prod | dev | build
mise run superuser | migrate
mise run test | test-race | lint | check
```
