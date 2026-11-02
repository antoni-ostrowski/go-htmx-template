# Go + HTMX + templ reference

Server-rendered HTMX todo app. `net/http`, `templ`, `sqlc`, PostgreSQL, `scs` sessions, OpenTelemetry. Minimal template.

## Run

Needs PostgreSQL. Default: `postgres://postgres:postgres@localhost:5432/todos?sslmode=disable`

```bash
mise run generate      # sqlc + templ codegen
mise run db-apply      # apply schema (sqldef, no migration files)
mise run dev           # hot reload (starts dev db, applies schema)
```

`DATABASE_URL` overrides the default.

## Architecture

```text
main.go      env + signals, then server.Run
server       wiring: mux + middleware + http.Server
handlers     one package per domain, sharing a Deps bag
sqlc         generated DB code from SQL
```

Each domain registers its own routes. Handlers call `*db.Queries` directly, templates take rows directly.

## Routes

```text
GET /                     full page
POST /todos               add
POST /todos/{id}/toggle   toggle
DELETE /todos/{id}        delete
GET+POST /signup|/signin  auth forms
POST /signout             log out
```

Page is public, mutations require login. Anonymous htmx requests get an `HX-Redirect` to sign-in.

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

## Styling

Tailwind via standalone CLI, no Node. `mise run dev` rebuilds CSS on change, `mise run build` minifies.

## Testing

Integration only: HTTP requests against real PostgreSQL, asserting response + DB state.

```bash
mise run test-int   # disposable Postgres, full suite
```

## Layout

```text
main.go
server/         Run + NewServer + NewHandler
db/             schema.sql, queries.sql, sqlc/
handlers/       handlers.go + todo/ + auth/ + static/
obs/            OTel setup + log handler
integration/    HTTP + DB tests
static/                  css input, vendored htmx
templates/               layout, todos, auth
```

New domain = new subpackage with `Register(mux, Deps)`, called from `server.NewHandler`.

## Tasks

```bash
mise run generate | css | dev | build
mise run db | db-plan | db-apply
mise run test | test-race | test-int | check
```
