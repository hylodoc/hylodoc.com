package session

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/xr0-org/progstack/internal/model"
)

const (
	CtxSessionKey = "session"
)

type SessionMiddleware struct {
	store *model.Store
}

func NewSessionMiddleware(s *model.Store) *SessionMiddleware {
	return &SessionMiddleware{
		store: s,
	}
}

func (a *SessionMiddleware) SessionMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Println("running session middleware...")

		cookie, err := r.Cookie(CookieName)
		if err != nil {
			/* create unauth session */
			session, err := CreateUnauthSession(a.store, w, unauthSessionDuration)
			if err != nil {
				log.Printf("error creating unauth session: %v", err)
				http.Error(w, "", http.StatusInternalServerError)
				return
			}
			ctx := context.WithValue(r.Context(), CtxSessionKey, session)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		/* cookie exists retrieve session */
		session, err := GetSession(a.store, w, cookie.Value)
		if err != nil {
			log.Printf("error getting session: %v", err)
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
			log.Println("session expired")
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
