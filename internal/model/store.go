package model

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"

	"github.com/xr0-org/progstack/internal/util"
)

type Store struct {
	*Queries
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{
		db:      db,
		Queries: New(db),
	}
}

func (s *Store) execTx(ctx context.Context, fn func(*Queries) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	q := New(tx)
	if err = fn(q); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			return fmt.Errorf("tx err: %w, rb err: %w", err, rbErr)
		}
		return err
	}
	return tx.Commit()
}

func (s *Store) CreateUserWithGithubAccountTx(ctx context.Context, arg CreateGithubAccountParams) error {
	return s.execTx(ctx, func(q *Queries) error {
		/* for ghAccount we can just use github username */
		user, err := s.CreateUser(ctx, CreateUserParams{
			Email:    arg.GhEmail,
			Username: arg.GhUsername,
		})
		if err != nil {
			return err
		}
		_, err = s.CreateGithubAccount(ctx, CreateGithubAccountParams{
			UserID:     user.ID,
			GhUserID:   arg.GhUserID,
			GhEmail:    arg.GhEmail,
			GhUsername: arg.GhUsername,
		})
		return err
	})
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
}

func (s *Store) CreateInstallationTx(ctx context.Context, arg InstallationTxParams) error {
	return s.execTx(ctx, func(q *Queries) error {
		installation, err := s.CreateInstallation(ctx, CreateInstallationParams{
			GhInstallationID: arg.InstallationID,
			UserID:           arg.UserID,
		})
		if err != nil {
			return err
		}
		for _, repositoryTxParams := range arg.RepositoriesTxParams {
			_, err := s.CreateRepository(ctx, CreateRepositoryParams{
				InstallationID: installation.GhInstallationID,
				RepositoryID:   repositoryTxParams.RepositoryID,
				Name:           repositoryTxParams.Name,
				FullName:       repositoryTxParams.FullName,
				Url:            fmt.Sprintf("https://github.com/%s", repositoryTxParams.FullName), /* ghUrl not always in events */
			})
			if err != nil {
				return err
			}
		}
		return nil
	})
}

type CreateSubscriberTxParams struct {
	BlogID int32
	Email  string
}

func (s *Store) CreateSubscriberTx(ctx context.Context, arg CreateSubscriberTxParams) error {
	return s.execTx(ctx, func(q *Queries) error {
		sub, err := s.GetSubscriberForBlog(ctx, GetSubscriberForBlogParams{
			BlogID: arg.BlogID,
			Email:  arg.Email,
		})
		if err != nil {
			if err != sql.ErrNoRows {
				return fmt.Errorf("error getting subscriber for blog: %w", err)
			}
			/* no subscription exists */
		}
		/* check if subscription already exists */
		if sub.Email == arg.Email {
			return fmt.Errorf("subscription already exists")
		}
		_, err = s.CreateSubscriber(ctx, CreateSubscriberParams{
			BlogID: arg.BlogID,
			Email:  arg.Email,
		})
		if err != nil {
			return fmt.Errorf("error writing subscriber for blog to db: %w", err)
		}
		return nil
	})
}

type UpdateSubdomainTxParams struct {
	BlogID    int32
	Subdomain string
}

func (s *Store) UpdateSubdomainTx(ctx context.Context, arg UpdateSubdomainTxParams) error {
	return s.execTx(ctx, func(q *Queries) error {
		exists, err := s.SubdomainExists(ctx, arg.Subdomain)
		if err != nil {
			return fmt.Errorf("error checking if subdomain exists")
		}
		if exists {
			return util.CreateCustomError(
				"subdomain already exists",
				http.StatusBadRequest,
			)
		}

		/* write new subdomain */
		err = s.UpdateSubdomainByID(ctx, UpdateSubdomainByIDParams{
			ID:        arg.BlogID,
			Subdomain: arg.Subdomain,
		})
		if err != nil {
			return fmt.Errorf("error creating subdomain `%s' for blog `%d': %w", arg.Subdomain, arg.BlogID, err)
		}
		return nil
	})
}
