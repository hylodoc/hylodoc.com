package blog

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
	"github.com/xr0-org/progstack/internal/app/handler"
	"github.com/xr0-org/progstack/internal/assert"
	"github.com/xr0-org/progstack/internal/model"
	"github.com/xr0-org/progstack/internal/session"
	"github.com/xr0-org/progstack/internal/util"
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

	userID := sesh.GetUserID()

	intBlogID, err := strconv.ParseInt(mux.Vars(r)["blogID"], 10, 32)
	if err != nil {
		return fmt.Errorf("parse blogID: %w", err)
	}
	blogID := int32(intBlogID)

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
		sesh.Printf("user `%d' does not own blog `%d'\n", userID, blogID)
		return util.CreateCustomError("", http.StatusNotFound)
	}

	if _, err := GetFreshGeneration(blogID, b.store); err != nil {
		return fmt.Errorf("get fresh generation: %w", err)
	}
	return nil
}
