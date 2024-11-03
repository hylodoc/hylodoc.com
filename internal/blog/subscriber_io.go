package blog

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"html/template"
	"io/ioutil"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/xr0-org/progstack/internal/config"
	"github.com/xr0-org/progstack/internal/logging"
	"github.com/xr0-org/progstack/internal/model"
	"github.com/xr0-org/progstack/internal/util"
)

func (b *BlogService) SubscribeToBlog() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger := logging.Logger(r)
		logger.Println("SubscribeToBlog handler...")

		b.mixpanel.Track("SubscribeToBlog", r)

		if err := b.subscribeToBlog(w, r); err != nil {
			logger.Printf("Error subscribing to blog: %v", err)
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
	/* XXX: better validation */
	if sr.Email == "" {
		return fmt.Errorf("email is required")
	}
	return nil
}

func (b *BlogService) subscribeToBlog(w http.ResponseWriter, r *http.Request) error {
	logger := logging.Logger(r)

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
		return fmt.Errorf("error unmarshaling request: %w", err)
	}
	if err = req.validate(); err != nil {
		return fmt.Errorf("error invalid request body: %w", err)
	}

	/* XXX: validate email format */

	intBlogID, err := strconv.ParseInt(blogID, 10, 32)
	if err != nil {
		return fmt.Errorf("error converting string path var to blogID: %w", err)
	}

	logger.Printf("subscribing email `%s' to blog with id: `%d'", req.Email, intBlogID)
	/* first check if exists */

	err = b.store.CreateSubscriberTx(context.TODO(), model.CreateSubscriberTxParams{
		BlogID: int32(intBlogID),
		Email:  req.Email,
	})
	if err != nil {
		return fmt.Errorf("error writing subscriber for blog to db: %w", err)
	}
	return nil
}

func (b *BlogService) UnsubscribeFromBlog() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger := logging.Logger(r)
		logger.Println("UnsubscribeFromBlog handler...")

		b.mixpanel.Track("UnsubscribeFromBlog", r)

		if err := b.unsubscribeFromBlog(w, r); err != nil {
			logger.Printf("error in unsubscribeFromBlog handler: %v\n", err)
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

	intBlogID, err := strconv.ParseInt(blogID, 10, 32)
	if err != nil {
		return fmt.Errorf("error converting string path var to blogID: %w", err)
	}
	token := r.URL.Query().Get("token")
	uuid, err := uuid.Parse(token)
	if err != nil {
		log.Fatalf("Failed to parse UUID: %v", err)
	}

	err = b.store.DeleteSubscriberForBlog(context.TODO(), model.DeleteSubscriberForBlogParams{
		BlogID:           int32(intBlogID),
		UnsubscribeToken: uuid,
	})
	if err != nil {
		return fmt.Errorf("error writing subscriber for blog to db: %w", err)
	}
	return nil
}

func (b *BlogService) ImportSubscribers() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

	}
}

func (b *BlogService) EditSubscriber() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger := logging.Logger(r)
		logger.Println("edit subscriber handler...")

		email := r.URL.Query().Get("email")

		blogID := mux.Vars(r)["blogID"]
		intBlogID, err := strconv.ParseInt(blogID, 10, 32)
		if err != nil {
			logger.Printf("could not parse blogID: %v", err)
			http.Error(w, "", http.StatusBadRequest)
			return
		}

		logger.Printf("blogID: %s, mail: %s\n", email, blogID)

		util.ExecTemplate(w, []string{"subscriber_edit.html"},
			util.PageInfo{
				Data: struct {
					Email               string
					RemoveSubscriberUrl string
				}{
					Email:               email,
					RemoveSubscriberUrl: buildRemoveSubscriberUrl(int32(intBlogID), email),
				},
			},
			template.FuncMap{},
			logger,
		)
	}
}

func buildRemoveSubscriberUrl(blogID int32, email string) string {
	return fmt.Sprintf(
		"%s://%s/user/blogs/%d/subscriber/delete?email=%s",
		config.Config.Progstack.Protocol,
		config.Config.Progstack.ServiceName,
		blogID,
		email,
	)
}

func (b *BlogService) DeleteSubscriber() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger := logging.Logger(r)
		logger.Println("DeleteSubscriber handler...")

		b.mixpanel.Track("DeleteSubscriber", r)

		url, err := b.deleteSubscriber(w, r)
		if err != nil {
			logger.Printf("Error deleting subscriber: %v\n", err)
			http.Error(w, "", http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, url, http.StatusSeeOther)
	}
}

func (b *BlogService) deleteSubscriber(w http.ResponseWriter, r *http.Request) (string, error) {
	logger := logging.Logger(r)

	email := r.URL.Query().Get("email")

	blogID := mux.Vars(r)["blogID"]
	intBlogID, err := strconv.ParseInt(blogID, 10, 32)
	if err != nil {
		return "", fmt.Errorf("error converting string path var to blogID: %w", err)
	}

	logger.Printf("deleting subscriber `%s' for blogID `%s'\n", email, blogID)
	if err := b.store.DeleteSubscriberByEmail(context.TODO(), model.DeleteSubscriberByEmailParams{
		BlogID: int32(intBlogID),
		Email:  email,
	}); err != nil {
		return "", fmt.Errorf("error deleting subscriber `%s' for blog `%s': %w", email, blogID, err)
	}
	return buildSubscriberMetricsUrl(int32(intBlogID)), nil
}

func (b *BlogService) ExportSubscribers() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger := logging.Logger(r)
		logger.Println("ExportSubscribers handler...")

		b.mixpanel.Track("ExportSubscribers", r)

		csvData, err := b.exportSubscribers(w, r)
		if err != nil {
			logger.Printf("Error exporting subsribers: %v", err)
			http.Error(w, "", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/csv")
		w.Header().Set("Content-Disposition", "attachment; filename=subscribers.csv")
		w.Write(csvData)
	}
}

func (b *BlogService) exportSubscribers(w http.ResponseWriter, r *http.Request) ([]byte, error) {
	blogID := mux.Vars(r)["blogID"]
	intBlogID, err := strconv.ParseInt(blogID, 10, 32)
	if err != nil {
		return nil, fmt.Errorf("error converting string path var to blogID: %w", err)
	}

	subs, err := b.store.ListActiveSubscribersByBlogID(context.TODO(), int32(intBlogID))
	if err != nil {
		return nil, fmt.Errorf("error listing subscribers: %w", err)
	}

	csv, err := buildSubscriberCSV(subs)
	if err != nil {
		return nil, fmt.Errorf("error building csv: %w", err)
	}

	return csv, nil
}

func buildSubscriberCSV(subs []model.Subscriber) ([]byte, error) {
	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)

	fields := []string{"Email", "CreatedAt"}
	if err := writer.Write(fields); err != nil {
		return nil, fmt.Errorf("error writing header: %w", err)
	}

	for _, sub := range subs {
		row := []string{sub.Email, sub.CreatedAt.Format(time.RFC3339)}
		if err := writer.Write(row); err != nil {
			return nil, fmt.Errorf("error writing row: %w", err)
		}
	}

	writer.Flush()
	return buf.Bytes(), nil
}
