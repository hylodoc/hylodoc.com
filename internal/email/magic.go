package email

import (
	"fmt"

	"github.com/hylodoc/hylodoc.com/internal/config"
	"github.com/hylodoc/hylodoc.com/internal/email/internal/emailtemplate"
	"github.com/hylodoc/hylodoc.com/internal/model"
)

const (
	magicRegisterLinkSubject = "Confirm your Hylodoc Account"
	magicLoginLinkSubject    = "Login to Hylodoc"
)

func (s *sender) SendRegisterLink(token string) error {
	text, err := emailtemplate.NewRegisterLink(
		fmt.Sprintf(
			"%s://%s/%s?token=%s",
			config.Config.Hylodoc.Protocol,
			config.Config.Hylodoc.RootDomain,
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
			config.Config.Hylodoc.Protocol,
			config.Config.Hylodoc.RootDomain,
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
