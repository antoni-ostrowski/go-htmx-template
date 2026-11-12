// Package handlers holds shared HTTP wiring: the Deps bag plus helpers
// every domain uses. Domains live in subpackages and import this package,
// never the reverse. All helpers operate on PocketBase's *core.RequestEvent.
package handlers

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"

	"go-htmx-template/templates"

	"github.com/a-h/templ"
	"github.com/pocketbase/pocketbase/core"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

// Deps carries process-wide dependencies, built once by the caller.
// Logger is the fan-out logger from obs.NewLogHandler: handlers call it with
// the request ctx (which carries the PB app) and records land on stderr +
// OTLP immediately and in the PB dashboard batched. Request-scoped state
// (auth record) always comes from *core.RequestEvent, never from here.
type Deps struct {
	Logger *slog.Logger
	Tel    *Telemetry
}

type Telemetry struct {
	Tracer trace.Tracer
	Meter  metric.Meter
}

// WriteError logs err with request context and renders an HTML error
// fragment with code. 4xx bodies carry the message (client-caused, safe);
// 5xx bodies stay generic while the real error goes to the log. Pass a
// custom fragment via opts.ErrComponent to override the default ErrorText
// span. 5xx logs at Error and marks spans failed (RecordError + Error
// status); 4xx logs at Debug and stays green on spans.
type WriteErrorOpts struct {
	Err          error
	Code         int
	Spans        []trace.Span
	ErrComponent templ.Component
}

func WriteError(e *core.RequestEvent, opts WriteErrorOpts) error {
	if opts.Err == nil {
		opts.Err = errors.New("internal server error")
	}
	if opts.Code == 0 {
		opts.Code = http.StatusInternalServerError
	}
	msg := opts.Err.Error()
	if opts.Code >= 500 {
		spans := append(opts.Spans, trace.SpanFromContext(e.Request.Context()))
		for _, s := range spans {
			s.RecordError(opts.Err)
			s.SetStatus(codes.Error, http.StatusText(opts.Code))
		}
		msg = http.StatusText(opts.Code)
	}
	if opts.ErrComponent == nil {
		opts.ErrComponent = templates.ErrorText(msg)
	}

	if opts.Code >= 500 {
		slog.Default().ErrorContext(e.Request.Context(), "handler error",
			"method", e.Request.Method, "path", e.Request.URL.Path, "status", opts.Code, "err", opts.Err)
	} else {
		slog.Default().DebugContext(e.Request.Context(), "handler error",
			"method", e.Request.Method, "path", e.Request.URL.Path, "status", opts.Code, "err", opts.Err)
	}

	html, err := renderToString(e.Request.Context(), opts.ErrComponent)
	if err != nil {
		return e.String(opts.Code, msg)
	}
	return e.HTML(opts.Code, html)
}

// Render writes page with 200 OK. This is the common case; for error-status
// re-renders (validation, conflicts) use RenderWithCode.
func Render(e *core.RequestEvent, page templ.Component) error {
	return render(e, page, http.StatusOK)
}

// RenderWithCode writes page with the given status. Use it for semantic
// error renders (422, 409, 401); success renders use Render. Redirects (3xx)
// never render — use Redirect instead.
func RenderWithCode(e *core.RequestEvent, page templ.Component, code int) error {
	return render(e, page, code)
}

func render(e *core.RequestEvent, page templ.Component, code int) error {
	html, err := renderToString(e.Request.Context(), page)
	if err != nil {
		return WriteError(e, WriteErrorOpts{Err: err, Code: http.StatusInternalServerError})
	}
	return e.HTML(code, html)
}

func renderToString(ctx context.Context, page templ.Component) (string, error) {
	var buf bytes.Buffer
	if err := page.Render(ctx, &buf); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// Redirect navigates the client to loc whether the request came from htmx
// or a plain navigation: htmx requests get HX-Redirect + 204 (client-side
// navigation — an XHR-followed 303 would swap the target page into the
// current one instead), plain requests get a 303. Preferably use it for
// every redirect from handlers.
func Redirect(e *core.RequestEvent, loc string) error {
	return redirect(e, loc, http.StatusNoContent)
}

// RedirectWithCode is Redirect with the htmx-branch status overridden (the
// plain branch is always a 303 — a 4xx to a plain navigation only shows the
// browser's error page). RequireAuth uses it for 401 denials.
func RedirectWithCode(e *core.RequestEvent, loc string, code int) error {
	return redirect(e, loc, code)
}

func redirect(e *core.RequestEvent, loc string, htmxCode int) error {
	htmx := e.Request.Header.Get("HX-Request") == "true"
	slog.Default().InfoContext(e.Request.Context(), "redirect",
		"method", e.Request.Method, "path", e.Request.URL.Path, "loc", loc, "htmx", htmx)
	if htmx {
		e.Response.Header().Set("HX-Redirect", loc)
		return e.NoContent(htmxCode)
	}
	return e.Redirect(http.StatusSeeOther, loc)
}
