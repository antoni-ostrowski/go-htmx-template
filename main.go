package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	_ "go-htmx-template/migrations"
	"go-htmx-template/obs"
	"go-htmx-template/server"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/plugins/migratecmd"
	"github.com/pocketbase/pocketbase/tools/osutils"
)

const service = "go-htmx-template"

func main() {
	slog.SetDefault(slog.New(obs.NewLogHandler(service)))

	otelShutdown, otelErr := obs.SetupOTelSDK(context.Background(), service)
	if otelErr != nil {
		slog.Warn("otel disabled", "reason", otelErr)
	} else {
		defer func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := otelShutdown(ctx); err != nil {
				slog.Error("shutdown otel SDK", "error", err)
			}
		}()
	}

	app := pocketbase.New()

	migratecmd.MustRegister(app, app.RootCmd, migratecmd.Config{
		// New migration files are generated from Dashboard changes in dev
		// only. Prod applies the committed ones on serve.
		Automigrate: osutils.IsProbablyGoRun(),
	})

	server.BindAppHooks(app, server.DefaultDeps(service))

	if err := app.Start(); err != nil {
		slog.Error("server error", "err", err)
		os.Exit(1)
	}
}
