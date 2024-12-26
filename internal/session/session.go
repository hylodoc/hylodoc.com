package session

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/xr0-org/progstack/internal/model"
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
	userID          *int32
	email           *string
	username        *string
	githubLinked    bool
	githubEmail     *string
	expiresAt       time.Time
	isAuthenticated bool
}

func CreateUnauthSession(
	s *model.Store, w http.ResponseWriter, expiresAt time.Time,
	logger *log.Logger,
) (*Session, error) {
	logger.Println("Creating unauth session...")

	unauthSession, err := s.CreateUnauthSession(context.TODO(), expiresAt)
	if err != nil {
		return nil, err
	}

	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    unauthSession.ID.String(),
		Expires:  unauthSession.ExpiresAt,
		Path:     "/",
		Secure:   true,
		HttpOnly: true,
	})

	return &Session{
		id:              unauthSession.ID,
		expiresAt:       unauthSession.ExpiresAt,
		isAuthenticated: false,
	}, nil
}

func CreateAuthSession(
	s *model.Store, w http.ResponseWriter, userID int32, expiresAt time.Time,
	logger *log.Logger,
) (*Session, error) {
	logger.Println("Creating auth session...")

	authSession, err := s.CreateAuthSession(context.TODO(), model.CreateAuthSessionParams{
		UserID:    userID,
		ExpiresAt: expiresAt,
	})
	if err != nil {
		return nil, fmt.Errorf("error creating auth session: %w", err)
	}
	row, err := s.GetAuthSession(context.TODO(), authSession.ID)
	if err != nil {
		return nil, fmt.Errorf("error getting auth session: %w", err)
	}

	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    authSession.ID.String(),
		Path:     "/",
		Secure:   true,
		HttpOnly: true,
		SameSite: http.SameSiteNoneMode,
		Expires:  authSessionDuration, /* XXX: make configurable */
	})

	return convertRowToSession(row), nil
}

func convertRowToSession(row model.GetAuthSessionRow) *Session {
	ghLinked := false
	ghEmail := ""
	if row.GhEmail.Valid {
		ghLinked = true
		ghEmail = row.GhEmail.String
	}
	return &Session{
		id:              row.ID,
		userID:          &row.UserID,
		email:           &row.Email,
		username:        &row.Username,
		githubLinked:    ghLinked,
		githubEmail:     &ghEmail,
		expiresAt:       row.ExpiresAt,
		isAuthenticated: true,
	}
}

func (sesh *Session) End(s *model.Store, logger *log.Logger) error {
	logger.Println("ending auth session...")

	return s.EndAuthSession(context.TODO(), sesh.id)
}

func GetSession(
	s *model.Store, w http.ResponseWriter, sessionID string,
	logger *log.Logger,
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
		return convertRowToSession(auth), nil
	}
	if err != sql.ErrNoRows {
		return nil, fmt.Errorf("error checking for auth session: %w", err)
	}

	/* try get unauth session */
	unauth, err := s.GetUnauthSession(context.TODO(), uuid)
	if err == nil {
		logger.Printf("Found unauth session\n")
		return &Session{
			id:        unauth.ID,
			expiresAt: unauth.ExpiresAt,
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

func (s *Session) IsGithubLinked() bool {
	return s.githubLinked
}

/* auth session */
func (s *Session) GetEmail() string {
	if s.IsAuthenticated() {
		return *s.email
	}
	return ""
}

func (s *Session) GetUserID() int32 {
	if s.IsAuthenticated() {
		return *s.userID
	}
	return -1
}

func (s *Session) GetUsername() string {
	if s.IsAuthenticated() {
		return *s.username
	}
	return ""
}

func (s *Session) GetGithubEmail() string {
	if s.IsAuthenticated() {
		return *s.githubEmail
	}
	return ""
}
