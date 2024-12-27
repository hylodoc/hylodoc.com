package authn

import (
	"fmt"
	"net/http"

	"github.com/xr0-org/progstack/internal/app/handler"
	"github.com/xr0-org/progstack/internal/assert"
	"github.com/xr0-org/progstack/internal/session"
)

func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := checkuserid(r); err != nil {
			handler.NotFound(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func checkuserid(r *http.Request) error {
	sesh, ok := r.Context().Value(session.CtxSessionKey).(*session.Session)
	assert.Assert(ok)
	/* XXX: enforce user is authenticated */
	if _, err := sesh.GetUserID(); err != nil {
		sesh.Printf("unauth user accessing `/user': %v\n", err)
		return fmt.Errorf("get user id: %w", err)
	}
	return nil
}
