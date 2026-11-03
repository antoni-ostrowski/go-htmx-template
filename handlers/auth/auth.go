// Package auth holds sign-up, sign-in, and sign-out plus the password
// hashing helpers. Sessions carry user_id and username; handlers.Deps
// stays untouched.
package auth

import (
	"context"
	"net/http"
	"time"

	db "go-htmx-template/db/sqlc"
	"go-htmx-template/handlers"

	"github.com/alexedwards/scs/pgxstore"
	"github.com/alexedwards/scs/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

func NewSessionManager(pool *pgxpool.Pool) *scs.SessionManager {
	sessions := scs.New()
	sessions.Store = pgxstore.New(pool)
	sessions.Lifetime = 7 * 24 * 60 * time.Minute
	return sessions
}

func NewUserID() (uuid.UUID, error) {
	return uuid.NewV7()
}

// HashPassword bcrypt-hashes password for storage. Never store plaintext.
func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// CheckPassword reports whether password matches the stored hash.
func CheckPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// login stores the user in a fresh session. RenewToken first so a login
// never inherits a pre-login session (fixation).
func login(d handlers.Deps, r *http.Request, user db.User) error {
	ctx := r.Context()
	if err := d.Sessions.RenewToken(ctx); err != nil {
		return err
	}
	d.Sessions.Put(ctx, "user_id", user.ID.String())
	d.Sessions.Put(ctx, "username", user.Username)
	return nil
}

type AuthData struct {
	UserID   uuid.UUID
	Username string
}

func (a AuthData) GetUserData(ctx context.Context, d handlers.Deps) (db.User, error) {
	return d.Queries.GetUserById(ctx, a.UserID)
}

type AuthedHandler func(w http.ResponseWriter, r *http.Request, a AuthData)

// WithAuth supplies request identity to next and nothing else. Anonymous
// requests get zero AuthData, never a redirect. This is the single place
// that reads the session for identity.
func WithAuth(next AuthedHandler, d handlers.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		var a AuthData
		if id, err := uuid.Parse(d.Sessions.GetString(ctx, "user_id")); err == nil {
			a = AuthData{UserID: id, Username: d.Sessions.GetString(ctx, "username")}
		}
		next(w, r, a)
	}
}

// RequireAuth enforces identity: anonymous requests go to /signin via
// [handlers.RedirectWithCode].
func RequireAuth(next AuthedHandler, d handlers.Deps) http.HandlerFunc {
	return WithAuth(func(w http.ResponseWriter, r *http.Request, a AuthData) {
		if a.UserID == uuid.Nil {
			handlers.RedirectWithCode(w, r, "/signin", http.StatusUnauthorized)
			return
		}
		next(w, r, a)
	}, d)
}
