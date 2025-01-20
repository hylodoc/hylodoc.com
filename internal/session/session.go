package session

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/knuthic/knuthic/internal/model"
)

const (
	CookieName = "session_id"
)

var (
	unauthSessionDuration = time.Now().Add(24 * time.Hour)
	authSessionDuration   = time.Now().Add(7 * 24 * time.Hour)
)

type Session struct {
	id              uuid.UUID
	userID          *string
	email           *string
	username        *string
	expiresAt       time.Time
	isAuthenticated bool

	*log.Logger
}

func CreateUnauthSession(
	s *model.Store, w http.ResponseWriter, expiresAt time.Time,
	logger *anonymousLogger,
) (*Session, error) {
	logger.Println("Creating unauth session...")

	unauthSession, err := s.CreateUnauthSession(context.TODO(), expiresAt)
	if err != nil {
		return nil, err
	}

	sessionid := unauthSession.ID.String()
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    sessionid,
		Expires:  unauthSession.ExpiresAt,
		Path:     "/",
		Secure:   true,
		HttpOnly: true,
	})

	return &Session{
		unauthSession.ID,
		nil,
		nil,
		nil,
		unauthSession.ExpiresAt,
		false,
		logger.toSessionLogger(sessionid),
	}, nil
}

func createAuthSession(
	s *model.Store, w http.ResponseWriter, userID string,
	expiresAt time.Time, logger *anonymousLogger,
) (*Session, error) {
	logger.Println("Creating auth session...")

	authSession, err := s.CreateAuthSession(
		context.TODO(),
		model.CreateAuthSessionParams{
			UserID:    userID,
			ExpiresAt: expiresAt,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("error creating auth session: %w", err)
	}
	row, err := s.GetAuthSession(context.TODO(), authSession.ID)
	if err != nil {
		return nil, fmt.Errorf("error getting auth session: %w", err)
	}

	sessionid := authSession.ID.String()
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    sessionid,
		Path:     "/",
		Secure:   true,
		HttpOnly: true,
		SameSite: http.SameSiteNoneMode,
		Expires:  authSessionDuration, /* XXX: make configurable */
	})

	return convertRowToSession(row, logger.toSessionLogger(sessionid)), nil
}

func (sesh *Session) Authenticate(
	s *model.Store, w http.ResponseWriter, userID string,
	expiresAt time.Time,
) (*Session, error) {
	return createAuthSession(
		s, w, userID, expiresAt, newAnonymousLogger(sesh.GetSessionID()),
	)
}

func convertRowToSession(
	row model.GetAuthSessionRow, logger *log.Logger,
) *Session {
	return &Session{
		row.ID,
		&row.UserID,
		&row.Email,
		&row.Username,
		row.ExpiresAt,
		true,
		logger,
	}
}

func (sesh *Session) End(s *model.Store) error {
	sesh.Println("ending auth session...")

	return s.EndAuthSession(context.TODO(), sesh.id)
}

func GetSession(
	s *model.Store, w http.ResponseWriter, sessionID string,
	logger *anonymousLogger,
) (*Session, error) {
	/* parse sessionID from cookie */
	uuid, err := uuid.Parse(sessionID)
	if err != nil {
		return nil, fmt.Errorf("error parsing sessionID: %w", err)
	}

	/* try get auth session */
	auth, err := s.GetAuthSession(context.TODO(), uuid)
	if err == nil {
		logger.Printf("Found auth session\n")
		return convertRowToSession(
			auth, logger.toSessionLogger(auth.ID.String()),
		), nil
	}
	if err != sql.ErrNoRows {
		return nil, fmt.Errorf("error checking for auth session: %w", err)
	}

	/* try get unauth session */
	unauth, err := s.GetUnauthSession(context.TODO(), uuid)
	if err == nil {
		logger.Printf("Found unauth session\n")
		return &Session{
			unauth.ID,
			nil,
			nil,
			nil,
			unauth.ExpiresAt,
			false,
			logger.toSessionLogger(unauth.ID.String()),
		}, nil
	}
	if err != sql.ErrNoRows {
		return nil, fmt.Errorf("error checking for unauth session: %w", err)
	}
	return nil, fmt.Errorf("no unauth session: %w", err)
}

/* both */
func (s *Session) GetSessionID() string {
	return s.id.String()
}

func (s *Session) IsAuthenticated() bool {
	return s.isAuthenticated
}

func (s *Session) GetExpiresAt() time.Time {
	return s.expiresAt
}

/* auth session */
func (s *Session) GetEmail() string {
	if s.IsAuthenticated() {
		return *s.email
	}
	return ""
}

func (s *Session) GetUserID() (string, error) {
	if s.IsAuthenticated() {
		return *s.userID, nil
	}
	return "", fmt.Errorf("unauthenticated")
}

func (s *Session) GetUsername() string {
	if s.IsAuthenticated() {
		return *s.username
	}
	return ""
}
