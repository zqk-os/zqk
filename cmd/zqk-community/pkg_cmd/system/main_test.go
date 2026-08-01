package system

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/paths"

	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/zqkenv"
)

func TestMain(m *testing.M) {
	// Isolate CAS index write queues per project root to prevent contention
	// and livelocks in parallel tests.
	storage.SetListingIndexWriteQueueFactoryToPerProjectRoot()

	// Set ZQK_TEST_ROOT globally to indicate we are running inside tests,
	// which increases the stuck watchdog timeout to 180s and avoids false-positive stuck exits.
	// Find actual project root to avoid resolving paths relative to "true".
	root := ""
	if dir, err := os.Getwd(); err == nil {
		for {
			if _, err := os.Stat(filepath.Join(dir, paths.ProjectDataDir)); err == nil {
				root = dir
				break
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}
	if root == "" {
		root = "true" // fallback
	}
	_ = os.Setenv(zqkenv.TestRoot(), root)

	os.Exit(m.Run())
}
