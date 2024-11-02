package email

import (
	"context"
	"fmt"

	"github.com/resend/resend-go/v2"
	"github.com/xr0-org/progstack/internal/config"
)

const (
	magicRegisterLinkSubject = "Confirm your Progstack Account"
	magicLoginLinkSubject    = "Login to Progstack"
)

type MagicLinkParams struct {
	To    string
	Token string
}

func SendRegisterLink(c *resend.Client, params MagicLinkParams) error {
	to := params.To
	link := buildRegisterLink(params.Token)
	body := fmt.Sprintf("Click here to register: %s", link)

	_, err := c.Emails.SendWithContext(context.TODO(), &resend.SendEmailRequest{
		From:    config.Config.Progstack.FromEmail,
		To:      []string{params.To},
		Subject: magicRegisterLinkSubject,
		Text:    body,
	})
	if err != nil {
		return fmt.Errorf("error sending register email to `%s: %w", to, err)
	}

	return nil
}

/* build magic register link */
func buildRegisterLink(token string) string {
	return fmt.Sprintf("%s://%s/%s?token=%s",
		config.Config.Progstack.Protocol,
		config.Config.Progstack.ServiceName,
		"magic/registercallback",
		token,
	)
}

func SendLoginLink(c *resend.Client, params MagicLinkParams) error {
	to := params.To
	link := buildLoginLink(params.Token)
	body := fmt.Sprintf("Click here to login: %s", link)

	_, err := c.Emails.SendWithContext(context.TODO(), &resend.SendEmailRequest{
		From:    config.Config.Progstack.FromEmail,
		To:      []string{params.To},
		Subject: magicLoginLinkSubject,
		Text:    body,
	})
	if err != nil {
		return fmt.Errorf("error sending login email to `%s: %w", to, err)
	}

	return nil
}

/* build magic login link */
func buildLoginLink(token string) string {
	return fmt.Sprintf("%s://%s/%s?token=%s",
		config.Config.Progstack.Protocol,
		config.Config.Progstack.ServiceName,
		"magic/logincallback",
		token,
	)
}
