package integration_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/google/uuid"

	db "go-htmx-template/db/sqlc"
)

func doSignup(t *testing.T, app http.Handler, username, password string) (*http.Cookie, *httptest.ResponseRecorder) {
	t.Helper()
	rec := Do(t, app, http.MethodPost, "/signup",
		url.Values{"username": {username}, "password": {password}}, nil, nil)
	var cookie *http.Cookie
	if cookies := rec.Result().Cookies(); len(cookies) > 0 {
		cookie = cookies[0]
	}
	return cookie, rec
}

func userID(t *testing.T, q *db.Queries, username string) uuid.UUID {
	t.Helper()
	u, err := q.GetUserByUsername(context.Background(), username)
	if err != nil {
		t.Fatalf("GetUserByUsername: %v", err)
	}
	return u.ID
}

// Happy path: signup -> add todo -> toggle -> delete.
func TestBasic_TodoFlow(t *testing.T) {
	app, q, _, _ := setup(t)

	cookie, rec := doSignup(t, app, "alice", "password123")
	WantCode(t, rec, http.StatusSeeOther)
	if cookie == nil {
		t.Fatal("no session cookie set on signup")
	}

	rec = Do(t, app, http.MethodPost, "/todos", url.Values{"title": {"buy milk"}}, cookie, nil)
	WantCode(t, rec, http.StatusOK)
	WantBody(t, rec, "buy milk")

	todos := listDB(t, q, userID(t, q, "alice"))
	if len(todos) != 1 || todos[0].Done {
		t.Fatalf("db = %#v, want one open todo", todos)
	}

	rec = Do(t, app, http.MethodPost, "/todos/1/toggle", nil, cookie, nil)
	WantCode(t, rec, http.StatusOK)

	rec = Do(t, app, http.MethodDelete, "/todos/1", nil, cookie, nil)
	WantCode(t, rec, http.StatusOK)
	WantNoBody(t, rec, "buy milk")
}

// Anonymous GET / renders sign-in entry points, no todo form.
func TestBasic_Anonymous(t *testing.T) {
	app, _, _, _ := setup(t)

	rec := Do(t, app, http.MethodGet, "/", nil, nil, nil)
	WantCode(t, rec, http.StatusOK)
	WantBody(t, rec, "/signin")
	WantNoBody(t, rec, `hx-post="/todos"`)
}
