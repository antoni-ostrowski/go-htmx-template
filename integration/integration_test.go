// Integration suite: critical paths only, driven through PocketBase's
// ApiScenario harness (real router + hooks, isolated temp SQLite per
// scenario). PB's own features (realtime, admin, API) are not tested here.
package integration_test

import (
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"go-htmx-template/handlers"
	"go-htmx-template/migrations"
	"go-htmx-template/server"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"go.opentelemetry.io/otel"
)

func testDeps() handlers.Deps {
	return handlers.Deps{
		Logger: slog.New(slog.DiscardHandler),
		Tel:    &handlers.Telemetry{Tracer: otel.Tracer("test"), Meter: otel.Meter("test")},
	}
}

func setupTestApp(t testing.TB) *tests.TestApp {
	testApp, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	if err := migrations.EnsureCollections(testApp); err != nil {
		t.Fatal(err)
	}
	server.BindAppHooks(testApp, testDeps())
	return testApp
}

func seedUser(t testing.TB, app *tests.TestApp, email string) *core.Record {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	rec := core.NewRecord(col)
	rec.Set("email", email)
	rec.Set("password", "password123")
	if err := app.Save(rec); err != nil {
		t.Fatal(err)
	}
	return rec
}

func seedTodo(t testing.TB, app *tests.TestApp, ownerID, id, title string) {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("todos")
	if err != nil {
		t.Fatal(err)
	}
	rec := core.NewRecord(col)
	rec.Set("id", id)
	rec.Set("title", title)
	rec.Set("done", false)
	rec.Set("owner", ownerID)
	if err := app.Save(rec); err != nil {
		t.Fatal(err)
	}
}

