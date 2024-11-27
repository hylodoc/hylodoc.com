package email

import (
	"fmt"

	"github.com/xr0-org/progstack/internal/email/emailtemplate"
	"github.com/xr0-org/progstack/internal/email/postbody"
)

func (s *sender) SendNewSubscriberEmail(sitename, unsublink string) error {
	text, err := emailtemplate.NewSubscriber(
		sitename, unsublink,
	).Render(s.emailmode)
	if err != nil {
		return fmt.Errorf("cannot render template: %w", err)
	}
	if err := s.sendwithheaders(
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
	posttitle, postlink, unsublink string, pb postbody.PostBody,
) error {
	body, err := pb.Read(s.emailmode)
	if err != nil {
		return fmt.Errorf("cannot read body: %w", err)
	}
	text, err := emailtemplate.NewPost(
		postlink, string(body), unsublink,
	).Render(s.emailmode)
	if err != nil {
		return fmt.Errorf("cannot render template: %w", err)
	}
	if err := s.sendwithheaders(
		posttitle, text, unsubscribeheaders(unsublink),
	); err != nil {
		return fmt.Errorf("error sending email: %w", err)
	}
	return nil
}
