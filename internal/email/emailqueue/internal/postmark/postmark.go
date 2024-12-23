package postmark

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/xr0-org/progstack/internal/config"
	"github.com/xr0-org/progstack/internal/httpclient"
	"github.com/xr0-org/progstack/internal/model"
)

type Email interface {
	Send(c *httpclient.Client) error
}

func NewEmail(
	from, to, subject, body string, mode model.EmailMode,
	headers map[string]string,
) Email {
	return &email{from, to, subject, body, mode, headers}
}

type email struct {
	from, to, subject, body string
	mode                    model.EmailMode
	headers                 map[string]string
}

func (e *email) Send(c *httpclient.Client) error {
	payload, err := e.payload()
	if err != nil {
		return fmt.Errorf("payload: %w", err)
	}
	req, err := http.NewRequest(
		"POST",
		"https://api.postmarkapp.com/email",
		bytes.NewBuffer(payload),
	)
	if err != nil {
		return fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(
		"X-Postmark-Server-Token",
		config.Config.Email.PostmarkApiKey,
	)
	resp, err := c.Do(req)
	if err != nil {
		return fmt.Errorf("do: %w", err)
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf(
			"status: %d, body: %s",
			resp.StatusCode,
			string(body),
		)
	}
	return nil
}

func (e *email) payload() ([]byte, error) {
	headers := getheaders(e.headers)
	type payload struct {
		From          string
		To            string
		Subject       string
		TextBody      string
		HtmlBody      string
		Headers       []header
		MessageStream string
	}
	switch e.mode {
	case model.EmailModePlaintext:
		return json.Marshal(payload{
			From:          e.from,
			To:            e.to,
			Subject:       e.subject,
			TextBody:      e.body,
			Headers:       headers,
			MessageStream: "outbound", /* XXX */
		})
	case model.EmailModeHtml:
		return json.Marshal(payload{
			From:          e.from,
			To:            e.to,
			Subject:       e.subject,
			HtmlBody:      e.body,
			Headers:       headers,
			MessageStream: "outbound", /* XXX */
		})
	default:
		return nil, fmt.Errorf("invalid mode %q", e.mode)
	}
}

type header struct {
	Name, Value string
}

func getheaders(h map[string]string) []header {
	var headers []header
	for name, value := range h {
		headers = append(headers, header{name, value})
	}
	return headers
}
