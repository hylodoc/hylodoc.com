package blog

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
	"github.com/resend/resend-go/v2"
	"github.com/xr0-org/progstack/internal/auth"
	"github.com/xr0-org/progstack/internal/model"
)

type BlogService struct {
	store        *model.Store
	resendClient *resend.Client
}

func NewBlogService(store *model.Store, resendClient *resend.Client) *BlogService {
	return &BlogService{
		store:        store,
		resendClient: resendClient,
	}
}

func (b *BlogService) SubscribeToBlog() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Println("subscribe to blog handler...")

		if err := b.subscribeToBlog(w, r); err != nil {
			log.Printf("error subscribing to blog: %v", err)
			http.Error(w, "", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}
}

type SubscribeRequest struct {
	Email string `json:"email"`
}

func (sr *SubscribeRequest) validate() error {
	if sr.Email == "" {
		return fmt.Errorf("email is required")
	}
	return nil
}

func (b *BlogService) subscribeToBlog(w http.ResponseWriter, r *http.Request) error {
	/* extract BlogID from path */
	vars := mux.Vars(r)
	blogID := vars["blogID"]

	/* parse the request body to get subscriber email */
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return fmt.Errorf("error reading request body: %w", err)
	}
	defer r.Body.Close()

	var req SubscribeRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return fmt.Errorf("error invalid request body: %w", err)
	}
	if err = req.validate(); err != nil {
		return fmt.Errorf("error invalid request body: %w", err)
	}

	/* XXX: validate email format */

	intBlogID, err := strconv.ParseInt(blogID, 10, 32)
	if err != nil {
		return fmt.Errorf("error converting string path var to blogID: %w", err)
	}

	unsubscribeToken, err := auth.GenerateToken()
	if err != nil {
		return fmt.Errorf("error generating unsubscribeToken: %w", err)
	}

	log.Printf("subscribing email `%s' to blog with id: `%d'", req.Email, intBlogID)
	/* first check if exists */

	err = b.store.CreateSubscriberTx(context.TODO(), model.CreateSubscriberTxParams{
		BlogID:           int32(intBlogID),
		Email:            req.Email,
		UnsubscribeToken: unsubscribeToken,
	})
	if err != nil {
		return fmt.Errorf("error writing subscriber for blog to db: %w", err)
	}
	return nil
}

func (b *BlogService) UnsubscribeFromBlog() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Println("unsubscribe from blog handler...")
		if err := b.unsubscribeFromBlog(w, r); err != nil {
			log.Printf("error in unsubscribeFromBlog handler: %w", err)
			http.Error(w, "", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}
}

type UnsubscribeRequest struct {
	Token string `json:"token"`
}

func (ur *UnsubscribeRequest) validate() error {
	if ur.Token == "" {
		return fmt.Errorf("token is required")
	}
	return nil
}

func (b *BlogService) unsubscribeFromBlog(w http.ResponseWriter, r *http.Request) error {
	/* extract BlogID from path */
	vars := mux.Vars(r)
	blogID := vars["blogID"]

	/* parse the request body to get unsubscribe token */
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return fmt.Errorf("error reading request body: %w", err)
	}
	defer r.Body.Close()

	var req UnsubscribeRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return fmt.Errorf("error invalid request body: %w", err)
	}
	if err = req.validate(); err != nil {
		return fmt.Errorf("error invalid request body: %w", err)
	}

	intBlogID, err := strconv.ParseInt(blogID, 10, 32)
	if err != nil {
		return fmt.Errorf("error converting string path var to blogID: %w", err)
	}
	err = b.store.DeleteSubscriberForBlog(context.TODO(), model.DeleteSubscriberForBlogParams{
		BlogID:           int32(intBlogID),
		UnsubscribeToken: req.Token,
	})
	if err != nil {
		return fmt.Errorf("error writing subscriber for blog to db: %w", err)
	}
	return nil
}
