package email

import (
	"fmt"

	"github.com/xr0-org/progstack/internal/config"
	"github.com/xr0-org/progstack/internal/email/emailtemplate"
)

func (s *sender) SendNewSubscriberEmail(to, sitename, unsublink string) error {
	text, err := emailtemplate.NewSubscriber(
		sitename, unsublink,
	).Render(s.emailmode)
	if err != nil {
		return fmt.Errorf("cannot render template: %w", err)
	}
	if err := s.send(
		to,
		config.Config.Progstack.FromEmail,
		fmt.Sprintf("Welcome to %s", sitename),
		text,
		unsubscribeheaders(unsublink),
	); err != nil {
		return fmt.Errorf("error sending email: %w", err)
	}
	return nil
}

func unsubscribeheaders(unsublink string) map[string]string {
	return map[string]string{
		"List-Unsubscribe-Post": "List-Unsubscribe=One-Click",
		"List-Unsubscribe":      fmt.Sprintf("<%s>", unsublink),
	}
}

func (s *sender) SendNewPostUpdate(
	to, posttitle, postlink, postbody, unsublink string,
) error {
	text, err := emailtemplate.NewPost(
		postlink, postbody, unsublink,
	).Render(s.emailmode)
	if err != nil {
		return fmt.Errorf("cannot render template: %w", err)
	}
	if err := s.send(
		to,
		config.Config.Progstack.FromEmail,
		posttitle,
		text,
		unsubscribeheaders(unsublink),
	); err != nil {
		return fmt.Errorf("error sending email: %w", err)
	}
	return nil
}
