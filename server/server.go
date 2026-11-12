// Package server composes the whole HTTP serving stack on top of
// PocketBase: OTel tracing middleware, the PB log forwarder, and every
// domain registered on the PB router inside OnServe. main.go builds the PB
// app and calls BindAppHooks; the integration suite does the same on a test
// app, so route wiring can't drift.
package server

import (
	"log/slog"
	"os"
	"strings"

	"go-htmx-template/handlers"
	"go-htmx-template/handlers/auth"
	"go-htmx-template/handlers/static"
	"go-htmx-template/handlers/todo"
	"go-htmx-template/obs"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
)

// DefaultDeps builds the process-wide deps: the default slog logger (fanned
// out to stderr + OTLP by main.go) and noop-safe OTel instruments.
func DefaultDeps(service string) handlers.Deps {
	return handlers.Deps{
		Logger: slog.Default(),
		Tel: &handlers.Telemetry{
			Tracer: otel.Tracer(service),
			Meter:  otel.Meter(service),
		},
	}
}

// BindAppHooks registers everything on app: the dev superuser seed, the OTel
// span middleware, the PB-logs forwarder, and all domain routes. Called once
// in main; tests call it on their own test app instance.
func BindAppHooks(app core.App, d handlers.Deps) {
	// Dev-only dummy superuser so the Dashboard is one click away.
	// Production (APP_ENV=production) is never touched, and an existing
	// superuser is never overwritten. Tests are unaffected: their apps
	// finish bootstrap before this is bound.
	app.OnBootstrap().BindFunc(func(e *core.BootstrapEvent) error {
		if err := e.Next(); err != nil {
			return err
		}
		return ensureDevSuperuser(e.App)
	})
	// Forward PB system logs (request logger, auth failures, hook errors)
	// into the default slog pipeline so they reach stderr + OTLP. Handler
	// logs already fan out via the obs handler, so skip those (tagged
	// via=handlers) instead of echoing them back.
	app.OnModelCreate(core.LogsTableName).BindFunc(func(e *core.ModelEvent) error {
		if l, ok := e.Model.(*core.Log); ok && l.Data["via"] != "handlers" {
			slog.Default().Info("pocketbase",
				"message", l.Message, "level", l.Level, "data", l.Data)
		}
		return e.Next()
	})

	app.OnServe().BindFunc(func(se *core.ServeEvent) error {
		if !strings.EqualFold(os.Getenv("APP_ENV"), "production") {
			// Never auto-open a browser in dev: PB's default installer
			// func runs `open <url>`. The dev seed above should make this
			// unreachable; this is just a quiet backstop.
			se.InstallerFunc = func(core.App, *core.Record, string) error {
				slog.Warn("dashboard superuser missing (dev): run go run . superuser upsert EMAIL PASS")
				return nil
			}
		}
		se.Router.BindFunc(otelMiddleware(d))
		todo.Register(se, d)
		auth.Register(se, d)
		static.Register(se)
		return se.Next()
	})
}

// devSuperuserEmail/Password are dummy Dashboard credentials, dev only.
const (
	devSuperuserEmail    = "admin@admin.com"
	devSuperuserPassword = "admin"
)

// ensureDevSuperuser creates the dummy superuser on non-production boots
// that have none yet. Production is skipped entirely; existing superusers
// are left alone.
func ensureDevSuperuser(app core.App) error {
	if strings.EqualFold(os.Getenv("APP_ENV"), "production") {
		return nil
	}
	total, err := app.CountRecords(core.CollectionNameSuperusers, dbx.Not(dbx.HashExp{
		// Mirror PB's own installer check: the ephemeral installer record
		// doesn't count as a real superuser.
		"email": core.DefaultInstallerEmail,
	}))
	if err != nil || total > 0 {
		return err
	}
	collection, err := app.FindCollectionByNameOrId(core.CollectionNameSuperusers)
	if err != nil {
		return err
	}
	record := core.NewRecord(collection)
	record.Set("email", devSuperuserEmail)
	record.Set("password", devSuperuserPassword)
	// NoValidate: "admin" is shorter than the password min-length rule.
	// Fine for a local dummy, never used in production (gated above).
	if err := app.SaveNoValidate(record); err != nil {
		return err
	}
	slog.Warn("dev superuser created (non-production only)",
		"email", devSuperuserEmail, "password", devSuperuserPassword)
	return nil
}

// otelMiddleware runs before every domain handler: it stashes the PB app in
// ctx (so the obs log handler fans out to the dashboard) and wraps the
// request in a span named after the matched route pattern (falling back to
// method + path), so all routes don't collapse into one span name.
func otelMiddleware(d handlers.Deps) func(e *core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		name := e.Request.Method + " " + e.Request.URL.Path
		if p := e.Request.Pattern; p != "" {
			name = p
		}
		ctx := obs.ContextWithApp(e.Request.Context(), e.App)
		ctx, span := d.Tel.Tracer.Start(ctx, name)
		defer span.End()
		e.Request = e.Request.WithContext(ctx)
		if err := e.Next(); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return err
		}
		return nil
	}
}
