package blog

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/lib/pq"
	"github.com/xr0-org/progstack/internal/app/handler/request"
	"github.com/xr0-org/progstack/internal/app/handler/response"
	"github.com/xr0-org/progstack/internal/config"
	"github.com/xr0-org/progstack/internal/email"
	"github.com/xr0-org/progstack/internal/email/emailaddr"
	"github.com/xr0-org/progstack/internal/logging"
	"github.com/xr0-org/progstack/internal/model"
	"github.com/xr0-org/progstack/internal/session"
	"github.com/xr0-org/progstack/internal/util"
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
	logger := r.Logger()
	logger.Println("SubscribeToBlog handler...")

	r.MixpanelTrack("SubscribeToBlog")

	/* TODO: validate email format */
	e, err := r.GetPostFormValue("email")
	if err != nil {
		return nil, fmt.Errorf("get email: %w", err)
	}

	blogIDRaw, ok := r.GetRouteVar("blogID")
	if !ok {
		return nil, fmt.Errorf("no blogID")
	}
	blogID, err := strconv.ParseInt(blogIDRaw, 10, 32)
	if err != nil {
		return nil, fmt.Errorf("parse blogID: %w", err)
	}
	blog, err := b.store.GetBlogByID(context.TODO(), int32(blogID))
	if err != nil {
		return nil, fmt.Errorf("get blog: %w", err)
	}

	unsubtoken, err := b.createsubscriber(e, blog.ID, logger)
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
				config.Config.Progstack.EmailDomain,
			),
		),
		blog.EmailMode,
		b.store,
	).SendNewSubscriberEmail(
		sitename,
		fmt.Sprintf(
			"%s://%s/blogs/unsubscribe?token=%s",
			config.Config.Progstack.Protocol,
			config.Config.Progstack.ServiceName,
			unsubtoken,
		),
	); err != nil {
		return nil, fmt.Errorf("send new subscriber email: %w", err)
	}

	return response.NewRedirect(
		fmt.Sprintf(
			"%s://%s.%s/subscribed",
			config.Config.Progstack.Protocol,
			blog.Subdomain,
			config.Config.Progstack.ServiceName,
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
	email string, blog int32, logger *log.Logger,
) (string, error) {
	logger.Printf("subscribing email `%s' to blog %d\n", email, blog)
	tk, err := b.createorgetsubscriber(email, blog, logger)
	if err != nil {
		return "", fmt.Errorf("cannot create or get subscriber: %w", err)
	}
	return tk.String(), nil
}

func (b *BlogService) createorgetsubscriber(
	email string, blog int32, logger *log.Logger,
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
			logger.Println("duplicate subscription")
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
	logger := r.Logger()
	logger.Println("UnsubscribeFromBlog handler...")
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
			config.Config.Progstack.Protocol,
			blog.Subdomain,
			config.Config.Progstack.ServiceName,
		),
		http.StatusTemporaryRedirect,
	), nil
}

func (b *BlogService) EditSubscriber() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger := logging.Logger(r)
		logger.Println("edit subscriber handler...")

		email := r.URL.Query().Get("email")

		sesh, ok := r.Context().Value(session.CtxSessionKey).(*session.Session)
		if !ok {
			logger.Println("No auth session")
			http.Error(w, "", http.StatusNotFound)
			return
		}

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
					Title    string
					UserInfo *session.UserInfo

					Email               string
					RemoveSubscriberUrl string
				}{
					Title:    "Edit Subscriber",
					UserInfo: session.ConvertSessionToUserInfo(sesh),

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
