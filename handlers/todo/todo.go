// Package todo holds the todo domain's routes and handlers. Handlers behind
// RequireAuth use e.Auth (the users record) directly.
package todo

import (
	"errors"
	"net/http"
	"strings"

	"go-htmx-template/handlers"
	"go-htmx-template/handlers/auth"
	"go-htmx-template/templates"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// Register wires the todo routes onto the PB router inside OnServe.
// Mutations require identity; the page is public and renders sign-in links
// when anonymous.
func Register(se *core.ServeEvent, d handlers.Deps) {
	se.Router.GET("/", auth.WithAuth(handlePage(d)))
	se.Router.GET("/todos-fragment", auth.RequireAuth(handleFragment(d)))
	se.Router.POST("/todos", auth.RequireAuth(handleAdd(d)))
	se.Router.POST("/todos/{id}/toggle", auth.RequireAuth(handleToggle(d)))
	se.Router.DELETE("/todos/{id}", auth.RequireAuth(handleDelete(d)))
}

// listTodos returns the owner's todos, oldest first. Server-side Go calls
// bypass PB API rules, so the owner filter here is the access control.
func listTodos(app core.App, ownerID string) ([]templates.Todo, error) {
	records, err := app.FindRecordsByFilter(
		"todos",
		"owner = {:owner}",
		"-created",
		200, 0,
		dbx.Params{"owner": ownerID},
	)
	if err != nil {
		return nil, err
	}
	out := make([]templates.Todo, 0, len(records))
	for _, r := range records {
		out = append(out, templates.Todo{
			ID:    r.Id,
			Title: r.GetString("title"),
			Done:  r.GetBool("done"),
		})
	}
	return out, nil
}

// ownedTodo loads the todo and proves it belongs to the caller. Missing or
// foreign records both read as 404 so IDs can't be probed.
func ownedTodo(app core.App, id, ownerID string) (*core.Record, error) {
	if id == "" {
		return nil, errors.New("todo not found")
	}
	t, err := app.FindRecordById("todos", id)
	if err != nil || t.GetString("owner") != ownerID {
		return nil, errors.New("todo not found")
	}
	return t, nil
}

func handlePage(d handlers.Deps) func(e *core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		if e.Auth == nil {
			return handlers.Render(e, templates.Page(nil, ""))
		}
		todos, err := listTodos(e.App, e.Auth.Id)
		if err != nil {
			return handlers.WriteError(e, handlers.WriteErrorOpts{Err: err, Code: http.StatusInternalServerError})
		}
		return handlers.Render(e, templates.Page(todos, e.Auth.GetString("email")))
	}
}

// handleFragment re-renders just the list. The realtime.js bridge refetches
// it on PB realtime events (triggered as pb-todos-changed).
func handleFragment(d handlers.Deps) func(e *core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		todos, err := listTodos(e.App, e.Auth.Id)
		if err != nil {
			return handlers.WriteError(e, handlers.WriteErrorOpts{Err: err, Code: http.StatusInternalServerError})
		}
		return handlers.Render(e, templates.List(todos))
	}
}

func handleAdd(d handlers.Deps) func(e *core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		title := strings.TrimSpace(e.Request.FormValue("title"))
		if title == "" {
			return handlers.WriteError(e, handlers.WriteErrorOpts{Err: errors.New("title cannot be empty"), Code: http.StatusUnprocessableEntity})
		}
		collection, err := e.App.FindCollectionByNameOrId("todos")
		if err != nil {
			return handlers.WriteError(e, handlers.WriteErrorOpts{Err: err, Code: http.StatusInternalServerError})
		}
		record := core.NewRecord(collection)
		record.Set("title", title)
		record.Set("done", false)
		record.Set("owner", e.Auth.Id)
		if err := e.App.Save(record); err != nil {
			return handlers.WriteError(e, handlers.WriteErrorOpts{Err: err, Code: http.StatusInternalServerError})
		}
		todos, err := listTodos(e.App, e.Auth.Id)
		if err != nil {
			return handlers.WriteError(e, handlers.WriteErrorOpts{Err: err, Code: http.StatusInternalServerError})
		}
		return handlers.Render(e, templates.List(todos))
	}
}

func handleToggle(d handlers.Deps) func(e *core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		id := e.Request.PathValue("id")
		trace.SpanFromContext(e.Request.Context()).SetAttributes(attribute.String("id", id))
		t, err := ownedTodo(e.App, id, e.Auth.Id)
		if err != nil {
			return handlers.WriteError(e, handlers.WriteErrorOpts{Err: errors.New("todo not found"), Code: http.StatusNotFound})
		}
		t.Set("done", !t.GetBool("done"))
		if err := e.App.Save(t); err != nil {
			return handlers.WriteError(e, handlers.WriteErrorOpts{Err: err, Code: http.StatusInternalServerError})
		}
		todos, err := listTodos(e.App, e.Auth.Id)
		if err != nil {
			return handlers.WriteError(e, handlers.WriteErrorOpts{Err: err, Code: http.StatusInternalServerError})
		}
		return handlers.Render(e, templates.List(todos))
	}
}

func handleDelete(d handlers.Deps) func(e *core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		id := e.Request.PathValue("id")
		trace.SpanFromContext(e.Request.Context()).SetAttributes(attribute.String("id", id))
		_, deleteSpan := d.Tel.Tracer.Start(e.Request.Context(), "db.delete")
		defer deleteSpan.End()
		t, err := ownedTodo(e.App, id, e.Auth.Id)
		if err != nil {
			return handlers.WriteError(e, handlers.WriteErrorOpts{Err: errors.New("todo not found"), Code: http.StatusNotFound})
		}
		if err := e.App.Delete(t); err != nil {
			return handlers.WriteError(e, handlers.WriteErrorOpts{Err: err, Code: http.StatusInternalServerError, Spans: []trace.Span{deleteSpan}})
		}
		todos, err := listTodos(e.App, e.Auth.Id)
		if err != nil {
			return handlers.WriteError(e, handlers.WriteErrorOpts{Err: err, Code: http.StatusInternalServerError})
		}
		return handlers.Render(e, templates.List(todos))
	}
}
