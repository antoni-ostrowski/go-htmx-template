package obs

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"strings"

	"github.com/pocketbase/pocketbase/core"
	"go.opentelemetry.io/contrib/bridges/otelslog"
)

// MinLevel drops records below Min before reaching the wrapped handler.
// It exists because the OTel bridge reports every level as enabled, so
// without the gate Debug records would also export.
type MinLevel struct {
	slog.Handler
	Min slog.Level
}

func (g MinLevel) Enabled(ctx context.Context, l slog.Level) bool {
	return l >= g.Min && g.Handler.Enabled(ctx, l)
}

func (g MinLevel) WithAttrs(attrs []slog.Attr) slog.Handler {
	return MinLevel{Handler: g.Handler.WithAttrs(attrs), Min: g.Min}
}

func (g MinLevel) WithGroup(name string) slog.Handler {
	return MinLevel{Handler: g.Handler.WithGroup(name), Min: g.Min}
}

// otlpMinLevel: production exports INFO and above (INFO, WARN, ERROR),
// anything else (dev/test) exports everything from DEBUG up.
func otlpMinLevel() slog.Level {
	if strings.EqualFold(os.Getenv("APP_ENV"), "production") {
		return slog.LevelInfo
	}
	return slog.LevelDebug
}

// NewLogHandler fans out: OTel at otlpMinLevel(), stderr JSON at Debug and
// above (everything). Build it after SetupOTelSDK so the bridge captures the
// real LoggerProvider instead of the noop.
//
// The returned handler also forwards to the PB dashboard, but only when the
// record's ctx carries a PB app (see ContextWithApp, set once per request by
// the server middleware). The PB copy is tagged via=handlers so the PB-log
// forwarder in server can skip it instead of echoing it back to stderr/OTLP.
func NewLogHandler(service string) slog.Handler {
	return fanoutHandler{primary: slog.NewMultiHandler(
		MinLevel{Handler: otelslog.NewHandler(service), Min: otlpMinLevel()},
		slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}),
	)}
}

// appKey is the ctx key holding the PB app for dashboard fan-out.
type appKey struct{}

// ContextWithApp stashes app in ctx so logs emitted with that ctx also reach
// the PB dashboard. Without it, logs go to stderr/OTLP only.
func ContextWithApp(ctx context.Context, app core.App) context.Context {
	return context.WithValue(ctx, appKey{}, app)
}

// fanoutHandler writes every record to primary and, when ctx carries a PB
// app, also to that app's dashboard logger.
type fanoutHandler struct {
	primary slog.Handler
}

func (h fanoutHandler) Enabled(ctx context.Context, l slog.Level) bool {
	return h.primary.Enabled(ctx, l)
}

func (h fanoutHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return fanoutHandler{primary: h.primary.WithAttrs(attrs)}
}

func (h fanoutHandler) WithGroup(name string) slog.Handler {
	return fanoutHandler{primary: h.primary.WithGroup(name)}
}

func (h fanoutHandler) Handle(ctx context.Context, r slog.Record) error {
	err := h.primary.Handle(ctx, r)
	app, ok := ctx.Value(appKey{}).(core.App)
	if !ok || app == nil {
		return err
	}
	r.AddAttrs(slog.String("via", "handlers"))
	if ferr := app.Logger().Handler().Handle(ctx, r); ferr != nil {
		return errors.Join(err, ferr)
	}
	return err
}
