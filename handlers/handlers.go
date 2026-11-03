// Package handlers holds shared HTTP wiring: the Deps bag plus helpers
// every domain uses. Domains live in subpackages and import this package,
// never the reverse.
package handlers

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	db "go-htmx-todo/db/sqlc"
	"go-htmx-todo/templates"

	"github.com/a-h/templ"
	"github.com/alexedwards/scs/v2"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

// Deps carries process-wide dependencies, built once by the caller.
// New shared deps join here as fields; never request-scoped data.
type Deps struct {
	Queries  *db.Queries
	Sessions *scs.SessionManager
	Logger   *slog.Logger
	Tel      *Telemetry
}

type Telemetry struct {
	Tracer trace.Tracer
	Meter  metric.Meter
}

// WriteError logs err with request context and renders an HTML error
// fragment with code. 4xx bodies carry the message (client-caused, safe);
// 5xx bodies stay generic while the real error goes to the log. Pass a
// custom fragment via opts.ErrComponent to override the default ErrorText
// span. 5xx logs at Error via slog.Default (set by main.go in prod) and
// marks spans failed for 5xx (RecordError + Error status): always the request span,
// plus any spans passed in (e.g. a child wrapping the failed work) — full
// red, so both discovery and diagnosis work. 4xx logs at Debug and stays
// green on spans: client errors are expected behavior, not failures.
type WriteErrorOpts struct {
	Err          error
	Code         int
	Spans        []trace.Span
	ErrComponent templ.Component
}

func WriteError(w http.ResponseWriter, r *http.Request, opts WriteErrorOpts) {
	if opts.Err == nil {
		opts.Err = errors.New("internal server error")
	}
	if opts.Code == 0 {
		opts.Code = http.StatusInternalServerError
	}
	msg := opts.Err.Error()
	if opts.Code >= 500 {
		spans := append(opts.Spans, trace.SpanFromContext(r.Context()))
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
		slog.Default().ErrorContext(r.Context(), "handler error",
			"method", r.Method, "path", r.URL.Path, "status", opts.Code, "err", opts.Err)
	} else {
		slog.Default().DebugContext(r.Context(), "handler error",
			"method", r.Method, "path", r.URL.Path, "status", opts.Code, "err", opts.Err)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(opts.Code)
	if err := opts.ErrComponent.Render(r.Context(), w); err != nil {
		slog.Default().ErrorContext(r.Context(), "error fragment render failed", "err", err)
	}
}

// Route registers pattern on mux and renames the otelhttp server span to the
// matched pattern (r.Pattern). Without it the outer otelhttp handler names
// every span after its static operation string, collapsing all routes into
// one useless span name.
func Route(mux *http.ServeMux, pattern string, h http.Handler) {
	mux.Handle(pattern, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s := trace.SpanFromContext(r.Context()); s.IsRecording() {
			if p := r.Pattern; p != "" {
				s.SetName(p)
			}
		}
		h.ServeHTTP(w, r)
	}))
}

// Render writes page with 200 OK, routing render failures to WriteError
// (500). This is the common case; for error-status re-renders (validation,
// conflicts) use [RenderWithCode].
func Render(w http.ResponseWriter, r *http.Request, page templ.Component) {
	render(w, r, page, http.StatusOK)
}

// RenderWithCode writes page with the given status, routing render failures
// to WriteError (500). Use it for semantic error renders (422, 409, 401);
// success renders use Render. Redirects (3xx) never render — use
// [http.Redirect] instead.
func RenderWithCode(w http.ResponseWriter, r *http.Request, page templ.Component, code int) {
	render(w, r, page, code)
}

func render(w http.ResponseWriter, r *http.Request, page templ.Component, code int) {
	w.WriteHeader(code)
	if err := page.Render(r.Context(), w); err != nil {
		WriteError(w, r, WriteErrorOpts{Err: err, Code: http.StatusInternalServerError})
	}
}

// Redirect navigates the client to loc whether the request came from htmx
// or a plain navigation: htmx requests get HX-Redirect + 204 (client-side
// navigation — an XHR-followed 303 would swap the target page into the
// current one instead), plain requests get a 303. Preferably use it for
// every redirect from handlers
func Redirect(w http.ResponseWriter, r *http.Request, loc string) {
	redirect(w, r, loc, http.StatusNoContent)
}

// RedirectWithCode is Redirect with the htmx-branch status overridden (the
// plain branch is always a 303 — a 4xx to a plain navigation only shows the
// browser's error page). RequireAuth uses it for 401 denials.
func RedirectWithCode(w http.ResponseWriter, r *http.Request, loc string, code int) {
	redirect(w, r, loc, code)
}

func redirect(w http.ResponseWriter, r *http.Request, loc string, htmxCode int) {
	htmx := r.Header.Get("HX-Request") == "true"
	slog.Default().InfoContext(r.Context(), "redirect",
		"method", r.Method, "path", r.URL.Path, "loc", loc, "htmx", htmx)
	if htmx {
		w.Header().Set("HX-Redirect", loc)
		w.WriteHeader(htmxCode)
		return
	}
	http.Redirect(w, r, loc, http.StatusSeeOther)
}

// ParseID reads the {id} path value as a positive integer.
func ParseID(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		return 0, errors.New("invalid id")
	}
	return id, nil
}
