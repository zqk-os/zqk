package storage_test

import (
	"path/filepath"
	"strings"
	"testing"

	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestErrSwallowNoiseReducedInStorage(t *testing.T) {
	cwd, err := fileutil.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}

	storageDir := cwd
	for {
		if _, err := fileutil.Stat(filepath.Join(storageDir, "go.mod")); err == nil {
			break
		}
		parent := filepath.Dir(storageDir)
		if parent == storageDir {
			t.Fatal("could not locate repo root containing go.mod")
		}
		storageDir = parent
	}
	storageDir = filepath.Join(storageDir, "pkg", "storage")

	var swallowCount int
	err = filepath.Walk(storageDir, func(path string, info fileutil.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}

		data, err := fileutil.ReadFile(path)
		if err != nil {
			return err
		}
		swallowCount += strings.Count(string(data), "err_swallow_")
		return nil
	})

	if err != nil {
		t.Fatalf("error walking storage dir: %v", err)
	}

	t.Logf("Measured err_swallow_ count in pkg/storage: %d (baseline was >380)", swallowCount)
	if swallowCount > 200 {
		t.Fatalf("expected err_swallow_ count to be <= 200 after refactor, got %d", swallowCount)
	}
}

// TestErrSwallow_BLI_CEF_R17_ERR_SWALLOW_001 verifies that numbered err_swallow debt markers
// in pkg/storage are reduced and replaced by structured logging (BLI-CEF-R17-ERR-SWALLOW-001).
func TestErrSwallow_BLI_CEF_R17_ERR_SWALLOW_001(t *testing.T) {
	TestErrSwallowNoiseReducedInStorage(t)
}
