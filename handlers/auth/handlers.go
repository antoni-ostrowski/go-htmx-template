// Route table and HTTP handlers for sign-up, sign-in, and sign-out.
// Identity and cookie helpers live in auth.go.
package auth

import (
	"net/http"
	"strings"

	"go-htmx-template/handlers"
	"go-htmx-template/templates"

	"github.com/pocketbase/pocketbase/core"
)

// Register wires the auth routes onto the PB router inside OnServe.
func Register(se *core.ServeEvent, d handlers.Deps) {
	se.Router.GET("/signup", showSignup(d))
	se.Router.POST("/signup", handleSignup(d))
	se.Router.GET("/signin", showSignin(d))
	se.Router.POST("/signin", handleSignin(d))
	se.Router.POST("/signout", handleSignout(d))
}

func showSignup(d handlers.Deps) func(e *core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		return handlers.Render(e, templates.Signup(""))
	}
}

func handleSignup(d handlers.Deps) func(e *core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		email := strings.TrimSpace(e.Request.FormValue("email"))
		password := e.Request.FormValue("password")
		if email == "" {
			return handlers.RenderWithCode(e, templates.Signup("email cannot be empty"), http.StatusUnprocessableEntity)
		}
		if len(password) < 8 {
			return handlers.RenderWithCode(e, templates.Signup("password must be at least 8 characters"), http.StatusUnprocessableEntity)
		}
		app := e.App
		if _, err := app.FindAuthRecordByEmail("users", email); err == nil {
			return handlers.RenderWithCode(e, templates.Signup("email is taken"), http.StatusConflict)
		}
		collection, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return handlers.WriteError(e, handlers.WriteErrorOpts{Err: err, Code: http.StatusInternalServerError})
		}
		record := core.NewRecord(collection)
		record.Set("email", email)
		record.Set("password", password)
		if err := app.Save(record); err != nil {
			// Usually a validation error (bad email format, duplicate race).
			d.Logger.DebugContext(e.Request.Context(), "signup save failed", "err", err)
			return handlers.RenderWithCode(e, templates.Signup("could not create account"), http.StatusUnprocessableEntity)
		}
		if err := login(e, record); err != nil {
			return handlers.WriteError(e, handlers.WriteErrorOpts{Err: err, Code: http.StatusInternalServerError})
		}
		e.Response.Header().Set("Clear-Site-Data", `"cache"`)
		return handlers.Redirect(e, "/")
	}
}

func showSignin(d handlers.Deps) func(e *core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		return handlers.Render(e, templates.Signin(""))
	}
}

func handleSignin(d handlers.Deps) func(e *core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		email := strings.TrimSpace(e.Request.FormValue("email"))
		password := e.Request.FormValue("password")
		record, err := e.App.FindAuthRecordByEmail("users", email)
		if err != nil || !record.ValidatePassword(password) {
			return handlers.RenderWithCode(e, templates.Signin("invalid email or password"), http.StatusUnauthorized)
		}
		if err := login(e, record); err != nil {
			return handlers.WriteError(e, handlers.WriteErrorOpts{Err: err, Code: http.StatusInternalServerError})
		}
		e.Response.Header().Set("Clear-Site-Data", `"cache"`)
		return handlers.Redirect(e, "/")
	}
}

func handleSignout(d handlers.Deps) func(e *core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		logout(e)
		e.Response.Header().Set("Clear-Site-Data", `"cache"`)
		return handlers.Redirect(e, "/signin")
	}
}
