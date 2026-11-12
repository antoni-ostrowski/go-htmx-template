// Package auth holds sign-up, sign-in, and sign-out plus the identity
// middleware. Identity is the PB auth record itself: e.Auth when the request
// carries an Authorization header (PB's own loader, also used by realtime
// and the API), else the pb_auth cookie set by login. Sessions carry
// nothing server-side — the JWT is stateless.
package auth

import (
	"net/http"

	"go-htmx-template/handlers"

	"github.com/pocketbase/pocketbase/core"
)

// CookieName is the readable (non-HttpOnly) cookie holding the PB auth
// token. Readable so realtime.js can reuse it for /api/realtime
// subscriptions. XSS could steal it like any bearer token — set HttpOnly +
// inline the token into pages instead if that worries you.
const CookieName = "pb_auth"

const cookieMaxAge = 7 * 24 * 60 * 60

// login issues a fresh auth token for record and stores it in the cookie.
func login(e *core.RequestEvent, record *core.Record) error {
	token, err := record.NewAuthToken()
	if err != nil {
		return err
	}
	http.SetCookie(e.Response, &http.Cookie{
		Name:     CookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   cookieMaxAge,
		SameSite: http.SameSiteLaxMode,
	})
	return nil
}

// logout clears the auth cookie. PB tokens are stateless, so there is no
// server-side session to destroy — forgetting the token is the logout.
func logout(e *core.RequestEvent) {
	http.SetCookie(e.Response, &http.Cookie{
		Name:     CookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		SameSite: http.SameSiteLaxMode,
	})
}

// WithAuth ensures e.Auth is populated, then calls next. PB's own loader
// already fills e.Auth from the Authorization header (API + realtime
// clients); this adds the pb_auth cookie fallback for browser page/form
// posts. Guests keep nil e.Auth, never a redirect — that is RequireAuth.
func WithAuth(next func(e *core.RequestEvent) error) func(e *core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		if e.Auth == nil {
			if c, err := e.Request.Cookie(CookieName); err == nil && c.Value != "" {
				if rec, err := e.App.FindAuthRecordByToken(c.Value, core.TokenTypeAuth); err == nil {
					e.Auth = rec
				}
			}
		}
		return next(e)
	}
}

// RequireAuth enforces identity: anonymous requests go to /signin via
// [handlers.RedirectWithCode]. Handlers behind it can use e.Auth directly.
func RequireAuth(next func(e *core.RequestEvent) error) func(e *core.RequestEvent) error {
	return WithAuth(func(e *core.RequestEvent) error {
		if e.Auth == nil {
			return handlers.RedirectWithCode(e, "/signin", http.StatusUnauthorized)
		}
		return next(e)
	})
}
