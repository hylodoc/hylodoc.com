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

type BlogMiddleware struct {
	store *model.Store
}

func NewBlogMiddleware(s *model.Store) *BlogMiddleware {
	return &BlogMiddleware{
		store: s,
	}
}

func (bm *BlogMiddleware) AuthoriseBlog(next http.Handler) http.Handler {
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
		userOwnsBlog, err := bm.store.CheckBlogOwnership(context.TODO(), model.CheckBlogOwnershipParams{
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
		next.ServeHTTP(w, r)
	})
}
