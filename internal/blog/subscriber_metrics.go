package blog

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"
	"github.com/xr0-org/progstack/internal/logging"
	"github.com/xr0-org/progstack/internal/model"
	"github.com/xr0-org/progstack/internal/session"
	"github.com/xr0-org/progstack/internal/util"
)

type SubscriberData struct {
	Count            int
	CumulativeCounts template.JS
	Subscribers      []Subscriber
}

type CumulativeCount struct {
	Timestamp string `json:"timestamp"` /* Date as a string in "YYYY-MM-DD" format */
	Count     int    `json:"count"`     /* Cumulative subscriber count on that date */
}

type Subscriber struct {
	Email        string
	SubscribedOn string
	Status       string
}

func (b *BlogService) SubscriberMetrics() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger := logging.Logger(r)
		logger.Println("SubscriberMetrics handler...")

		sesh, ok := r.Context().Value(session.CtxSessionKey).(*session.Session)
		if !ok {
			logger.Println("No auth session")
			http.Error(w, "", http.StatusNotFound)
			return
		}

		data, err := b.subscriberMetrics(w, r)
		if err != nil {
			logger.Printf("Error getting subscriber metrics: %v", err)
			http.Error(w, "", http.StatusInternalServerError)
			return
		}
		logger.Printf("subscribers: %v\n", data)

		util.ExecTemplate(w, []string{"subscriber_metrics.html", "subscribers.html"},
			util.PageInfo{
				Data: struct {
					Title          string
					UserInfo       *session.UserInfo
					SubscriberData SubscriberData
				}{
					Title:          "Dashboard",
					UserInfo:       session.ConvertSessionToUserInfo(sesh),
					SubscriberData: data,
				},
			},
			template.FuncMap{},
			logger,
		)
	}
}

func (b *BlogService) subscriberMetrics(w http.ResponseWriter, r *http.Request) (SubscriberData, error) {
	blogID := mux.Vars(r)["blogID"]
	intBlogID, err := strconv.ParseInt(blogID, 10, 32)
	if err != nil {
		return SubscriberData{}, fmt.Errorf("error converting string path var to blogID: %w", err)
	}
	subs, err := b.store.ListActiveSubscribersByBlogID(context.TODO(), int32(intBlogID))
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
		Subscribers:      convertSubscribers(subs),
	}, nil
}

func convertSubscribers(subs []model.Subscriber) []Subscriber {
	var res []Subscriber
	for _, s := range subs {
		res = append(res, Subscriber{
			Email:        s.Email,
			SubscribedOn: s.CreatedAt.Format("January 2, 2006"),
			Status:       string(s.Status),
		})
	}
	return res
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
