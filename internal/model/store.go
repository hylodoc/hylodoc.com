package model

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
)

/* A Store models access to the DB in such a way that queries can be executed
 * and transactions can be safely initiated (i.e. it refuses to nest
 * transactions. */
type Store struct {
	*Queries
	_intx bool
	_db   *sql.DB
}

func NewStore(db *sql.DB) *Store { return &Store{New(db), false, db} }

func (s *Store) ExecTx(ctx context.Context, fn func(*Store) error) error {
	if s._intx {
		return fmt.Errorf("cannot nest transactions")
	}
	tx, err := s._db.BeginTx(ctx, nil)
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

func (s *Store) CreateUserTx(ctx context.Context, arg CreateUserParams) (*User, error) {
	var res User
	if err := s.ExecTx(ctx, func(tx *Store) error {
		/* create user */
		u, err := tx.CreateUser(ctx, CreateUserParams{
			Email:    arg.Email,
			Username: arg.Username,
		})
		if err != nil {
			return fmt.Errorf("error creating user: %w", err)
		}
		res = u
		return nil
	}); err != nil {
		return nil, fmt.Errorf("error in creating user tx: %w", err)
	}
	return &res, nil
}

func (s *Store) CreateUserWithGithubAccountTx(ctx context.Context, arg CreateGithubAccountParams) (User, error) {
	var res User
	if err := s.ExecTx(ctx, func(tx *Store) error {
		/* for ghAccount we can just use github username */
		u, err := tx.CreateUser(ctx, CreateUserParams{
			Email:    arg.GhEmail,
			Username: arg.GhUsername,
		})
		if err != nil {
			return fmt.Errorf("Error creating user: %w", err)
		}
		_, err = tx.CreateGithubAccount(ctx, CreateGithubAccountParams{
			UserID:     u.ID,
			GhUserID:   arg.GhUserID,
			GhEmail:    arg.GhEmail,
			GhUsername: arg.GhUsername,
		})
		if err != nil {
			return fmt.Errorf("Error creating githubAccount: %w", err)
		}
		res = u
		return nil
	}); err != nil {
		return User{}, fmt.Errorf("error creating user with github account: %w", err)
	}
	return res, nil
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

func (s *Store) CreateInstallationTx(ctx context.Context, arg InstallationTxParams) error {
	return s.ExecTx(ctx, func(tx *Store) error {
		installation, err := tx.CreateInstallation(ctx, CreateInstallationParams{
			GhInstallationID: arg.InstallationID,
			UserID:           arg.UserID,
		})
		if err != nil {
			return err
		}
		for _, repositoryTxParams := range arg.RepositoriesTxParams {
			_, err := tx.CreateRepository(ctx, CreateRepositoryParams{
				InstallationID: installation.GhInstallationID,
				RepositoryID:   repositoryTxParams.RepositoryID,
				Name:           repositoryTxParams.Name,
				FullName:       repositoryTxParams.FullName,
				Url:            fmt.Sprintf("https://github.com/%s", repositoryTxParams.FullName), /* ghUrl not always in events */
				PathOnDisk: filepath.Join(
					arg.RepositoriesPath,
					repositoryTxParams.FullName,
				),
			})
			if err != nil {
				return err
			}
		}
		return nil
	})
}
