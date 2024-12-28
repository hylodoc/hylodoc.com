package email

import (
	"fmt"

	"github.com/xr0-org/progstack/internal/config"
	"github.com/xr0-org/progstack/internal/email/internal/emailtemplate"
	"github.com/xr0-org/progstack/internal/model"
)

const (
	magicRegisterLinkSubject = "Confirm your Progstack Account"
	magicLoginLinkSubject    = "Login to Progstack"
)

func (s *sender) SendRegisterLink(token string) error {
	text, err := emailtemplate.NewRegisterLink(
		fmt.Sprintf(
			"%s://%s/%s?token=%s",
			config.Config.Progstack.Protocol,
			config.Config.Progstack.RootDomain,
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
			config.Config.Progstack.Protocol,
			config.Config.Progstack.RootDomain,
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
