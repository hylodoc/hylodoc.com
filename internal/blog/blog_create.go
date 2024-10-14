package blog

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"

	"github.com/xr0-org/progstack/internal/auth"
	"github.com/xr0-org/progstack/internal/config"
	"github.com/xr0-org/progstack/internal/model"
)

func (b *BlogService) CreateBlog() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("create blog handler...")

		if err := b.createBlog(w, r); err != nil {
			log.Printf("error creating blog: %v", err)
			http.Error(w, "", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
	}
}

type CreateBlogRequest struct {
	Subdomain    string `json:"subdomain"`
	RepositoryID string `json:"repository_id"`
	TestBranch   string `json:"test_branch"`
	LiveBranch   string `json:"live_branch"`
}

func (b *BlogService) createBlog(w http.ResponseWriter, r *http.Request) error {
	session, ok := r.Context().Value(auth.CtxSessionKey).(*auth.Session)
	if !ok {
		return fmt.Errorf("user not found")
	}

	var req CreateBlogRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return fmt.Errorf("could not decode body: %w", err)
	}
	fmt.Printf("req: %v", req)

	intRepoID, err := strconv.ParseInt(req.RepositoryID, 10, 64)
	if err != nil {
		return fmt.Errorf("could not convert repositoryID `%s' to int64: %w", req.RepositoryID, err)
	}

	repo, err := b.store.GetRepositoryByGhRepositoryID(context.TODO(), intRepoID)
	if err != nil {
		return fmt.Errorf("could not get repository for ghRepoId `%s': %w", intRepoID, err)
	}

	blog, err := b.store.CreateBlog(context.TODO(), model.CreateBlogParams{
		UserID:         session.UserID,
		GhRepositoryID: intRepoID,
		GhFullName:     repo.FullName,
		Subdomain:      req.Subdomain,
		FromAddress:    config.Config.Progstack.FromEmail,
		BlogType:       model.BlogTypeRepository,
	})
	if err != nil {
		return fmt.Errorf("could not create blog: %w", err)
	}

	/* add first user as subscriber */
	if err = b.store.CreateSubscriberTx(context.TODO(), model.CreateSubscriberTxParams{
		BlogID: blog.ID,
		Email:  session.Email,
	}); err != nil {
		return fmt.Errorf("error creating first subscriber: %w", err)
	}
	fmt.Printf("blog: %v", blog)
	return nil
}
