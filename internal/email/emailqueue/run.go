package emailqueue

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/resend/resend-go/v2"
	"github.com/xr0-org/progstack/internal/assert"
	"github.com/xr0-org/progstack/internal/config"
	"github.com/xr0-org/progstack/internal/model"
)

const resendBatchSize = 100

func Run(s *model.Store) error {
	r := resend.NewClient(config.Config.Email.ResendApiKey)
	for {
		if err := s.ExecTx(
			context.TODO(),
			func(q *model.Queries) error {
				return runbatchtx(r, q)
			},
		); err != nil {
			return err
		}
		time.Sleep(config.Config.Email.Queue.Period)
	}
}

func runbatchtx(r *resend.Client, q *model.Queries) error {
	emails, err := q.GetTopNQueuedEmails(context.TODO(), resendBatchSize)
	if err != nil {
		return fmt.Errorf("get top N: %w", err)
	}
	for _, e := range emails {
		if err := trysend(&e, r, q); err != nil {
			return fmt.Errorf("try send: %w", err)
		}
	}
	return nil
}

func trysend(e *model.QueuedEmail, r *resend.Client, q *model.Queries) error {
	if err := send(e, r, q); err != nil {
		/* TODO: detect critical error perhaps? */
		log.Printf("email send error %d: %s\n", e.ID, err)

		if err := q.IncrementQueuedEmailFailCount(
			context.TODO(),
			e.ID,
		); err != nil {
			return fmt.Errorf("failed count: %w", err)
		}
		assert.Assert(
			e.FailCount <= config.Config.Email.Queue.MaxRetries,
		)
		if e.FailCount == config.Config.Email.Queue.MaxRetries {
			if err := q.MarkQueuedEmailFailed(
				context.TODO(), e.ID,
			); err != nil {
				return fmt.Errorf("mark failed: %w", err)
			}
		}
		/* successfully incrementing the count or marking as failed is a
		 * failure to send but not a failure to *try* sending */
		return nil
	}
	if err := q.MarkQueuedEmailSent(context.TODO(), e.ID); err != nil {
		return fmt.Errorf("mark sent: %w", err)
	}
	return nil
}

func send(e *model.QueuedEmail, r *resend.Client, q *model.Queries) error {
	headers, err := getheaders(e.ID, q)
	if err != nil {
		return fmt.Errorf("headers: %w", err)
	}
	switch e.Mode {
	case model.EmailModePlaintext:
		_, err := r.Emails.SendWithContext(
			context.TODO(),
			&resend.SendEmailRequest{
				From:    e.FromAddr,
				To:      []string{e.ToAddr},
				Subject: e.Subject,
				Text:    e.Body,
				Headers: headers,
			},
		)
		if err != nil {
			return fmt.Errorf("plaintext: %w", err)
		}
		return nil
	case model.EmailModeHtml:
		_, err := r.Emails.SendWithContext(
			context.TODO(),
			&resend.SendEmailRequest{
				From:    e.FromAddr,
				To:      []string{e.ToAddr},
				Subject: e.Subject,
				Html:    e.Body,
				Headers: headers,
			},
		)
		if err != nil {
			return fmt.Errorf("html: %w", err)
		}
		return nil
	default:
		return fmt.Errorf("unknown mode %q", e.Mode)
	}
}

func getheaders(emailid int32, q *model.Queries) (map[string]string, error) {
	dbheaders, err := q.GetQueuedEmailHeaders(context.TODO(), emailid)
	if err != nil {
		return nil, err
	}
	headers := map[string]string{}
	for _, h := range dbheaders {
		headers[h.Name] = headers[h.Value]
	}
	return headers, nil
}
