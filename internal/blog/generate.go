package blog

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/google/uuid"
	"github.com/xr0-org/progstack-ssg/pkg/ssg"
	"github.com/xr0-org/progstack/internal/assert"
	"github.com/xr0-org/progstack/internal/authz"
	"github.com/xr0-org/progstack/internal/config"
	"github.com/xr0-org/progstack/internal/model"
)

func GetFreshGeneration(blogid string, s *model.Store) (int32, error) {
	freshgen, err := s.GetFreshGeneration(context.TODO(), blogid)
	if err == nil {
		return freshgen, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return -1, fmt.Errorf("query error: %w", err)
	}
	assert.Assert(errors.Is(err, sql.ErrNoRows))

	b, err := s.GetBlogByID(context.TODO(), blogid)
	if err != nil {
		return -1, fmt.Errorf("cannot get blog: %w", err)
	}
	site, err := ssgGenerateWithAuthZRestrictions(&b, s)
	if err != nil {
		return -1, fmt.Errorf("generate with authz: %w", err)
	}
	if title := site.Title(); title != "" {
		if err := s.UpdateBlogName(
			context.TODO(),
			model.UpdateBlogNameParams{
				ID:   b.ID,
				Name: title,
			},
		); err != nil {
			return -1, fmt.Errorf("cannot set title %q: %w", title, err)
		}
	}
	if !b.LiveHash.Valid {
		return -1, fmt.Errorf("no live hash: %w", err)
	}
	gen, err := s.InsertGeneration(context.TODO(), b.LiveHash.String)
	if err != nil {
		return -1, fmt.Errorf("error inserting generation: %w", err)
	}
	for url, rsc := range site.Bindings() {
		if err := s.InsertBinding(
			context.TODO(),
			model.InsertBindingParams{
				Gen:  gen,
				Url:  url,
				Path: rsc.Path(),
			},
		); err != nil {
			return -1, fmt.Errorf("error inserting binding: %w", err)
		}
		if rsc.IsPost() {
			post := rsc.Post()
			if err := upsertPost(
				post, url, b.ID, s,
			); err != nil {
				return -1, fmt.Errorf(
					"error upserting post: %w", err,
				)
			}
			if err := s.InsertPostEmailBinding(
				context.TODO(),
				model.InsertPostEmailBindingParams{
					Gen:  gen,
					Url:  url,
					Html: post.HtmlPath(),
					Text: post.PlaintextPath(),
				},
			); err != nil {
				return -1, fmt.Errorf(
					"cannot insert email params: %w", err,
				)
			}
		}
	}
	return gen, nil
}

func ssgGenerateWithAuthZRestrictions(
	b *model.Blog, s *model.Store,
) (ssg.Site, error) {
	canHaveSubs, err := authz.HasAnalyticsCustomDomainsImagesEmails(
		s, b.UserID,
	)
	if err != nil {
		return nil, fmt.Errorf("can have subscribers: %w", err)
	}
	assert.Assert(b.LiveHash.Valid)
	src := filepath.Join(
		config.Config.Progstack.CheckoutsPath,
		b.LiveHash.String,
	)
	dst := filepath.Join(
		config.Config.Progstack.WebsitesPath,
		b.Subdomain.String(),
		uuid.New().String(),
	)
	if !canHaveSubs {
		return ssg.GenerateSiteWithBindings(
			src,
			dst,
			config.Config.ProgstackSsg.Themes[string(b.Theme)].Path,
			"algol_nu", "", "",
			map[string]ssg.CustomPage{
				"/unsubscribed": ssg.NewMessagePage(
					"Unsubscribed",
					`<p>
					You have been unsubscribed from this site.
					You will no longer receive email updates for posts.
				</p>`,
				),
			},
		)
	}
	return ssg.GenerateSiteWithBindings(
		src,
		dst,
		config.Config.ProgstackSsg.Themes[string(b.Theme)].Path,
		"algol_nu",
		"",
		"<p>Subscribe via <a href=\"/subscribe\">email</a>.</p>",
		map[string]ssg.CustomPage{
			"/subscribe": ssg.NewSubscriberPage(
				fmt.Sprintf(
					"%s://%s/blogs/%s/subscribe",
					config.Config.Progstack.Protocol,
					config.Config.Progstack.RootDomain,
					b.ID,
				),
			),
			"/subscribed": ssg.NewMessagePage(
				"Subscribed",
				"<p>You have been subscribed. Please check your email.</p>",
			),
			"/unsubscribed": ssg.NewMessagePage(
				"Unsubscribed",
				`<p>
					You have been unsubscribed from this site.
					You will no longer receive email updates for posts.
				</p>
				<p>
					If this was a mistake, you can resubscribe
					<a href="/subscribe">here</a>.
				</p>`,
			),
		},
	)
}

func upsertPost(
	post ssg.Post, url string, blogid string, s *model.Store,
) error {
	params := model.InsertRPostParams{
		Url:         url,
		Blog:        blogid,
		PublishedAt: publishedat(post),
		Title:       post.Title(),
	}
	if _, err := s.GetPostExists(
		context.TODO(),
		model.GetPostExistsParams{
			Url:  url,
			Blog: blogid,
		},
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return s.InsertRPost(context.TODO(), params)
		}
		return fmt.Errorf("error checking if exists: %w", err)
	}
	return s.UpdateRPost(context.TODO(), model.UpdateRPostParams(params))
}

func publishedat(post ssg.Post) sql.NullTime {
	t, ok := post.Time()
	return sql.NullTime{t, ok}
}
