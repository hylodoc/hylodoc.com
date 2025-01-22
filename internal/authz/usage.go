package authz

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/hylodoc/hylodoc.com/internal/authz/internal/size"
	"github.com/hylodoc/hylodoc.com/internal/model"
)

func UserStorageUsed(s *model.Store, userID string) (size.Size, error) {
	paths, err := s.ListRepositoriesGitdirPathsByUserID(
		context.TODO(), userID,
	)
	if err != nil {
		return 0, fmt.Errorf("paths: %w", err)
	}
	/* loop over repos */
	var totalBytes int64
	for _, path := range paths {
		if _, err := os.Stat(path); err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return 0, fmt.Errorf("stat: %w", err)
		}
		bytes, err := dirBytes(path)
		if err != nil {
			return 0, fmt.Errorf("path: %w", err)
		}
		totalBytes += bytes
	}
	return size.Size(totalBytes), nil
}

/* calculate the disk usage of a single folder */
func dirBytes(repopath string) (int64, error) {
	var usage int64

	if err := filepath.Walk(repopath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			usage += info.Size()
		}
		return nil
	}); err != nil {
		return 0, err
	}
	return usage, nil
}
