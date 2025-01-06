package model

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
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

type RepositoryTxParams struct {
	RepositoryID int64
	Name         string
	FullName     string
	Url          string
}

type InstallationTxParams struct {
	InstallationID       int64
	UserID               int32
	Email                string
	RepositoriesTxParams []RepositoryTxParams
	RepositoriesPath     string
}

func (s *Store) CreateInstallationTx(arg InstallationTxParams) error {
	return s.ExecTx(func(tx *Store) error {
		installation, err := tx.CreateInstallation(
			context.TODO(),
			CreateInstallationParams{
				GhInstallationID: arg.InstallationID,
				UserID:           arg.UserID,
			},
		)
		if err != nil {
			return err
		}
		for _, repositoryTxParams := range arg.RepositoriesTxParams {
			_, err := tx.CreateRepository(
				context.TODO(),
				CreateRepositoryParams{
					InstallationID: installation.GhInstallationID,
					RepositoryID:   repositoryTxParams.RepositoryID,
					Name:           repositoryTxParams.Name,
					FullName:       repositoryTxParams.FullName,
					Url:            fmt.Sprintf("https://github.com/%s", repositoryTxParams.FullName), /* ghUrl not always in events */
					PathOnDisk: filepath.Join(
						arg.RepositoriesPath,
						repositoryTxParams.FullName,
					),
				},
			)
			if err != nil {
				return err
			}
		}
		return nil
	})
}
