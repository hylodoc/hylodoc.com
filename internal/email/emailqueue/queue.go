package emailqueue

import (
	"context"
	"fmt"

	"github.com/xr0-org/progstack/internal/model"
)

type Email struct {
	from, to, subject, body string
	mode                    model.EmailMode
	headers                 map[string]string
}

func NewEmail(
	from, to, subject, body string, mode model.EmailMode,
	headers map[string]string,
) *Email {
	return &Email{from, to, subject, body, mode, headers}
}

func (e *Email) Queue(store *model.Store) error {
	return store.ExecTx(context.TODO(), e.queuetx)
}

func (e *Email) queuetx(q *model.Queries) error {
	id, err := q.InsertQueuedEmail(
		context.TODO(),
		model.InsertQueuedEmailParams{
			FromAddr: e.from,
			ToAddr:   e.to,
			Subject:  e.subject,
			Body:     e.body,
			Mode:     e.mode,
		},
	)
	if err != nil {
		return fmt.Errorf("insert: %w", err)
	}
	for name, value := range e.headers {
		if err := q.InsertQueuedEmailHeader(
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
