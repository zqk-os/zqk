package system

import (
	"os"
	"path/filepath"
	"testing"

	caspkg "github.com/zqk-os/zqk/pkg/storage/cas"
	"github.com/zqk-os/zqk/pkg/zqkenv"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestMain(m *testing.M) {
	// Isolate CAS index write queues per project root to prevent contention
	// and livelocks in parallel tests.
	caspkg.SetListingIndexWriteQueueFactoryToPerProjectRoot()

	// Set ZQK_TEST_ROOT globally to indicate we are running inside tests,
	// which increases the stuck watchdog timeout to 180s and avoids false-positive stuck exits.
	// Find actual project root to avoid resolving paths relative to "true".
	root := ""
	if dir, err := fileutil.Getwd(); err == nil {
		for {
			if _, err := fileutil.Stat(filepath.Join(dir, paths.ProjectDataDir)); err == nil {
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
	zqkenv.OSEnvSetter(zqkenv.TestRoot().Name(), root)

	os.Exit(m.Run())
}
