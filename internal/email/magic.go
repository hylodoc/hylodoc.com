package email

import (
	"fmt"

	"github.com/knuthic/knuthic/internal/config"
	"github.com/knuthic/knuthic/internal/email/internal/emailtemplate"
	"github.com/knuthic/knuthic/internal/model"
)

const (
	magicRegisterLinkSubject = "Confirm your Knuthic Account"
	magicLoginLinkSubject    = "Login to Knuthic"
)

func (s *sender) SendRegisterLink(token string) error {
	text, err := emailtemplate.NewRegisterLink(
		fmt.Sprintf(
			"%s://%s/%s?token=%s",
			config.Config.Knuthic.Protocol,
			config.Config.Knuthic.RootDomain,
			"magic/registercallback",
			token,
		),
	).Render(s.emailmode)
	if err != nil {
		return fmt.Errorf("cannot render template: %w", err)
	}
	if err := s.send(
		magicRegisterLinkSubject, text, model.PostmarkStreamOutbound,
	); err != nil {
		return fmt.Errorf("send error: %w", err)
	}
	return nil
}

func (s *sender) SendLoginLink(token string) error {
	text, err := emailtemplate.NewLoginLink(
		fmt.Sprintf(
			"%s://%s/%s?token=%s",
			config.Config.Knuthic.Protocol,
			config.Config.Knuthic.RootDomain,
			"magic/logincallback",
			token,
		),
	).Render(s.emailmode)
	if err != nil {
		return fmt.Errorf("cannot render template: %w", err)
	}
	if err := s.send(
		magicLoginLinkSubject, text, model.PostmarkStreamOutbound,
	); err != nil {
		return fmt.Errorf("send error: %w", err)
	}
	return nil
}
