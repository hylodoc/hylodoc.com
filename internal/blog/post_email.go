package blog

import (
	"context"
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"github.com/knuthic/knuthic/internal/app/handler/request"
	"github.com/knuthic/knuthic/internal/app/handler/response"
	"github.com/knuthic/knuthic/internal/authz"
	"github.com/knuthic/knuthic/internal/config"
	"github.com/knuthic/knuthic/internal/email"
	"github.com/knuthic/knuthic/internal/email/emailaddr"
	"github.com/knuthic/knuthic/internal/email/postbody"
	"github.com/knuthic/knuthic/internal/model"
)

func (b *BlogService) SendPostEmail(
	r request.Request,
) (response.Response, error) {
	sesh := r.Session()
	sesh.Println("SendPostEmail handler...")

	r.MixpanelTrack("SendPostEmail")

	userID, err := sesh.GetUserID()
	if err != nil {
		return nil, fmt.Errorf("get user id: %w", err)
	}
	canSend, err := authz.HasAnalyticsCustomDomainsImagesEmails(
		b.store, userID,
	)
	if err != nil {
		return nil, fmt.Errorf("can send email: %w", err)
	}
	if !canSend {
		return nil, authz.SubscriptionError
	}

	token, err := uuid.Parse(r.GetURLQueryValue("token"))
	if err != nil {
		return nil, fmt.Errorf("parse uuid: %w", err)
	}
	post, err := b.store.GetPostByToken(
		context.TODO(), token,
	)
	if err != nil {
		return nil, fmt.Errorf("get post: %w", err)
	}
	blog, err := b.store.GetBlogByID(context.TODO(), post.Blog)
	if err != nil {
		return nil, fmt.Errorf("get blog: %w", err)
	}
	subscribers, err := b.store.ListActiveSubscribersByBlogID(
		context.TODO(), blog.ID,
	)
	if err != nil {
		return nil, fmt.Errorf("get active subscribers: %w", err)
	}
	fromaddr := emailaddr.NewNamedAddr(
		getsitename(&blog),
		fmt.Sprintf(
			"%s@%s",
			blog.Subdomain,
			config.Config.Knuthic.EmailDomain,
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
			return nil, fmt.Errorf("cannot insert email: %w", err)
		}
		if err != nil {
			return nil, fmt.Errorf("url error: %w", err)
		}
		if err := email.NewSender(
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
				config.Config.Knuthic.Protocol,
				blog.Subdomain,
				config.Config.Knuthic.RootDomain,
				post.Url,
				token,
			),
			fmt.Sprintf(
				"%s://%s/blogs/unsubscribe?token=%s",
				config.Config.Knuthic.Protocol,
				config.Config.Knuthic.RootDomain,
				sub.UnsubscribeToken,
			),
			postbody.NewPostBody(
				post.HtmlEmailPath,
				post.TextEmailPath,
			),
		); err != nil {
			return nil, fmt.Errorf(
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
		return nil, fmt.Errorf("cannot set post email sent: %w", err)
	}
	return response.NewRedirect(
		fmt.Sprintf(
			"%s://%s/user/blogs/%s/metrics",
			config.Config.Knuthic.Protocol,
			config.Config.Knuthic.RootDomain,
			blog.ID,
		),
		http.StatusTemporaryRedirect,
	), nil
}
