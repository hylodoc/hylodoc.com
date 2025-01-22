package blog

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/hylodoc/hylodoc.com/internal/app/handler/request"
	"github.com/hylodoc/hylodoc.com/internal/app/handler/response"
	"github.com/hylodoc/hylodoc.com/internal/config"
	"github.com/hylodoc/hylodoc.com/internal/email"
	"github.com/hylodoc/hylodoc.com/internal/email/emailaddr"
	"github.com/hylodoc/hylodoc.com/internal/model"
	"github.com/hylodoc/hylodoc.com/internal/session"
	"github.com/hylodoc/hylodoc.com/internal/util"
)

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

func (b *BlogService) SubscribeToBlog(
	r request.Request,
) (response.Response, error) {
	sesh := r.Session()
	sesh.Println("SubscribeToBlog handler...")

	r.MixpanelTrack("SubscribeToBlog")

	/* TODO: validate email format */
	e, err := r.GetPostFormValue("email")
	if err != nil {
		return nil, fmt.Errorf("get email: %w", err)
	}

	blogID, ok := r.GetRouteVar("blogID")
	if !ok {
		return nil, createCustomError("", http.StatusNotFound)
	}

	blog, err := b.store.GetBlogByID(context.TODO(), blogID)
	if err != nil {
		return nil, fmt.Errorf("get blog: %w", err)
	}

	unsubtoken, err := b.createsubscriber(e, blog.ID, sesh)
	if err != nil {
		return nil, fmt.Errorf("create subscriber: %w", err)
	}
	sitename := getsitename(&blog)
	if err := email.NewSender(
		emailaddr.NewAddr(e),
		emailaddr.NewNamedAddr(
			sitename,
			fmt.Sprintf(
				"%s@%s",
				blog.Subdomain,
				config.Config.Hylodoc.EmailDomain,
			),
		),
		blog.EmailMode,
		b.store,
	).SendNewSubscriberEmail(
		sitename,
		fmt.Sprintf(
			"%s://%s/blogs/unsubscribe?token=%s",
			config.Config.Hylodoc.Protocol,
			config.Config.Hylodoc.RootDomain,
			unsubtoken,
		),
	); err != nil {
		return nil, fmt.Errorf("send new subscriber email: %w", err)
	}

	return response.NewRedirect(
		fmt.Sprintf(
			"%s://%s.%s/subscribed",
			config.Config.Hylodoc.Protocol,
			blog.Subdomain,
			config.Config.Hylodoc.RootDomain,
		),
		http.StatusTemporaryRedirect,
	), nil
}

func getsitename(blog *model.Blog) string {
	if blog.Name.Valid {
		return blog.Name.String
	}
	return blog.Subdomain.String()
}

func (b *BlogService) createsubscriber(
	email string, blog string, sesh *session.Session,
) (string, error) {
	sesh.Printf("subscribing email `%s' to blog %s\n", email, blog)
	tk, err := b.createorgetsubscriber(email, blog, sesh)
	if err != nil {
		return "", fmt.Errorf("cannot create or get subscriber: %w", err)
	}
	return tk.String(), nil
}

func (b *BlogService) createorgetsubscriber(
	email string, blog string, sesh *session.Session,
) (*uuid.UUID, error) {
	tk, err := b.store.CreateSubscriber(
		context.TODO(),
		model.CreateSubscriberParams{
			BlogID: blog,
			Email:  email,
		},
	)
	if err != nil {
		if isUniqueActiveSubscriberPerBlogViolation(err) {
			sesh.Println("duplicate subscription")
			sub, err := b.store.GetSubscriberForBlog(
				context.TODO(),
				model.GetSubscriberForBlogParams{
					BlogID: blog,
					Email:  email,
				},
			)
			if err != nil {
				return nil, fmt.Errorf(
					"error getting subscriber: %w", err,
				)
			}
			return &sub.UnsubscribeToken, nil
		}
		return nil, fmt.Errorf("error creating: %w", err)
	}
	return &tk, nil
}

func isUniqueActiveSubscriberPerBlogViolation(err error) bool {
	var pqerr *pq.Error
	return errors.As(err, &pqerr) &&
		pqerr.Code.Name() == "unique_violation" &&
		pqerr.Constraint == "unique_active_subscriber_per_blog"
}

