package blog

import (
	"context"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
	"github.com/xr0-org/progstack/internal/logging"
	"github.com/xr0-org/progstack/internal/model"
	"github.com/xr0-org/progstack/internal/session"
)

func (b *BlogService) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger := logging.Logger(r)

		logger.Println("Running blog authorisation middleware...")
		/* authorise blog */
		sesh, ok := r.Context().Value(session.CtxSessionKey).(*session.Session)
		if !ok {
			logger.Println("No auth session")
			http.Error(w, "", http.StatusNotFound)
			return
		}
		/* blogID and userID */
		blogID := mux.Vars(r)["blogID"]
		userID := sesh.GetUserID()

		intBlogID, err := strconv.ParseInt(blogID, 10, 32)
		if err != nil {
			logger.Printf("Error converting string path var to blogID: %v\n", err)
			http.Error(w, "", http.StatusInternalServerError)
			return
		}
		userOwnsBlog, err := b.store.CheckBlogOwnership(
			context.TODO(), model.CheckBlogOwnershipParams{
				ID:     int32(intBlogID),
				UserID: userID,
			})
		if err != nil {
			logger.Printf("Error checking blog ownership: %v\n", err)
			http.Error(w, "", http.StatusInternalServerError)
			return
		}
		if !userOwnsBlog {
			logger.Printf("User `%d' does not own blog `%d'\n", userID, intBlogID)
			http.Error(w, "", http.StatusNotFound)
			return
		}

		if _, err := GetFreshGeneration(
			int32(intBlogID), b.store,
		); err != nil {
			logger.Printf("Error getting fresh generation: %v\n", err)
			http.Error(w, "", http.StatusInternalServerError)
			return
		}
		next.ServeHTTP(w, r)
	})
}
