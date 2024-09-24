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
	Name           string
	FullName       string
	Url            string
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
				Name:           blog.Name,
				FullName:       blog.FullName,
				Url:            blog.Url,
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
