package authz

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/xr0-org/progstack/internal/model"
)

func UserStorageUsed(s *model.Store, userID int32) (int64, error) {
	paths, err := listUserDiskPaths(userID, s)
	if err != nil {
		return -1, fmt.Errorf("paths: %w", err)
	}
	/* loop over repos */
	var totalBytes int64
	for _, path := range paths {
		bytes, err := dirBytes(path)
		if err != nil {
			return -1, fmt.Errorf("path `%s': %w", path, err)
		}
		totalBytes += bytes
	}
	return totalBytes, nil
}

func listUserDiskPaths(userID int32, s *model.Store) ([]string, error) {
	folders, err := s.ListBlogFolderPathsByUserID(context.TODO(), userID)
	if err != nil {
		return nil, fmt.Errorf("folder: %w", err)
	}
	repos, err := s.ListRepositoryPathsOnDiskByUserID(context.TODO(), userID)
	if err != nil {
		return nil, fmt.Errorf("repo: %w", err)
	}
	return append(folders, repos...), nil
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
