package session

import (
	"context"
	"database/sql"
	"errors"
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
	id           uuid.UUID
	userID       int32
	email        string
	username     string
	githubLinked bool
	githubEmail  string
	expiresAt    time.Time
	isUnauth     bool
}

func createUnauthSession(
	s *model.Store, w http.ResponseWriter,
	logger *log.Logger,
) (*Session, error) {
	logger.Println("Creating unauth session...")

	unauthSession, err := s.CreateUnauthSession(
		context.TODO(), unauthSessionDuration,
	)
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
		id:        unauthSession.ID,
		expiresAt: unauthSession.ExpiresAt,
		isUnauth:  true,
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
	return &Session{
		id:           row.ID,
		userID:       row.UserID,
		email:        row.Email,
		username:     row.Username,
		githubLinked: row.GhEmail.Valid,
		githubEmail:  row.GhEmail.String,
		expiresAt:    row.ExpiresAt,
		isUnauth:     false,
	}
}

func (sesh *Session) End(s *model.Store, logger *log.Logger) error {
	logger.Println("ending auth session...")

	return s.EndAuthSession(context.TODO(), sesh.id)
}

func getSession(
	s *model.Store, id string, logger *log.Logger,
) (*Session, error) {
	/* parse sessionID from cookie */
	uuid, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("error parsing sessionID: %w", err)
	}

	/* try get auth session */
	auth, err := s.GetAuthSession(context.TODO(), uuid)
	if err == nil {
		logger.Printf("found auth session\n")
		return convertRowToSession(auth), nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("error checking for auth session: %w", err)
	}

	/* try get unauth session */
	unauth, err := s.GetUnauthSession(context.TODO(), uuid)
	if err == nil {
		logger.Printf("found unauth session\n")
		return &Session{
			id:        unauth.ID,
			expiresAt: unauth.ExpiresAt,
		}, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("error checking for unauth session: %w", err)
	}
	return nil, fmt.Errorf("no unauth session: %w", err)
}

/* both */
func (s *Session) GetSessionID() string {
	return s.id.String()
}

func (s *Session) IsUnauth() bool {
	return s.isUnauth
}

func (s *Session) GetExpiresAt() time.Time {
	return s.expiresAt
}

func (s *Session) IsGithubLinked() bool {
	return s.githubLinked
}

/* auth session */
func (s *Session) GetEmail() (string, error) {
	if s.IsUnauth() {
		return "", fmt.Errorf("unauth")
	}
	return s.email, nil
}

func (s *Session) GetUserID() (int32, error) {
	if s.IsUnauth() {
		return -1, fmt.Errorf("unauth")
	}
	return s.userID, nil
}

func (s *Session) GetUsername() (string, error) {
	if s.IsUnauth() {
		return "", fmt.Errorf("unauth")
	}
	return s.username, nil
}

func (s *Session) GetGithubEmail() (string, error) {
	if s.IsUnauth() {
		return "", fmt.Errorf("unauth")
	}
	return s.githubEmail, nil
}
