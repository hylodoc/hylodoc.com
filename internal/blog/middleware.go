package blog

import (
	"context"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
	"github.com/xr0-org/progstack/internal/assert"
	"github.com/xr0-org/progstack/internal/model"
	"github.com/xr0-org/progstack/internal/session"
)

func (b *BlogService) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		/* authorise blog */
		sesh, ok := r.Context().Value(session.CtxSessionKey).(*session.Session)
		assert.Assert(ok)

		sesh.Println("Running blog authorisation middleware...")

		/* blogID and userID */
		blogID := mux.Vars(r)["blogID"]
		userID := sesh.GetUserID()

		intBlogID, err := strconv.ParseInt(blogID, 10, 32)
		if err != nil {
			sesh.Printf("Error converting string path var to blogID: %v\n", err)
			http.Error(w, "", http.StatusInternalServerError)
			return
		}
		userOwnsBlog, err := b.store.CheckBlogOwnership(
			context.TODO(), model.CheckBlogOwnershipParams{
				ID:     int32(intBlogID),
				UserID: userID,
			})
		if err != nil {
			sesh.Printf("Error checking blog ownership: %v\n", err)
			http.Error(w, "", http.StatusInternalServerError)
			return
		}
		if !userOwnsBlog {
			sesh.Printf("User `%d' does not own blog `%d'\n", userID, intBlogID)
			http.Error(w, "", http.StatusNotFound)
			return
		}

		if _, err := GetFreshGeneration(
			int32(intBlogID), b.store,
		); err != nil {
			sesh.Printf("Error getting fresh generation: %v\n", err)
			http.Error(w, "", http.StatusInternalServerError)
			return
		}
		next.ServeHTTP(w, r)
	})
}
