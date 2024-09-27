package auth

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/xr0-org/progstack/internal/model"
)

/* AuthMiddleware */

type AuthMiddleware struct {
	store *model.Store
}

func NewAuthMiddleware(s *model.Store) *AuthMiddleware {
	return &AuthMiddleware{
		store: s,
	}
}

func (a *AuthMiddleware) ValidateAuthSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Println("running auth session middleware...")

		session, err := validateAuthSession(w, r, a.store)
		if err != nil {
			http.Redirect(w, r, "/", http.StatusFound)
			return
		}
		ctx := context.WithValue(r.Context(), CtxSessionKey, session)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func validateAuthSession(w http.ResponseWriter, r *http.Request, s *model.Store) (*Session, error) {
	cookie, err := r.Cookie(authCookieName)
	authSessionId := cookie.Value
	if err != nil || authSessionId == "" {
		return nil, fmt.Errorf("error reading auth cookie")
	}
	session, err := validateAuthSessionId(authSessionId, w, s)
	if err != nil {
		log.Println("error validating authSessionId: ", err)
		return nil, fmt.Errorf("error validating auth session id: %w", err)
	}
	return session, nil
}

type Session struct {
	UserID   int32  `json:"user_id"`
	Email    string `json:"email"`
	Username string `json:"login"`
}

func validateAuthSessionId(sessionId string, w http.ResponseWriter, s *model.Store) (*Session, error) {
	session, err := s.GetSession(context.TODO(), sessionId)
	if err != nil {
		if err != sql.ErrNoRows {
			/* db error */
			return nil, err
		}
		/* no auth session exists, delete auth cookie */
		http.SetCookie(w, &http.Cookie{
			Name:    authCookieName,
			Value:   "",
			Path:    "/",
			Expires: time.Unix(0, 0),
			MaxAge:  -1,
		})
		return nil, err
	}
	if session.ExpiresAt.Before(time.Now()) {
		log.Println("auth token expired")
		/* expired session in db */
		err := s.EndSession(context.TODO(), sessionId)
		if err != nil {
			return nil, err
		}
		/* delete cookie */
		http.SetCookie(w, &http.Cookie{
			Name:    authCookieName,
			Value:   "",
			Path:    "/",
			Expires: time.Unix(0, 0),
			MaxAge:  -1,
		})
		return nil, fmt.Errorf("session expired")
	}
	return &Session{
		UserID: session.UserID,
	}, nil
}
