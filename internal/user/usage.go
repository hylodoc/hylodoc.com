package user

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/xr0-org/progstack/internal/model"
)

func userBytes(s *model.Store, userID int32) (int64, error) {
	paths, err := s.ListBlogRepoPathsByUserID(context.TODO(), userID)
	if err != nil {
		if err != sql.ErrNoRows {
			return 0, err
		}
		return 0, nil
	}
	/* loop over repos */
	var totalBytes int64
	for _, path := range paths {
		bytes, err := dirBytes(path)
		if err != nil {
			return 0, fmt.Errorf("error calculating usage for user `%d' path `%s': %w", userID, path, err)
		}
		log.Printf("path `%s' used `%d' bytes\n", path, bytes)
		totalBytes += bytes
	}
	log.Printf("user `%d' total usage is `%d' bytes\n", totalBytes, userID)
	return totalBytes, nil
}

/* calculate the disk usage of a single folder */
func dirBytes(repopath string) (int64, error) {
	var usage int64

	if err := filepath.Walk(repopath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			log.Printf("err: %v\n", err)
			return err
		}
		if !info.IsDir() {
			usage += info.Size()
		}
		return nil
	}); err != nil {
		log.Printf("error: %v\n", err)
		return 0, err
	}
	return usage, nil
}
