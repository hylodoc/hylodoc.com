package email

import (
	"context"
	"fmt"

	"github.com/xr0-org/progstack/internal/email/emailaddr"
	"github.com/xr0-org/progstack/internal/email/postbody"
	"github.com/xr0-org/progstack/internal/model"
)

type Synthesiser interface {
	SendRegisterLink(token string) error
	SendLoginLink(token string) error

	SendNewSubscriberEmail(sitename, unsublink string) error

	SendNewPostUpdate(
		posttitle, postlink, unsublink string,
		body postbody.PostBody,
	) error
}

type synth struct {
	to, from  emailaddr.EmailAddr
	emailmode model.EmailMode
	store     *model.Store
}

func NewSynthesiser(
	to, from emailaddr.EmailAddr, mode model.EmailMode, store *model.Store,
) Synthesiser {
	return &synth{to, from, mode, store}
}

func (s *synth) send(subject, body string) error {
	return s.sendwithheaders(subject, body, nil)
}

func (s *synth) sendwithheaders(
	subject, body string, headers map[string]string,
) error {
	id, err := s.store.InsertQueuedEmail(
		context.TODO(),
		model.InsertQueuedEmailParams{
			FromAddr: s.from.Addr(),
			ToAddr:   s.to.Addr(),
			Subject:  subject,
			Body:     body,
			//Headers: headers,
			Mode: s.emailmode,
		},
	)
	if err != nil {
		return fmt.Errorf("insert: %w", err)
	}
	for name, value := range headers {
		if err := s.store.InsertQueuedEmailHeader(
			context.TODO(),
			model.InsertQueuedEmailHeaderParams{
				Email: id,
				Name:  name,
				Value: value,
			},
		); err != nil {
			return fmt.Errorf("header: %w", err)
		}
	}
	return nil
}
