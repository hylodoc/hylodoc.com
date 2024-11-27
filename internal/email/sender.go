package email

import (
	"context"
	"fmt"

	"github.com/resend/resend-go/v2"
	"github.com/xr0-org/progstack/internal/email/emailaddr"
	"github.com/xr0-org/progstack/internal/email/postbody"
	"github.com/xr0-org/progstack/internal/model"
)

type Sender interface {
	SendRegisterLink(token string) error
	SendLoginLink(token string) error

	SendNewSubscriberEmail(sitename, unsublink string) error

	SendNewPostUpdate(
		posttitle, postlink, unsublink string,
		body postbody.PostBody,
	) error
}

type sender struct {
	to, from  emailaddr.EmailAddr
	client    *resend.Client
	emailmode model.EmailMode
}

func NewSender(
	to, from emailaddr.EmailAddr, c *resend.Client, mode model.EmailMode,
) Sender {
	return &sender{to, from, c, mode}
}

func (s *sender) send(subject, body string) error {
	return s.sendwithheaders(subject, body, nil)
}

func (s *sender) sendwithheaders(
	subject, body string, headers map[string]string,
) error {
	switch s.emailmode {
	case model.EmailModePlaintext:
		_, err := s.client.Emails.SendWithContext(
			context.TODO(),
			&resend.SendEmailRequest{
				From:    s.from.Addr(),
				To:      []string{s.to.Addr()},
				Subject: subject,
				Text:    body,
				Headers: headers,
			},
		)
		if err != nil {
			return fmt.Errorf("plaintext: %w", err)
		}
		return nil
	case model.EmailModeHtml:
		_, err := s.client.Emails.SendWithContext(
			context.TODO(),
			&resend.SendEmailRequest{
				From:    s.from.Addr(),
				To:      []string{s.to.Addr()},
				Subject: subject,
				Html:    body,
				Headers: headers,
			},
		)
		if err != nil {
			return fmt.Errorf("html: %w", err)
		}
		return nil
	default:
		return fmt.Errorf("unknown email mode %q", s.emailmode)
	}
}