func (b *BlogService) UnsubscribeFromBlog(
	r request.Request,
) (response.Response, error) {
	sesh := r.Session()
	sesh.Println("UnsubscribeFromBlog handler...")
	r.MixpanelTrack("UnsubscribeFromBlog")

	token, err := uuid.Parse(r.GetURLQueryValue("token"))
	if err != nil {
		return nil, fmt.Errorf("parse token: %w", err)
	}
	sub, err := b.store.GetSubscriberByToken(context.TODO(), token)
	if err != nil {
		return nil, fmt.Errorf("get subscriber: %w", err)
	}
	if err := b.store.DeleteSubscriber(context.TODO(), sub.ID); err != nil {
		return nil, fmt.Errorf("delete subcriber: %w", err)
	}
	blog, err := b.store.GetBlogByID(context.TODO(), sub.BlogID)
	if err != nil {
		return nil, fmt.Errorf("get blog: %w", err)
	}
	return response.NewRedirect(
		fmt.Sprintf(
			"%s://%s.%s/unsubscribed",
			config.Config.Hylodoc.Protocol,
			blog.Subdomain,
			config.Config.Hylodoc.RootDomain,
		),
		http.StatusTemporaryRedirect,
	), nil
}

func (b *BlogService) EditSubscriber(
	r request.Request,
) (response.Response, error) {
	sesh := r.Session()
	sesh.Println("edit subscriber handler...")

	r.MixpanelTrack("EditSubscribers")

	email := r.GetURLQueryValue("email")

	blogID, ok := r.GetRouteVar("blogID")
	if !ok {
		return nil, createCustomError("", http.StatusNotFound)
	}
	intBlogID, err := strconv.ParseInt(blogID, 10, 32)
	if err != nil {
		return nil, fmt.Errorf("parse int: %w", err)
	}

	return response.NewTemplate(
		[]string{"subscriber_edit.html"},
		util.PageInfo{
			Data: struct {
				Title    string
				UserInfo *session.UserInfo

				Email               string
				RemoveSubscriberUrl string
			}{
				Title: "Edit Subscriber",
				UserInfo: session.ConvertSessionToUserInfo(
					r.Session(),
				),
				Email: email,
				RemoveSubscriberUrl: buildRemoveSubscriberUrl(
					int32(intBlogID), email,
				),
			},
		},
	), nil
}

func buildRemoveSubscriberUrl(blogID int32, email string) string {
	return fmt.Sprintf(
		"%s://%s/user/blogs/%d/subscriber/delete?email=%s",
		config.Config.Hylodoc.Protocol,
		config.Config.Hylodoc.RootDomain,
		blogID,
		email,
	)
}

func (b *BlogService) DeleteSubscriber(
	r request.Request,
) (response.Response, error) {
	sesh := r.Session()
	sesh.Println("DeleteSubscriber handler...")

	r.MixpanelTrack("DeleteSubscriber")
	email := r.GetURLQueryValue("email")

	blogID, ok := r.GetRouteVar("blogID")
	if !ok {
		return nil, createCustomError("", http.StatusNotFound)
	}

	if err := b.store.DeleteSubscriberByEmail(
		context.TODO(),
		model.DeleteSubscriberByEmailParams{
			BlogID: blogID,
			Email:  email,
		},
	); err != nil {
		return nil, fmt.Errorf("delete subscriber by email: %w", err)
	}

	return response.NewRedirect(
		buildSubscriberMetricsUrl(blogID),
		http.StatusSeeOther,
	), nil
}

func (b *BlogService) ExportSubscribers(
	r request.Request,
) (response.Response, error) {
	sesh := r.Session()
	sesh.Println("ExportSubscribers handler...")

	r.MixpanelTrack("ExportSubscribers")

	blogID, ok := r.GetRouteVar("blogID")
	if !ok {
		return nil, createCustomError("", http.StatusNotFound)
	}

	subs, err := b.store.ListActiveSubscribersByBlogID(
		context.TODO(), blogID,
	)
	if err != nil {
		return nil, fmt.Errorf("list subscribers: %w", err)
	}

	csv, err := buildSubscriberCSV(subs)
	if err != nil {
		return nil, fmt.Errorf("build csv: %w", err)
	}
	return response.NewCsvFile("subscribers.csv", csv), nil
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
