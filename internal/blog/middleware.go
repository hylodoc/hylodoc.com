package blog

import (
	"context"
	"fmt"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/hylodoc/hylodoc.com/internal/app/handler"
	"github.com/hylodoc/hylodoc.com/internal/assert"
	"github.com/hylodoc/hylodoc.com/internal/model"
	"github.com/hylodoc/hylodoc.com/internal/session"
)

func (b *BlogService) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := b.middleware(w, r); err != nil {
			handler.HandleError(w, r, err)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (b *BlogService) middleware(w http.ResponseWriter, r *http.Request) error {
	sesh, ok := r.Context().Value(session.CtxSessionKey).(*session.Session)
	assert.Assert(ok)

	sesh.Println("Running blog authorisation middleware...")

	userID, err := sesh.GetUserID()
	if err != nil {
		return fmt.Errorf("get user id: %w", err)
	}

	blogID, ok := mux.Vars(r)["blogID"]
	if !ok {
		return createCustomError("", http.StatusNotFound)
	}

	userOwnsBlog, err := b.store.CheckBlogOwnership(
		context.TODO(), model.CheckBlogOwnershipParams{
			ID:     blogID,
			UserID: userID,
		},
	)
	if err != nil {
		return fmt.Errorf("check user owns blog: %w", err)
	}
	if !userOwnsBlog {
		sesh.Printf("user `%s' does not own blog `%s'\n", userID, blogID)
		return createCustomError("", http.StatusNotFound)
	}

	if _, err := GetFreshGeneration(blogID, b.store); err != nil {
		return fmt.Errorf("get fresh generation: %w", err)
	}
	return nil
}
