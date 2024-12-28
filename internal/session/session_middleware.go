package session

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/xr0-org/progstack/internal/model"
)

func NewSession(
	w http.ResponseWriter, r *http.Request,
	logger *log.Logger, s *model.Store,
) (*Session, error) {
	cookie, err := r.Cookie(CookieName)
	if err != nil {
		if !errors.Is(err, http.ErrNoCookie) {
			logger.Printf("error getting cookie: %v", err)
		}
		sesh, err := createUnauthSession(s, w, logger)
		if err != nil {
			return nil, fmt.Errorf("create unauth session: %w", err)
		}
		return sesh, nil
	}

	sesh, err := getUnexpiredSession(cookie.Value, logger, s)
	if err != nil {
		/* expire cookie */
		http.SetCookie(w, &http.Cookie{
			Name:     CookieName,
			Value:    "",
			Expires:  time.Now().Add(-1 * time.Hour),
			Path:     "/",
			Secure:   true,
			HttpOnly: true,
		})
		/* http.Redirect(w, r, "/", http.StatusTemporaryRedirect) */
		return nil, fmt.Errorf("get unexpired session: %w", err)
	}
	return sesh, nil
}

func getUnexpiredSession(
	id string, logger *log.Logger, s *model.Store,
) (*Session, error) {
	sesh, err := getSession(s, id, logger)
	if err != nil {
		return nil, fmt.Errorf("get session: %w", err)
	}
	if sesh.expiresAt.Before(time.Now()) {
		return nil, fmt.Errorf("expired: %w", err)
	}
	return sesh, nil
}
