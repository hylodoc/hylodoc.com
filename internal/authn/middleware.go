package authn

import (
	"net/http"

	"github.com/xr0-org/progstack/internal/app/handler"
	"github.com/xr0-org/progstack/internal/assert"
	"github.com/xr0-org/progstack/internal/session"
	"github.com/xr0-org/progstack/internal/util"
)

func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := middleware(r); err != nil {
			handler.HandleError(w, r, err)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func middleware(r *http.Request) error {
	sesh, ok := r.Context().Value(session.CtxSessionKey).(*session.Session)
	assert.Assert(ok)
	if sesh.GetUserID() == -1 {
		sesh.Printf("not authorized\n")
		return util.CreateCustomError("", http.StatusNotFound)
	}
	return nil
}
