package session

import (
	"context"
	"net/http"
	"time"

	"github.com/xr0-org/progstack/internal/logging"
	"github.com/xr0-org/progstack/internal/model"
)

const (
	CtxSessionKey = "session"
)

type SessionService struct {
	store *model.Store
}

func NewSessionService(s *model.Store) *SessionService {
	return &SessionService{
		store: s,
	}
}

func (s *SessionService) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger := logging.Logger(r)

		cookie, err := r.Cookie(CookieName)
		if err != nil {
			logger.Printf("Error getting cookie: %v", err)
			/* create unauth session */
			session, err := CreateUnauthSession(
				s.store, w, unauthSessionDuration, logger,
			)
			if err != nil {
				logger.Printf("Error creating unauth session: %v", err)
				http.Error(w, "", http.StatusInternalServerError)
				return
			}
			ctx := context.WithValue(r.Context(), CtxSessionKey, session)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		/* cookie exists retrieve session */
		session, err := GetSession(s.store, w, cookie.Value, logger)
		if err != nil {
			logger.Printf("Error getting session: %v", err)
			/* expire cookie if error */
			http.SetCookie(w, &http.Cookie{
				Name:     CookieName,
				Value:    "",
				Expires:  time.Now().Add(-1 * time.Hour),
				Path:     "/",
				Secure:   true,
				HttpOnly: true,
			})
			http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
			return
		}
		if session.expiresAt.Before(time.Now()) {
			logger.Println("session expired")
			/* expire cookie if session expired */
			http.SetCookie(w, &http.Cookie{
				Name:     CookieName,
				Value:    "",
				Expires:  time.Now().Add(-1 * time.Hour),
				Path:     "/",
				Secure:   true,
				HttpOnly: true,
			})
			http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
			return
		}

		ctx := context.WithValue(r.Context(), CtxSessionKey, session)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
