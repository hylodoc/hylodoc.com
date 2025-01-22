package postmark

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/hylodoc/hylodoc.com/internal/config"
	"github.com/hylodoc/hylodoc.com/internal/httpclient"
	"github.com/hylodoc/hylodoc.com/internal/metrics"
	"github.com/hylodoc/hylodoc.com/internal/model"
)

type Email interface {
	payload() (*payload, error)
}

type email struct {
	from, to, subject, body string
	mode                    model.EmailMode
	stream                  model.PostmarkStream
	headers                 map[string]string
}

func NewEmail(
	from, to, subject, body string,
	mode model.EmailMode,
	stream model.PostmarkStream,
	headers map[string]string,
) Email {
	return &email{from, to, subject, body, mode, stream, headers}
}

type payload struct {
	From          string
	To            string
	Subject       string
	TextBody      string
	HtmlBody      string
	Headers       []header
	MessageStream model.PostmarkStream
}

func (e *email) payload() (*payload, error) {
	headers := getheaders(e.headers)
	switch e.mode {
	case model.EmailModePlaintext:
		return &payload{
			From:          e.from,
			To:            e.to,
			Subject:       e.subject,
			TextBody:      e.body,
			Headers:       headers,
			MessageStream: e.stream,
		}, nil
	case model.EmailModeHtml:
		return &payload{
			From:          e.from,
			To:            e.to,
			Subject:       e.subject,
			HtmlBody:      e.body,
			Headers:       headers,
			MessageStream: e.stream,
		}, nil
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

type Response interface {
	ErrorCode() int
	Message() string
}

func SendBatch(emails []Email, c *httpclient.Client) ([]Response, error) {
	batch, err := batchpayload(emails)
	if err != nil {
		return nil, fmt.Errorf("payload: %w", err)
	}
	payload, err := json.Marshal(batch)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}
	req, err := http.NewRequest(
		"POST",
		"https://api.postmarkapp.com/email/batch",
		bytes.NewBuffer(payload),
	)
	if err != nil {
		return nil, fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(
		"X-Postmark-Server-Token",
		config.Config.Email.PostmarkApiKey,
	)
	metrics.RecordEmailBatchRequest()
	resp, err := c.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do: %w", err)
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		metrics.RecordEmailBatchError()
		return nil, fmt.Errorf(
			"status: %d, body: %s",
			resp.StatusCode,
			string(body),
		)
	}
	emailresps, err := unmarshalbatchresponse(body)
	if err != nil {
		return nil, fmt.Errorf("unmarshal batch: %w", err)
	}
	metrics.RecordEmailBatchSuccess()
	return emailresps, nil
}

func batchpayload(batch []Email) ([]payload, error) {
	p := make([]payload, len(batch))
	for i := range batch {
		payload, err := batch[i].payload()
		if err != nil {
			return nil, fmt.Errorf("%v: %w", batch[i], err)
		}
		p[i] = *payload
	}
	return p, nil
}

type postmarkresponse struct {
	ErrorCode_ int    `json:"ErrorCode"`
	Message_   string `json:"Message"`
}

func (r *postmarkresponse) ErrorCode() int  { return r.ErrorCode_ }
func (r *postmarkresponse) Message() string { return r.Message_ }

func unmarshalbatchresponse(body []byte) ([]Response, error) {
	var resp []postmarkresponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}
	return convert(resp), nil
}

func convert(pmresps []postmarkresponse) []Response {
	resps := make([]Response, len(pmresps))
	for i := range resps {
		resps[i] = &pmresps[i]
	}
	return resps
}
