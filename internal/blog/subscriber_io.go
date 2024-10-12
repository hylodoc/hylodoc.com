package blog

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"
	"github.com/xr0-org/progstack/internal/config"
	"github.com/xr0-org/progstack/internal/model"
	"github.com/xr0-org/progstack/internal/util"
)

func (b *BlogService) ImportSubscribers() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

	}
}

func (b *BlogService) EditSubscriber() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Println("edit subscriber handler...")

		email := r.URL.Query().Get("email")

		blogID := mux.Vars(r)["blogID"]
		intBlogID, err := strconv.ParseInt(blogID, 10, 32)
		if err != nil {
			log.Printf("could not parse blogID: %v", err)
			http.Error(w, "", http.StatusBadRequest)
			return
		}

		log.Printf("blogID: %s, mail: %s\n", email, blogID)

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
		log.Println("delete subscriber handler...")
		url, err := b.deleteSubscriber(w, r)
		if err != nil {
			log.Printf("error deleting subscriber: %v\n", err)
			http.Error(w, "", http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, url, http.StatusSeeOther)
	}
}

func (b *BlogService) deleteSubscriber(w http.ResponseWriter, r *http.Request) (string, error) {
	email := r.URL.Query().Get("email")

	blogID := mux.Vars(r)["blogID"]
	intBlogID, err := strconv.ParseInt(blogID, 10, 32)
	if err != nil {
		return "", fmt.Errorf("error converting string path var to blogID: %w", err)
	}

	log.Printf("deleting subscriber `%s' for blogID `%s'\n", email, blogID)
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
		csvData, err := b.exportSubscribers(w, r)
		if err != nil {
			log.Printf("error exporting subsribers: %v", err)
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
