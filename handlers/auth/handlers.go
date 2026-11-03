// Route table and HTTP handlers for sign-up, sign-in, and sign-out.
// Identity, sessions, and password helpers live in auth.go.
package auth

import (
	"errors"
	"net/http"
	"strings"

	db "go-htmx-template/db/sqlc"
	"go-htmx-template/handlers"
	"go-htmx-template/templates"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Register wires the auth routes onto mux.
func Register(mux *http.ServeMux, d handlers.Deps) {
	handlers.Route(mux, "GET /signup", showSignup(d))
	handlers.Route(mux, "POST /signup", handleSignup(d))
	handlers.Route(mux, "GET /signin", showSignin(d))
	handlers.Route(mux, "POST /signin", handleSignin(d))
	handlers.Route(mux, "POST /signout", handleSignout(d))
}

func showSignup(d handlers.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		handlers.Render(w, r, templates.Signup(""))
	}
}

func handleSignup(d handlers.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		username := strings.TrimSpace(r.FormValue("username"))
		password := r.FormValue("password")
		if username == "" {
			handlers.RenderWithCode(w, r, templates.Signup("username cannot be empty"), http.StatusUnprocessableEntity)
			return
		}
		if len(password) < 8 {
			handlers.RenderWithCode(w, r, templates.Signup("password must be at least 8 characters"), http.StatusUnprocessableEntity)
			return
		}
		hash, err := HashPassword(password)
		if err != nil {
			handlers.WriteError(w, r, handlers.WriteErrorOpts{Err: err, Code: http.StatusInternalServerError})
			return
		}
		id := NewUserID()
		user, err := d.Queries.CreateUser(ctx, db.CreateUserParams{
			ID: id, Username: username, PasswordHash: hash,
		})
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				handlers.RenderWithCode(w, r, templates.Signup("username is taken"), http.StatusConflict)
				return
			}
			handlers.WriteError(w, r, handlers.WriteErrorOpts{Err: err, Code: http.StatusInternalServerError})
			return
		}
		if err := login(d, r, user); err != nil {
			handlers.WriteError(w, r, handlers.WriteErrorOpts{Err: err, Code: http.StatusInternalServerError})
			return
		}
		w.Header().Set("Clear-Site-Data", `"cache"`)
		handlers.Redirect(w, r, "/")
	}
}

func showSignin(d handlers.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		handlers.Render(w, r, templates.Signin(""))
	}
}

func handleSignin(d handlers.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		user, err := d.Queries.GetUserByUsername(ctx, strings.TrimSpace(r.FormValue("username")))
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				handlers.RenderWithCode(w, r, templates.Signin("invalid username or password"), http.StatusUnauthorized)
				return
			}
			handlers.WriteError(w, r, handlers.WriteErrorOpts{Err: err, Code: http.StatusInternalServerError})
			return
		}
		if !CheckPassword(user.PasswordHash, r.FormValue("password")) {
			handlers.RenderWithCode(w, r, templates.Signin("invalid username or password"), http.StatusUnauthorized)
			return
		}
		if err := login(d, r, user); err != nil {
			handlers.WriteError(w, r, handlers.WriteErrorOpts{Err: err, Code: http.StatusInternalServerError})
			return
		}
		w.Header().Set("Clear-Site-Data", `"cache"`)
		handlers.Redirect(w, r, "/")
	}
}

func handleSignout(d handlers.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := d.Sessions.Destroy(r.Context()); err != nil {
			handlers.WriteError(w, r, handlers.WriteErrorOpts{Err: err, Code: http.StatusInternalServerError})
			return
		}
		w.Header().Set("Clear-Site-Data", `"cache"`)
		handlers.Redirect(w, r, "/signin")
	}
}
