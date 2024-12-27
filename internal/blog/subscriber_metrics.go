package blog

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"time"

	"github.com/xr0-org/progstack/internal/app/handler/request"
	"github.com/xr0-org/progstack/internal/app/handler/response"
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

func (b *BlogService) SubscriberMetrics(
	r request.Request,
) (response.Response, error) {
	logger := r.Logger()
	logger.Println("SubscriberMetrics handler...")

	r.MixpanelTrack("SubscriberMetrics")

	sesh := r.Session()
	blogID, ok := r.GetRouteVar("blogID")
	if !ok {
		return nil, util.CreateCustomError("", http.StatusNotFound)
	}
	intBlogID, err := strconv.ParseInt(blogID, 10, 32)
	if err != nil {
		return nil, fmt.Errorf("parse int: %w", err)
	}
	data, err := b.subscriberMetrics(int32(intBlogID))
	if err != nil {
		return nil, fmt.Errorf("subscriber metrics: %w", err)
	}
	return response.NewTemplate(
		[]string{"subscriber_metrics.html", "subscribers.html"},
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
	), nil
}

func (b *BlogService) subscriberMetrics(intBlogID int32) (SubscriberData, error) {
	subs, err := b.store.ListActiveSubscribersByBlogID(
		context.TODO(), intBlogID,
	)
	if err != nil {
		return SubscriberData{}, fmt.Errorf(
			"list active subscriber: %w", err,
		)
	}

	jsonSubscriberData, err := json.Marshal(
		buildSubscriberCumulativeCounts(subs),
	)
	if err != nil {
		return SubscriberData{}, fmt.Errorf(
			"json marshall: %w", err,
		)
	}
	return SubscriberData{
		Count:            len(subs),
		CumulativeCounts: template.JS(jsonSubscriberData),
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
