package blog

import (
	"context"
	"log"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
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
		log.Println("running blog middleware...")
		/* authorise blog */
		sesh, ok := r.Context().Value(session.CtxSessionKey).(*session.Session)
		if !ok {
			log.Println("error getting session from context")
			http.Error(w, "User not found", http.StatusUnauthorized)
			return
		}
		/* blogID and userID */
		blogID := mux.Vars(r)["blogID"]
		userID := sesh.GetUserID()

		intBlogID, err := strconv.ParseInt(blogID, 10, 32)
		if err != nil {
			log.Printf("error converting string path var to blogID: %v\n", err)
			http.Error(w, "", http.StatusInternalServerError)
			return
		}
		userOwnsBlog, err := bm.store.CheckBlogOwnership(context.TODO(), model.CheckBlogOwnershipParams{
			ID:     int32(intBlogID),
			UserID: userID,
		})
		if err != nil {
			log.Printf("error checking blog ownership: %v\n", err)
			http.Error(w, "", http.StatusInternalServerError)
			return
		}
		if !userOwnsBlog {
			log.Printf("user `%d' does not own blog `%d'\n", userID, intBlogID)
			http.Error(w, "", http.StatusNotFound)
			return
		}
		next.ServeHTTP(w, r)
	})
}
