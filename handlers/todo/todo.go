// Package todo holds the todo domain's routes and handlers.
package todo

import (
	"errors"
	"net/http"
	"strings"

	db "go-htmx-todo/db/sqlc"
	"go-htmx-todo/handlers"
	"go-htmx-todo/handlers/auth"
	"go-htmx-todo/templates"

	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// Register wires the todo routes onto mux. Mutations require identity;
// the page is public and renders sign-in links when anonymous.
func Register(mux *http.ServeMux, d handlers.Deps) {
	handlers.Route(mux, "GET /{$}", auth.WithAuth(handlePage(d), d))
	handlers.Route(mux, "POST /todos", auth.RequireAuth(handleAdd(d), d))
	handlers.Route(mux, "POST /todos/{id}/toggle", auth.RequireAuth(handleToggle(d), d))
	handlers.Route(mux, "DELETE /todos/{id}", auth.RequireAuth(handleDelete(d), d))
}

func handlePage(d handlers.Deps) auth.AuthedHandler {
	return func(w http.ResponseWriter, r *http.Request, a auth.AuthData) {
		ctx := r.Context()
		todos, err := d.Queries.ListTodos(ctx, a.UserID)
		if err != nil {
			handlers.WriteError(w, r, handlers.WriteErrorOpts{Err: err, Code: http.StatusInternalServerError})
			return
		}
		handlers.Render(w, r, templates.Page(todos, a.Username))
	}
}

func handleAdd(d handlers.Deps) auth.AuthedHandler {
	return func(w http.ResponseWriter, r *http.Request, a auth.AuthData) {
		handlers.WriteError(w, r, handlers.WriteErrorOpts{Err: errors.New("test err"), Code: http.StatusUnprocessableEntity})
		return
		title := strings.TrimSpace(r.FormValue("title"))
		if title == "" {
			handlers.WriteError(w, r, handlers.WriteErrorOpts{Err: errors.New("title cannot be empty"), Code: http.StatusUnprocessableEntity})
			return
		}
		ctx := r.Context()
		if _, err := d.Queries.CreateTodo(ctx, db.CreateTodoParams{UserID: a.UserID, Title: title}); err != nil {
			handlers.WriteError(w, r, handlers.WriteErrorOpts{Err: err, Code: http.StatusInternalServerError})
			return
		}
		todos, err := d.Queries.ListTodos(ctx, a.UserID)
		if err != nil {
			handlers.WriteError(w, r, handlers.WriteErrorOpts{Err: err, Code: http.StatusInternalServerError})
			return
		}
		handlers.Render(w, r, templates.List(todos))
	}
}

func handleToggle(d handlers.Deps) auth.AuthedHandler {
	return func(w http.ResponseWriter, r *http.Request, a auth.AuthData) {
		id, err := handlers.ParseID(r)
		if err != nil {
			handlers.WriteError(w, r, handlers.WriteErrorOpts{Err: err, Code: http.StatusBadRequest})
			return
		}
		ctx := r.Context()
		trace.SpanFromContext(ctx).SetAttributes(attribute.Int64("id", id))
		t, err := d.Queries.GetTodo(ctx, db.GetTodoParams{ID: id, UserID: a.UserID})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				handlers.WriteError(w, r, handlers.WriteErrorOpts{Err: errors.New("todo not found"), Code: http.StatusNotFound})
				return
			}
			handlers.WriteError(w, r, handlers.WriteErrorOpts{Err: err, Code: http.StatusInternalServerError})
			return
		}
		if err := d.Queries.UpdateTodo(ctx, db.UpdateTodoParams{
			ID: t.ID, UserID: t.UserID, Title: t.Title, Done: !t.Done,
		}); err != nil {
			handlers.WriteError(w, r, handlers.WriteErrorOpts{Err: err, Code: http.StatusInternalServerError})
			return
		}
		todos, err := d.Queries.ListTodos(ctx, a.UserID)
		if err != nil {
			handlers.WriteError(w, r, handlers.WriteErrorOpts{Err: err, Code: http.StatusInternalServerError})
			return
		}
		handlers.Render(w, r, templates.List(todos))
	}
}

func handleDelete(d handlers.Deps) auth.AuthedHandler {
	return func(w http.ResponseWriter, r *http.Request, a auth.AuthData) {
		id, err := handlers.ParseID(r)
		if err != nil {
			handlers.WriteError(w, r, handlers.WriteErrorOpts{Err: err, Code: http.StatusBadRequest})
			return
		}
		ctx := r.Context()
		trace.SpanFromContext(ctx).SetAttributes(attribute.Int64("id", id))
		deleteCtx, deleteSpan := d.Tel.Tracer.Start(ctx, "db.delete")
		defer deleteSpan.End()
		if err := d.Queries.DeleteTodo(deleteCtx, db.DeleteTodoParams{ID: id, UserID: a.UserID}); err != nil {
			handlers.WriteError(w, r, handlers.WriteErrorOpts{Err: err, Code: http.StatusInternalServerError, Spans: []trace.Span{deleteSpan}})
			return
		}
		todos, err := d.Queries.ListTodos(ctx, a.UserID)
		if err != nil {
			handlers.WriteError(w, r, handlers.WriteErrorOpts{Err: err, Code: http.StatusInternalServerError})
			return
		}
		handlers.Render(w, r, templates.List(todos))
	}
}
