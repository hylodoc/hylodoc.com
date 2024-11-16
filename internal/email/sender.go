package email

import (
	"context"
	"fmt"

	"github.com/resend/resend-go/v2"
	"github.com/xr0-org/progstack/internal/config"
	"github.com/xr0-org/progstack/internal/model"
)

type Sender interface {
	SendRegisterLink(to, token string) error
	SendLoginLink(to, token string) error

	SendNewSubscriberEmail(to, sitename, unsublink string) error

	SendNewPostUpdate(
		to, posttitle, postlink, postbody, unsublink string,
	) error
}

type sender struct {
	client    *resend.Client
	emailmode model.EmailMode
}

func NewSender(c *resend.Client, mode model.EmailMode) Sender {
	return &sender{c, mode}
}

func (s *sender) send(
	to, from, subject, body string, headers map[string]string,
) error {
	switch s.emailmode {
	case model.EmailModePlaintext:
		_, err := s.client.Emails.SendWithContext(
			context.TODO(),
			&resend.SendEmailRequest{
				From:    config.Config.Progstack.FromEmail,
				To:      []string{to},
				Subject: subject,
				Text:    body,
				Headers: headers,
			},
		)
		return err
	case model.EmailModeHtml:
		_, err := s.client.Emails.SendWithContext(
			context.TODO(),
			&resend.SendEmailRequest{
				From:    config.Config.Progstack.FromEmail,
				To:      []string{to},
				Subject: subject,
				Html:    body,
				Headers: headers,
			},
		)
		return err
	default:
		return fmt.Errorf("unknown email mode %q", s.emailmode)
	}
}
