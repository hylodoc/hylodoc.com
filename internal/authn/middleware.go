package authn

import (
	"net/http"

	"github.com/xr0-org/progstack/internal/assert"
	"github.com/xr0-org/progstack/internal/session"
)

func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		/* get session from context */
		sesh, ok := r.Context().Value(session.CtxSessionKey).(*session.Session)
		assert.Assert(ok)
		/* check if the session is authenticated (i.e., UserID is not * nil) */
		if sesh.GetUserID() == -1 {
			sesh.Printf("not authorized")
			http.Error(w, "", http.StatusNotFound)
			return
		}

		next.ServeHTTP(w, r)
	})
}
