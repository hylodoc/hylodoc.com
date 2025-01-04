package model

import (
	"context"
	"database/sql"
	"fmt"
)

// A Store models access to the DB in such a way that queries can be executed
// and transactions can be safely initiated (i.e. it refuses to nest
// transactions).
type Store struct {
	*Queries
	_intx bool
	_db   *sql.DB
}

func NewStore(db *sql.DB) *Store { return &Store{New(db), false, db} }

func (s *Store) ExecTx(fn func(*Store) error) error {
	if s._intx {
		return fmt.Errorf("cannot nest transactions")
	}
	tx, err := s._db.BeginTx(context.TODO(), nil)
	if err != nil {
		return err
	}
	if err = fn(&Store{New(tx), true, s._db}); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			return fmt.Errorf("tx err: %w, rb err: %w", err, rbErr)
		}
		return err
	}
	return tx.Commit()
}
