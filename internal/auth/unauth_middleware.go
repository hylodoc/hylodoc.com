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

const unauthCookieName = "unauth_session_id"

/* UnauthMiddleware */

type UnauthMiddleware struct {
	store *model.Store
}

func NewUnauthMiddleware(s *model.Store) *UnauthMiddleware {
	return &UnauthMiddleware{
		store: s,
	}
}

func (ua *UnauthMiddleware) HandleUnauthSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Println("running unauth session middleware...")
		user, err := handleUnauthSession(w, r, ua.store)
		if err != nil {
			log.Println("error handling unauth session: ", err)
			http.Error(w, "", http.StatusInternalServerError)
			return
		}
		if user != nil {
			/* user logged in, add userinfo to context */
			r = r.WithContext(context.WithValue(r.Context(), CtxSessionKey, user))
		}
		next.ServeHTTP(w, r)
	})
}

func handleUnauthSession(w http.ResponseWriter, r *http.Request, s *model.Store) (*Session, error) {
	authCookie, err := r.Cookie(authCookieName)
	if err == nil {
		/* authCookie exists */
		log.Println("authCookie exists")
		session, err := validateAuthSessionId(authCookie.Value, w, s)
		if err == nil {
			return session, nil
		}
		/* if error try do unauth */
	}
	log.Println("authSession does not exist")
	unauthCookie, err := r.Cookie(unauthCookieName)
	if err != nil || unauthCookie.Value == "" {
		/* unauthCookie does not exist, generate one */
		log.Println("unauthCookie does not exist")
		return nil, createUnauthSession(w, s)
	}
	/* unauthCookie exists, check is valid */
	log.Println("unauthCookie exists")
	return nil, manageUnauthSession(unauthCookie.Value, w, s)
}

func createUnauthSession(w http.ResponseWriter, s *model.Store) error {
	/* generate unauthSessionId */
	unauthSessionId, err := GenerateToken()
	if err != nil {
		return fmt.Errorf("error generating unauthSessionId: %w", err)
	}
	/* check generated doesn't exist in db */
	_, err = s.GetUnauthSession(context.TODO(), unauthSessionId)
	if err == nil {
		return fmt.Errorf("error unauthSessionId already exists")
	} else {
		if err != sql.ErrNoRows {
			return err
		}
		/* can't find session, good */
	}
	/* create unauthSession in db */
	_, err = s.CreateUnauthSession(context.TODO(), unauthSessionId)
	if err != nil {
		return fmt.Errorf("error writing unauth cookie to db: %w", err)
	}
	/* set cookie */
	http.SetCookie(w, &http.Cookie{
		Name:     unauthCookieName,
		Value:    unauthSessionId,
		Path:     "/",
		Secure:   true,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(24 * time.Hour), /* XXX: make configurable */
	})
	return nil
}

func manageUnauthSession(unauthSessionId string, w http.ResponseWriter, s *model.Store) error {
	session, err := s.GetUnauthSession(context.TODO(), unauthSessionId)
	if err != nil {
		if err != sql.ErrNoRows {
			/* db error  */
			return err
		}
		log.Println("no unauth session, refreshing")
		/* no unauth session exists, delete it and create new unauth session */
		http.SetCookie(w, &http.Cookie{
			Name:    unauthCookieName,
			Value:   "",
			Path:    "/",
			Expires: time.Unix(0, 0),
			MaxAge:  -1,
		})
		return createUnauthSession(w, s)
	}
	/* found session, check if expired */
	if session.ExpiresAt.Before(time.Now()) {
		log.Println("unauth session expired, deleting cookie and refreshing")
		/* expired so set to inactive */
		err := s.EndUnauthSession(context.TODO(), unauthSessionId)
		if err != nil {
			return err
		}
		/* delete cookie */
		http.SetCookie(w, &http.Cookie{
			Name:    unauthCookieName,
			Value:   "",
			Path:    "/",
			Expires: time.Unix(0, 0),
			MaxAge:  -1,
		})
		/* create new unauth session */
		err = createUnauthSession(w, s)
		if err != nil {
			return fmt.Errorf("error creating unauth session: %w", err)
		}
	}
	return nil
}
