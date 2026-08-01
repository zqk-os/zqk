package internal

import (
	"testing"

	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/testkit"
	"github.com/lanceman/zqk/pkg/zqkenv"
)

// envForIsolatedCLIProject returns the environment for spawning zqk against tmpRoot.
// It uses [zqkenv.SubprocessEnvironWithTestRoot] so inherited ZQK_PROJECT_ROOT / ZQK_TEST_DATA_DIR
// cannot override isolation. Prefer [zqkenv.WireExecForIsolatedProject] when wiring *exec.Cmd.
func envForIsolatedCLIProject(tmpRoot string) []string {
	env := zqkenv.SubprocessEnvironWithTestRoot(tmpRoot)
	env = append(env, "ZQK_API_KEY=account:test-harness")
	return env
}

// getStorageProviderForTest returns a storage provider and registers cleanup
// to prevent HashRegistry worker goroutine leaks in tests.
// Use this instead of getStorageProvider() in tests.
func getStorageProviderForTest(t *testing.T, projectRoot string) (storage.ObjectStorageProvider, error) {
	t.Helper()
	provider, err := getStorageProvider(projectRoot)
	if err != nil {
		return nil, err
	}
	if fs, ok := provider.(*storage.FileObjectStorage); ok {
		testkit.RegisterTempProjectTeardown(t, projectRoot, fs)
	}
	return provider, nil
}