func tokenFor(t testing.TB, rec *core.Record) string {
	t.Helper()
	token, err := rec.NewAuthToken()
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func formBody(t testing.TB, values url.Values) *strings.Reader {
	t.Helper()
	return strings.NewReader(values.Encode())
}

const formHeaders = "application/x-www-form-urlencoded"

// Anonymous GET / renders sign-in entry points, no todo form.
func TestBasic_Anonymous(t *testing.T) {
	scenario := tests.ApiScenario{
		Name:   "anonymous page",
		Method: http.MethodGet,
		URL:    "/",
		Headers: map[string]string{
			"Content-Type": formHeaders,
		},
		ExpectedStatus:     http.StatusOK,
		ExpectedContent:    []string{"/signin"},
		NotExpectedContent: []string{`hx-post="/todos"`},
		TestAppFactory:     setupTestApp,
	}
	scenario.Test(t)
}

// Signup creates the user, sets the cookie, and bounces to /.
func TestBasic_Signup(t *testing.T) {
	scenario := tests.ApiScenario{
		Name:   "signup",
		Method: http.MethodPost,
		URL:    "/signup",
		Body: formBody(t, url.Values{
			"email":    {"alice@example.com"},
			"password": {"password123"},
		}),
		Headers: map[string]string{
			"Content-Type": formHeaders,
		},
		ExpectedStatus: http.StatusSeeOther,
		AfterTestFunc: func(t testing.TB, app *tests.TestApp, res *http.Response) {
			if loc := res.Header.Get("Location"); loc != "/" {
				t.Fatalf("Location = %q, want /", loc)
			}
			if cookie := res.Header.Get("Set-Cookie"); !strings.Contains(cookie, "pb_auth=") {
				t.Fatalf("missing pb_auth cookie: %q", cookie)
			}
			if _, err := app.FindAuthRecordByEmail("users", "alice@example.com"); err != nil {
				t.Fatalf("user not created: %v", err)
			}
		},
		TestAppFactory: setupTestApp,
	}
	scenario.Test(t)
}

// Duplicate signup is a 409 re-render, not a 500.
func TestBasic_SignupDuplicate(t *testing.T) {
	scenario := tests.ApiScenario{
		Name:   "duplicate signup",
		Method: http.MethodPost,
		URL:    "/signup",
		Body: formBody(t, url.Values{
			"email":    {"bob@example.com"},
			"password": {"password123"},
		}),
		Headers: map[string]string{
			"Content-Type": formHeaders,
		},
		ExpectedStatus:  http.StatusConflict,
		ExpectedContent: []string{"taken"},
		BeforeTestFunc: func(t testing.TB, app *tests.TestApp, e *core.ServeEvent) {
			seedUser(t, app, "bob@example.com")
		},
		TestAppFactory: setupTestApp,
	}
	scenario.Test(t)
}

// Authenticated add renders the new row.
func TestBasic_AddTodo(t *testing.T) {
	scenario := tests.ApiScenario{
		Name:   "add todo",
		Method: http.MethodPost,
		URL:    "/todos",
		Body: formBody(t, url.Values{
			"title": {"buy milk"},
		}),
		Headers: map[string]string{
			"Content-Type": formHeaders,
			// filled in BeforeTestFunc below (headers apply after it runs)
			"Authorization": "",
		},
		ExpectedStatus:  http.StatusOK,
		ExpectedContent: []string{"buy milk"},
		AfterTestFunc: func(t testing.TB, app *tests.TestApp, res *http.Response) {
			records, err := app.FindRecordsByFilter("todos", "title = 'buy milk'", "", 10, 0, nil)
			if err != nil || len(records) != 1 {
				t.Fatalf("todo not stored: %v %#v", err, records)
			}
		},
		TestAppFactory: setupTestApp,
	}
	scenario.BeforeTestFunc = func(t testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		scenario.Headers["Authorization"] = tokenFor(t, seedUser(t, app, "carol@example.com"))
	}
	scenario.Test(t)
}

// Toggle flips done on the owner's todo.
func TestBasic_ToggleTodo(t *testing.T) {
	const todoID = "testtodo0000001"
	scenario := tests.ApiScenario{
		Name:   "toggle todo",
		Method: http.MethodPost,
		URL:    "/todos/" + todoID + "/toggle",
		Headers: map[string]string{
			"Content-Type":  formHeaders,
			"Authorization": "",
		},
		ExpectedStatus: http.StatusOK,
		ExpectedContent: []string{"toggle me"},
		AfterTestFunc: func(t testing.TB, app *tests.TestApp, res *http.Response) {
			rec, err := app.FindRecordById("todos", todoID)
			if err != nil || !rec.GetBool("done") {
				t.Fatalf("todo not toggled: %v %#v", err, rec)
			}
		},
		TestAppFactory: setupTestApp,
	}
	scenario.BeforeTestFunc = func(t testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		user := seedUser(t, app, "dave@example.com")
		seedTodo(t, app, user.Id, todoID, "toggle me")
		scenario.Headers["Authorization"] = tokenFor(t, user)
	}
	scenario.Test(t)
}

// Delete removes the row and the re-rendered list drops the title.
func TestBasic_DeleteTodo(t *testing.T) {
	const todoID = "testtodo0000002"
	scenario := tests.ApiScenario{
		Name:   "delete todo",
		Method: http.MethodDelete,
		URL:    "/todos/" + todoID,
		Headers: map[string]string{
			"Content-Type":  formHeaders,
			"Authorization": "",
		},
		ExpectedStatus:     http.StatusOK,
		NotExpectedContent: []string{"delete me"},
		AfterTestFunc: func(t testing.TB, app *tests.TestApp, res *http.Response) {
			if _, err := app.FindRecordById("todos", todoID); err == nil {
				t.Fatal("todo still in db")
			}
		},
		TestAppFactory: setupTestApp,
	}
	scenario.BeforeTestFunc = func(t testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		user := seedUser(t, app, "erin@example.com")
		seedTodo(t, app, user.Id, todoID, "delete me")
		scenario.Headers["Authorization"] = tokenFor(t, user)
	}
	scenario.Test(t)
}

// Cookie auth (what browsers send) works for mutations, not just the
// Authorization header.
func TestBasic_CookieAuth(t *testing.T) {
	scenario := tests.ApiScenario{
		Name:   "cookie auth add",
		Method: http.MethodPost,
		URL:    "/todos",
		Body: formBody(t, url.Values{
			"title": {"via cookie"},
		}),
		Headers: map[string]string{
			"Content-Type": formHeaders,
			"Cookie":       "",
		},
		ExpectedStatus:  http.StatusOK,
		ExpectedContent: []string{"via cookie"},
		TestAppFactory:  setupTestApp,
	}
	scenario.BeforeTestFunc = func(t testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		user := seedUser(t, app, "frank@example.com")
		scenario.Headers["Cookie"] = "pb_auth=" + tokenFor(t, user)
	}
	scenario.Test(t)
}

// Anonymous mutation bounces to signin instead of leaking anything.
func TestBasic_RequireAuth(t *testing.T) {
	scenario := tests.ApiScenario{
		Name:   "anonymous add",
		Method: http.MethodPost,
		URL:    "/todos",
		Body: formBody(t, url.Values{
			"title": {"nope"},
		}),
		Headers: map[string]string{
			"Content-Type": formHeaders,
		},
		ExpectedStatus: http.StatusSeeOther,
		AfterTestFunc: func(t testing.TB, app *tests.TestApp, res *http.Response) {
			if loc := res.Header.Get("Location"); loc != "/signin" {
				t.Fatalf("Location = %q, want /signin", loc)
			}
		},
		TestAppFactory: setupTestApp,
	}
	scenario.Test(t)
}
