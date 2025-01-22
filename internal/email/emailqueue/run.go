package emailqueue

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/hylodoc/hylodoc.com/internal/assert"
	"github.com/hylodoc/hylodoc.com/internal/config"
	"github.com/hylodoc/hylodoc.com/internal/email/emailqueue/internal/postmark"
	"github.com/hylodoc/hylodoc.com/internal/httpclient"
	"github.com/hylodoc/hylodoc.com/internal/metrics"
	"github.com/hylodoc/hylodoc.com/internal/model"
)

const postmarkBatchSize = 500

func Run(c *httpclient.Client, s *model.Store) error {
	period := config.Config.Email.Queue.Period
	if period == 0 {
		return fmt.Errorf("no period")
	}
	for {
		if err := s.ExecTx(
			func(s *model.Store) error {
				return runbatchtx(c, s)
			},
		); err != nil {
			return err
		}
		time.Sleep(period)
	}
}

func runbatchtx(c *httpclient.Client, s *model.Store) error {
	emails, err := s.GetTopNQueuedEmails(context.TODO(), postmarkBatchSize)
	if err != nil {
		return fmt.Errorf("get top N: %w", err)
	}
	batch, err := buildbatch(emails, s)
	if err != nil {
		return fmt.Errorf("build batch: %w", err)
	}
	responses, err := postmark.SendBatch(batch, c)
	if err != nil {
		return fmt.Errorf("send batch: %w", err)
	}
	for i := range responses {
		e := emails[i]
		if err := processresp(&e, responses[i], s); err != nil {
			return fmt.Errorf("process %d: %w", e.ID, err)
		}
	}
	return nil
}

func buildbatch(
	emails []model.QueuedEmail, s *model.Store,
) ([]postmark.Email, error) {
	batch := make([]postmark.Email, len(emails))
	for i, e := range emails {
		headers, err := getheaders(e.ID, s)
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
	e *model.QueuedEmail, resp postmark.Response, s *model.Store,
) error {
	if resp.ErrorCode() == 0 {
		if err := s.MarkQueuedEmailSent(
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
	if err := s.InsertQueuedEmailPostmarkError(
		context.TODO(),
		model.InsertQueuedEmailPostmarkErrorParams{
			Email:   e.ID,
			Code:    int32(resp.ErrorCode()),
			Message: resp.Message(),
		},
	); err != nil {
		return fmt.Errorf("save postmark error: %w", err)
	}
	if err := s.IncrementQueuedEmailFailCount(
		context.TODO(),
		e.ID,
	); err != nil {
		return fmt.Errorf("failed count: %w", err)
	}
	assert.Assert(
		e.FailCount <= config.Config.Email.Queue.MaxRetries,
	)
	if e.FailCount == config.Config.Email.Queue.MaxRetries {
		if err := s.MarkQueuedEmailFailed(
			context.TODO(), e.ID,
		); err != nil {
			return fmt.Errorf("mark failed: %w", err)
		}
	}
	/* successfully incrementing the count or marking as failed is a
	 * failure to send but not a failure to *try* sending */
	return nil
}

func getheaders(emailid int32, s *model.Store) (map[string]string, error) {
	dbheaders, err := s.GetQueuedEmailHeaders(context.TODO(), emailid)
	if err != nil {
		return nil, err
	}
	headers := map[string]string{}
	for _, h := range dbheaders {
		headers[h.Name] = headers[h.Value]
	}
	return headers, nil
}
