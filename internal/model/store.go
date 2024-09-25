package model

import (
	"context"
	"database/sql"
	"fmt"
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

type BlogTxParams struct {
	GhRepositoryID int64
	GhName         string
	GhFullName     string
	GhUrl          string
	Subdomain      string
	FromAddress    string
}

type InstallationTxParams struct {
	InstallationID int64
	UserID         int32
	Blogs          []BlogTxParams
}

func (s *Store) CreateInstallationTx(ctx context.Context, arg InstallationTxParams) error {
	err := s.execTx(ctx, func(q *Queries) error {
		createInstallationArgs := CreateInstallationParams{
			GhInstallationID: arg.InstallationID,
			UserID:           arg.UserID,
		}
		installation, err := s.CreateInstallation(ctx, createInstallationArgs)
		if err != nil {
			return err
		}
		for _, blog := range arg.Blogs {
			createBlogArgs := CreateBlogParams{
				InstallationID: installation.ID,
				GhRepositoryID: blog.GhRepositoryID,
				GhName:         blog.GhName,
				GhFullName:     blog.GhFullName,
				GhUrl:          blog.GhUrl,
				Subdomain:      blog.Subdomain,
				FromAddress:    blog.FromAddress,
			}
			_, err := s.CreateBlog(ctx, createBlogArgs)
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	return nil
}

type CreateSubscriberTxParams struct {
	BlogID           int32
	Email            string
	UnsubscribeToken string
}

func (s *Store) CreateSubscriberTx(ctx context.Context, arg CreateSubscriberTxParams) error {
	err := s.execTx(ctx, func(q *Queries) error {
		sub, err := s.GetSubscriberForBlog(ctx, GetSubscriberForBlogParams{
			BlogID: arg.BlogID,
			Email:  arg.Email,
		})
		if err != nil {
			if err != sql.ErrNoRows {
				return fmt.Errorf("error getting subscriber for blog")
			}
			/* no subscription exists */
		}
		/* check if subscription already exists */
		if sub.Email == arg.Email {
			return fmt.Errorf("subscription already exists")
		}
		_, err = s.CreateSubscriberForBlog(ctx, CreateSubscriberForBlogParams{
			BlogID:           arg.BlogID,
			Email:            arg.Email,
			UnsubscribeToken: arg.UnsubscribeToken,
		})
		if err != nil {
			return fmt.Errorf("error writing subscriber for blog to db: %w", err)
		}
		return nil
	})
	if err != nil {
		return err
	}
	return nil
}
