package emailqueue

import (
	"context"
	"fmt"

	"github.com/hylodoc/hylodoc.com/internal/model"
)

type Email interface {
	Queue(*model.Store) error
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

func (e *email) Queue(store *model.Store) error {
	return store.ExecTx(e.queuetx)
}

func (e *email) queuetx(s *model.Store) error {
	id, err := s.InsertQueuedEmail(
		context.TODO(),
		model.InsertQueuedEmailParams{
			FromAddr: e.from,
			ToAddr:   e.to,
			Subject:  e.subject,
			Body:     e.body,
			Mode:     e.mode,
			Stream:   e.stream,
		},
	)
	if err != nil {
		return fmt.Errorf("insert: %w", err)
	}
	for name, value := range e.headers {
		if err := s.InsertQueuedEmailHeader(
			context.TODO(),
			model.InsertQueuedEmailHeaderParams{
				Email: id,
				Name:  name,
				Value: value,
			},
		); err != nil {
			return fmt.Errorf("header: %w", err)
		}
	}
	return nil
}
