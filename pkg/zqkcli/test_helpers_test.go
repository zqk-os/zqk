package internal

import (
	"os/exec"
	"testing"

	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/testkit"
	"github.com/lanceman/zqk/pkg/zqkenv"
)

// harnessAccountID is the identity isolated child processes authenticate as. It is an ACC- id
// rather than a ZQK- one on purpose: the middleware treats a ZQK- token as a session id and
// looks it up in the store, which an isolated root does not contain.
const harnessAccountID = "ACC-TEST-HARNESS"

// envForIsolatedCLIProject returns the environment for spawning zqk against tmpRoot.
// It uses [zqkenv.SubprocessEnvironWithTestRoot] so inherited ZQK_PROJECT_ROOT / ZQK_TEST_DATA_DIR
// cannot override isolation. Prefer [wireIsolatedCLI] when wiring *exec.Cmd.
func envForIsolatedCLIProject(tmpRoot string) []string {
	env := zqkenv.SubprocessEnvironWithTestRoot(tmpRoot)
	env = append(env, zqkenv.APIKey()+"="+harnessAccountID)
	return env
}

// wireIsolatedCLI wires a child zqk process to tmpRoot and gives it a harness identity.
//
// [zqkenv.WireExecForIsolatedProject] alone is not enough: it isolates paths but passes HOME
// through, so a child with no explicit key falls back to $HOME/.zqk/credentials and
// authenticates as whichever developer is logged in. The suite then fails resolving that
// session id against the empty temp store, which makes the outcome depend on the developer's
// login state rather than on the code under test.
// TRACK: REDACTED — identity isolation belongs in zqkenv, but that
// function is also on 19 production spawn paths, so widening it needs its own pass.
func wireIsolatedCLI(cmd *exec.Cmd, tmpRoot string) {
	if cmd == nil {
		return
	}
	zqkenv.WireExecForIsolatedProject(cmd, tmpRoot)
	cmd.Env = append(cmd.Env, zqkenv.APIKey()+"="+harnessAccountID)
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
