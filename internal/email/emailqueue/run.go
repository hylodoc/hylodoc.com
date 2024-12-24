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
	"github.com/xr0-org/progstack/internal/metrics"
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
	batch, err := buildbatch(emails, q)
	if err != nil {
		return fmt.Errorf("build batch: %w", err)
	}
	responses, err := postmark.SendBatch(batch, c)
	if err != nil {
		return fmt.Errorf("send batch: %w", err)
	}
	for i := range responses {
		e := emails[i]
		if err := processresp(&e, responses[i], q); err != nil {
			return fmt.Errorf("process %d: %w", e.ID, err)
		}
	}
	return nil
}

func buildbatch(
	emails []model.QueuedEmail, q *model.Queries,
) ([]postmark.Email, error) {
	batch := make([]postmark.Email, len(emails))
	for i, e := range emails {
		headers, err := getheaders(e.ID, q)
		if err != nil {
			return nil, fmt.Errorf("headers: %w", err)
		}
		batch[i] = postmark.NewEmail(
			e.FromAddr, e.ToAddr, e.Subject, e.Body,
			e.Mode,
			e.Stream,
			headers,
		)
	}
	return batch, nil
}

func processresp(
	e *model.QueuedEmail, resp postmark.Response, q *model.Queries,
) error {
	if resp.ErrorCode() == 0 {
		if err := q.MarkQueuedEmailSent(
			context.TODO(), e.ID,
		); err != nil {
			return fmt.Errorf("mark sent: %w", err)
		}
		metrics.RecordEmailInBatchSuccess(e.Stream)
		return nil
	}
	metrics.RecordEmailInBatchError(e.Stream)
	log.Printf(
		"e.ID send error %d: code: %d, message: %s\n",
		e.ID, resp.ErrorCode(), resp.Message(),
	)
	if err := q.InsertQueuedEmailPostmarkError(
		context.TODO(),
		model.InsertQueuedEmailPostmarkErrorParams{
			Email:   e.ID,
			Code:    int32(resp.ErrorCode()),
			Message: resp.Message(),
		},
	); err != nil {
		return fmt.Errorf("save postmark error: %w", err)
	}
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
