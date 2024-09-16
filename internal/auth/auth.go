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

const authCookieName = "auth_session_id"

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
		user, err := validateAuthSession(w, r, a.store)
		if err != nil {
			http.Redirect(w, r, "/", http.StatusFound)
			return
		}
		ctx := context.WithValue(r.Context(), "user", user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func validateAuthSession(w http.ResponseWriter, r *http.Request, s *model.Store) (*User, error) {
	cookie, err := r.Cookie(authCookieName)
	authSessionId := cookie.Value
	if err != nil || authSessionId == "" {
		return nil, fmt.Errorf("error reading auth cookie")
	}
	u, err := validateAuthSessionId(authSessionId, w, s)
	if err != nil {
		log.Println("error validating authSessionId: ", err)
		return nil, fmt.Errorf("error validating auth session id: %w", err)
	}
	return u, nil
}

/* json tags for unmarshalling of Github userinfo during OAuth */
type User struct {
	Email    string `json:"email"`
	Username string `json:"login"`
}

func validateAuthSessionId(sessionId string, w http.ResponseWriter, s *model.Store) (*User, error) {
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
	return &User{
		Username: session.Username,
		Email:    session.Email,
	}, nil
}

/* HandleOAuth */

func HandleOAuth(user *User, w http.ResponseWriter, s *model.Store) error {
	/* create user or login user */
	/* XXX: these db accesses to end of func should all be atomic in a Tx */
	u, err := s.GetUserByEmail(context.TODO(), user.Email)
	if err != nil {
		if err != sql.ErrNoRows {
			return fmt.Errorf("error checking for user existence: %w", err)
		}
		log.Println("creating user in db...")
		req := model.CreateUserParams{
			Email: user.Email, Username: user.Username,
		}
		u, err = s.CreateUser(context.TODO(), req)
		if err != nil {
			log.Println("error creating user in db: ", err)
			return fmt.Errorf("error creating user in db: %w", err)
		}
		log.Println("created user: ", u)
	}
	log.Println("got user: ", u)

	err = createAuthSession(u.ID, w, s)
	if err != nil {
		log.Println("error creating auth session: ", err)
		return fmt.Errorf("error creating auth session: ", err)
	}
	return nil
}

func createAuthSession(userId int32, w http.ResponseWriter, s *model.Store) error {
	sessionId, err := generateToken()
	if err != nil {
		return fmt.Errorf("error generating sessionId: %w", err)
	}
	/* check generated sessionId doesn't exist in db */
	_, err = s.GetSession(context.TODO(), sessionId)
	if err == nil {
		return fmt.Errorf("error sessionId already exists")
	} else {
		if err != sql.ErrNoRows {
			return err
		}
	}
	/* create unauthSession in db */
	_, err = s.CreateSession(context.TODO(), model.CreateSessionParams{
		Token: sessionId, UserID: userId,
	})
	if err != nil {
		return fmt.Errorf("error writing unauth cookie to db: %w", err)
	}
	/* set cookie */
	http.SetCookie(w, &http.Cookie{
		Name:     authCookieName,
		Value:    sessionId,
		Path:     "/",
		Secure:   true,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(168 * time.Hour), /* XXX: make configurable */
	})
	return nil
}

/* Logout */

func Logout(user *User, w http.ResponseWriter, r *http.Request, s *model.Store) error {
	cookie, err := r.Cookie(authCookieName)
	authSessionId := cookie.Value
	if err != nil || authSessionId == "" {
		return fmt.Errorf("error reading auth cookie")
	}
	/* end session */
	err = s.EndSession(context.TODO(), cookie.Value)
	if err != nil {
		return err
	}
	/* expire the cookie */
	http.SetCookie(w, &http.Cookie{
		Name:    authCookieName,
		Value:   "",
		Path:    "/",
		Expires: time.Unix(0, 0),
		MaxAge:  -1,
	})
	return nil
}
