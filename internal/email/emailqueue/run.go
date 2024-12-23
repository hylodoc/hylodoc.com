package emailqueue

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/xr0-org/progstack/internal/assert"
	"github.com/xr0-org/progstack/internal/config"
	"github.com/xr0-org/progstack/internal/email/emailqueue/internal/postmark"
	"github.com/xr0-org/progstack/internal/httpclient"
	"github.com/xr0-org/progstack/internal/model"
)

const postmarkBatchSize = 500

func Run(c *httpclient.Client, s *model.Store) error {
	period := config.Config.Email.Queue.Period
	if period == 0 {
		return fmt.Errorf("no period")
	}
	for {
		if err := s.ExecTx(
			context.TODO(),
			func(q *model.Queries) error {
				return runbatchtx(c, q)
			},
		); err != nil {
			return err
		}
		time.Sleep(period)
	}
}

func runbatchtx(c *httpclient.Client, q *model.Queries) error {
	emails, err := q.GetTopNQueuedEmails(context.TODO(), postmarkBatchSize)
	if err != nil {
		return fmt.Errorf("get top N: %w", err)
	}
	for _, e := range emails {
		if err := trysend(&e, c, q); err != nil {
			return fmt.Errorf("try send: %w", err)
		}
	}
	return nil
}

func trysend(e *model.QueuedEmail, c *httpclient.Client, q *model.Queries) error {
	headers, err := getheaders(e.ID, q)
	if err != nil {
		return fmt.Errorf("headers: %w", err)
	}
	if err := postmark.NewEmail(
		e.FromAddr, e.ToAddr, e.Subject, e.Body, e.Mode, headers,
	).Send(c); err != nil {
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
