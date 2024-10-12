package blog

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"
	"github.com/xr0-org/progstack/internal/model"
)

func (b *BlogService) ImportSubscribers() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

	}
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

	subs, err := b.store.ListSubscribersByBlogID(context.TODO(), int32(intBlogID))
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
