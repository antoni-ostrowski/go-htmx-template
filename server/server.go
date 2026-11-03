// Package server composes the whole HTTP serving stack: every domain
// registered on one mux behind session middleware and the otelhttp server
// span, plus the configured http.Server. main.go and the integration suite
// share it so route wiring can't drift; only the deps (pool, logger,
// static dir) differ per environment.
package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	db "go-htmx-template/db/sqlc"
	"go-htmx-template/handlers"
	"go-htmx-template/handlers/auth"
	"go-htmx-template/handlers/static"
	"go-htmx-template/handlers/todo"
	"go-htmx-template/obs"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
)

// Config carries everything Run needs that differs per environment.
// main.go fills it from env; tests fill it with test values.
type Config struct {
	DatabaseURL string
	Addr        string
	Service     string
}

// Run boots the full stack — logger, OTel SDK, pool, sessions, server —
// and serves until ctx is done, then drains gracefully. main.go calls it
// with env config; the smoke test calls it with test config to prove the
// real wiring serves. OTel failures never block startup (warning + noop);
// everything else fails fast. Callers own slog.SetDefault.
func Run(ctx context.Context, cfg Config) error {
	if cfg.DatabaseURL == "" || cfg.Addr == "" || cfg.Service == "" {
		return fmt.Errorf("server: DatabaseURL, Addr and Service are required")
	}
	logger := slog.New(obs.NewLogHandler(cfg.Service))

	otelShutdown, otelErr := obs.SetupOTelSDK(ctx, cfg.Service)
	if otelErr != nil {
		logger.Warn("otel disabled", "reason", otelErr)
		otelShutdown = nil
	}
	defer func() {
		if otelShutdown == nil {
			return
		}
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := otelShutdown(shutdown); err != nil {
			logger.Error("shutdown otel SDK", "error", err)
		}
	}()

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("create database pool: %w", err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("ping database: %w", err)
	}

	sessions := auth.NewSessionManager(pool)
	deps := handlers.Deps{
		Queries:  db.New(pool),
		Sessions: sessions, Logger: logger,
		Tel: &handlers.Telemetry{Tracer: otel.Tracer(cfg.Service), Meter: otel.Meter(cfg.Service)},
	}
	srv := NewServer(deps, cfg.Addr)

	srvErr := make(chan error, 1)
	go func() {
		logger.Info("listening", "address", "http://"+cfg.Addr)
		srvErr <- srv.ListenAndServe()
	}()

	select {
	case err := <-srvErr:
		// Startup failed: nothing to drain, and the pool never served
		// traffic, so exiting directly is safe.
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	case <-ctx.Done():
		// Drain in-flight work within budget.
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdown); err != nil {
			return err
		}
		logger.Info("stopped")
		return nil
	}
}

// NewServer builds the production-ready http.Server around NewHandler.
func NewServer(d handlers.Deps, addr string) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           NewHandler(d),
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      10 * time.Second,
	}
}

// NewHandler registers every domain and wraps the mux. Static assets come
// from the embedded FS, so no dir argument is needed in any environment.
func NewHandler(d handlers.Deps) http.Handler {
	mux := http.NewServeMux()
	todo.Register(mux, d)
	auth.Register(mux, d)
	static.Register(mux)
	return otelhttp.NewHandler(d.Sessions.LoadAndSave(mux), "server")
}
