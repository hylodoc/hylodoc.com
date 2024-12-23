package blog

import (
	"context"
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"github.com/xr0-org/progstack/internal/config"
	"github.com/xr0-org/progstack/internal/email"
	"github.com/xr0-org/progstack/internal/email/emailaddr"
	"github.com/xr0-org/progstack/internal/email/postbody"
	"github.com/xr0-org/progstack/internal/logging"
	"github.com/xr0-org/progstack/internal/model"
)

func (b *BlogService) SendPostEmail() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger := logging.Logger(r)
		logger.Println("SendPostEmail handler...")

		b.mixpanel.Track("SendPostEmail", r)

		if err := b.sendPostEmail(w, r); err != nil {
			logger.Printf("Error sending email: %v", err)
			http.Error(w, "", http.StatusInternalServerError)
			return
		}
	}
}

func (b *BlogService) sendPostEmail(
	w http.ResponseWriter, r *http.Request,
) error {
	token, err := uuid.Parse(r.URL.Query().Get("token"))
	if err != nil {
		return fmt.Errorf("failed to parse token: %w", err)
	}
	post, err := b.store.GetPostByToken(context.TODO(), token)
	if err != nil {
		return fmt.Errorf("cannot get post: %w", err)
	}
	blog, err := b.store.GetBlogByID(context.TODO(), post.Blog)
	if err != nil {
		return fmt.Errorf("cannot get blog: %w", err)
	}
	subscribers, err := b.store.ListActiveSubscribersByBlogID(
		context.TODO(), blog.ID,
	)
	if err != nil {
		return fmt.Errorf("cannot get subscribers: %w", err)
	}
	fromaddr := emailaddr.NewNamedAddr(
		getsitename(&blog),
		fmt.Sprintf(
			"%s@%s",
			blog.Subdomain,
			config.Config.Progstack.EmailDomain,
		),
	)
	for _, sub := range subscribers {
		token, err := b.store.InsertSubscriberEmail(
			context.TODO(),
			model.InsertSubscriberEmailParams{
				Subscriber: sub.ID,
				Url:        post.Url,
				Blog:       blog.ID,
			},
		)
		if err != nil {
			return fmt.Errorf("cannot insert email: %w", err)
		}
		if err != nil {
			return fmt.Errorf("url error: %w", err)
		}
		if err := email.NewSynthesiser(
			emailaddr.NewAddr(sub.Email),
			fromaddr,
			blog.EmailMode,
			b.store,
		).SendNewPostUpdate(
			post.Title,
			fmt.Sprintf(
				/* urls in post table begin with `/' so we omit
				 * it beneath */
				"%s://%s.%s%s?subscriber=%s",
				config.Config.Progstack.Protocol,
				blog.Subdomain,
				config.Config.Progstack.ServiceName,
				post.Url,
				token,
			),
			fmt.Sprintf(
				"%s://%s/blogs/unsubscribe?token=%s",
				config.Config.Progstack.Protocol,
				config.Config.Progstack.ServiceName,
				sub.UnsubscribeToken,
			),
			postbody.NewPostBody(
				post.HtmlEmailPath,
				post.TextEmailPath,
			),
		); err != nil {
			return fmt.Errorf(
				"error with subscriber %q: %w", sub.Email, err,
			)
		}
	}
	if err := b.store.SetPostEmailSent(
		context.TODO(),
		model.SetPostEmailSentParams{
			Url:  post.Url,
			Blog: post.Blog,
		},
	); err != nil {
		return fmt.Errorf("cannot set post email sent: %w", err)
	}
	http.Redirect(
		w, r,
		fmt.Sprintf(
			"%s://%s/user/blogs/%d/metrics",
			config.Config.Progstack.Protocol,
			config.Config.Progstack.ServiceName,
			blog.ID,
		),
		http.StatusTemporaryRedirect,
	)
	return nil
}
