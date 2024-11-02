package auth

import (
	"net/http"

	"github.com/xr0-org/progstack/internal/logging"
	"github.com/xr0-org/progstack/internal/session"
)

func AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger := logging.Logger(r)

		logger.Println("auth middleware...")
		/* get session from context */
		session, ok := r.Context().Value(session.CtxSessionKey).(*session.Session)
		if !ok || session == nil {
			logger.Println("no session")
			http.Error(w, "", http.StatusNotFound)
			return
		}
		/* check if the session is authenticated (i.e., UserID is not * nil) */
		if session.GetUserID() == -1 {
			logger.Printf("not authorized")
			http.Error(w, "", http.StatusNotFound)
			return
		}

		next.ServeHTTP(w, r)
	})
}
