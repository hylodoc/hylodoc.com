package blog

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"
	"github.com/xr0-org/progstack/internal/auth"
	"github.com/xr0-org/progstack/internal/model"
	"github.com/xr0-org/progstack/internal/util"
)

type SubscriberData struct {
	Count            int
	CumulativeCounts template.JS
}

type CumulativeCount struct {
	Timestamp string `json:"timestamp"` /* Date as a string in "YYYY-MM-DD" format */
	Count     int    `json:"count"`     /* Cumulative subscriber count on that date */
}

func (b *BlogService) SubscriberMetrics() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Println("subscriber metrics handler...")

		session, _ := r.Context().Value(auth.CtxSessionKey).(*auth.Session)
		if session == nil {
			http.Error(w, "", http.StatusNotFound)
			return
		}
		log.Printf("session: %v\n", session)

		data, err := b.subscriberMetrics(w, r)
		if err != nil {
			log.Printf("error getting subscriber metrics: %v", err)
			http.Error(w, "", http.StatusInternalServerError)
			return
		}
		log.Printf("subscribers: %s\n", data)

		util.ExecTemplate(w, []string{"dashboard.html"},
			util.PageInfo{
				Data: struct {
					Title          string
					Session        *auth.Session
					SubscriberData SubscriberData
				}{
					Title:          "Dashboard",
					Session:        session,
					SubscriberData: data,
				},
			},
		)
	}
}

func (b *BlogService) subscriberMetrics(w http.ResponseWriter, r *http.Request) (SubscriberData, error) {
	blogID := mux.Vars(r)["blogID"]
	intBlogID, err := strconv.ParseInt(blogID, 10, 32)
	if err != nil {
		return SubscriberData{}, fmt.Errorf("error converting string path var to blogID: %w", err)
	}
	subs, err := b.store.ListSubscribersByBlogID(context.TODO(), int32(intBlogID))
	if err != nil {
		if err != sql.ErrNoRows {
			return SubscriberData{}, fmt.Errorf("error listing subscriber counts: %w", err)
		}
	}
	subscriberData := buildSubscriberCumulativeCounts(subs)
	jsonSubscriberData, err := json.Marshal(subscriberData)
	if err != nil {
		return SubscriberData{}, fmt.Errorf("error marshaling subscriber data: %w", err)
	}
	return SubscriberData{
		Count:            len(subs),
		CumulativeCounts: template.JS(string(jsonSubscriberData)),
	}, nil
}

func buildSubscriberCumulativeCounts(subs []model.Subscriber) []CumulativeCount {
	/* hold cumulative counts per hour */
	cumulativeCounts := make(map[time.Time]int)

	for _, sub := range subs {
		hour := sub.CreatedAt.Truncate(time.Hour)
		cumulativeCounts[hour]++
	}
	var result []CumulativeCount
	cumulativeSum := 0
	for hour := range cumulativeCounts {
		cumulativeSum += cumulativeCounts[hour]
		result = append(result, CumulativeCount{
			Timestamp: hour.Format(time.RFC1123Z), /* ISO format for browser compatibility */
			Count:     cumulativeSum,
		})
	}
	return result
}
