package user

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
)

func TestCalculateDiskUsage(t *testing.T) {
	/* create a temp dir */

	tmpDir, err := ioutil.TempDir("", "test-repo")
	if err != nil {
		t.Fatalf("failed to create tmpDir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	/* create files with known sizes */
	testfiles := map[string]int64{
		"f1.txt": 100,
		"f2.txt": 200,
	}

	for name, size := range testfiles {
		path := filepath.Join(tmpDir, name)
		if err := ioutil.WriteFile(path, make([]byte, size), 0644); err != nil {
			t.Fatalf("failed to create test file: %v", err)
		}
	}

	/* calculate disk size */
	actualSize, err := dirBytes(tmpDir)
	if err != nil {
		t.Fatalf("error calculating disk usage: %v", err)
	}

	/* calculate expected size */
	expectedSize := int64(0)
	for _, size := range testfiles {
		expectedSize += size
	}

	if actualSize != expectedSize {
		t.Errorf("expected total size %d, got %d", expectedSize, actualSize)
	}
}
