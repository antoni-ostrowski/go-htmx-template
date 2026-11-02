package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"go-htmx-todo/obs"
	"go-htmx-todo/server"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		databaseURL = "postgres://postgres:postgres@localhost:5432/todos?sslmode=disable"
	}
	cfg := server.Config{
		DatabaseURL: databaseURL,
		StaticDir:   "static",
		Addr:        ":8080",
		Service:     "go-htmx-template",
	}
	slog.SetDefault(slog.New(obs.NewLogHandler(cfg.Service)))
	if err := server.Run(ctx, cfg); err != nil {
		slog.Error("server error", "err", err)
		os.Exit(1)
	}
}
