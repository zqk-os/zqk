package system

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/utils/syscallutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestPrecommitTimeoutEnv_Parsing(t *testing.T) {
	t.Setenv("ZQK_PRECOMMIT_TIMEOUT", "180s")
	d, ok := zqkenv.ParsePrecommitTimeout()
	require.True(t, ok)
	assert.Equal(t, 180*time.Second, d)

	t.Setenv("ZQK_PRECOMMIT_TIMEOUT", "240")
	d2, ok2 := zqkenv.ParsePrecommitTimeout()
	require.True(t, ok2)
	assert.Equal(t, 240*time.Second, d2)
}

func TestPrecommitTimeoutEnv_Calculation(t *testing.T) {
	t.Setenv("ZQK_PRECOMMIT_TIMEOUT", "150s")
	cmd := &cobra.Command{Use: "check"}
	cmd.Flags().String("timeout", "", "")
	cmd.Flags().Int("tier", 1, "")

	timeout := CalculateCheckProgressiveTimeout(cmd, []string{})
	assert.Equal(t, 150*time.Second, timeout)
}

func TestStaleLock_DeadPIDDetection(t *testing.T) {
	assert.True(t, syscallutil.IsProcessAlive(os.Getpid()))
	assert.False(t, syscallutil.IsProcessAlive(999999))
}

func assertLockCanBeAcquiredAndReleased(t *testing.T, lockPath string) {
	t.Helper()
	fl, err := storagepkg.NewFileLock(lockPath)
	require.NoError(t, err)
	defer func() {
		closeErr := fl.Close()
		require.NoError(t, closeErr)
	}()

	acquired, lockErr := fl.TryLock()
	require.NoError(t, lockErr)
	assert.True(t, acquired)

	unlockErr := fl.Unlock()
	require.NoError(t, unlockErr)
}

func TestStaleLock_DeadPIDEviction(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "dead_pid.lock")
	meta := &storagepkg.LockMetadata{
		PID:       999999,
		CreatedAt: time.Now(),
	}
	err := storagepkg.WriteLockMetadata(lockPath, meta)
	require.NoError(t, err)

	assertLockCanBeAcquiredAndReleased(t, lockPath)
}

func TestStaleLock_TTLEviction(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "expired_ttl.lock")
	meta := &storagepkg.LockMetadata{
		PID:       os.Getpid(),
		CreatedAt: time.Now().Add(-10 * time.Minute),
	}
	err := storagepkg.WriteLockMetadata(lockPath, meta)
	require.NoError(t, err)

	expiredTime := time.Now().Add(-10 * time.Minute)
	err = fileutil.Chtimes(lockPath, expiredTime, expiredTime)
	require.NoError(t, err)

	assertLockCanBeAcquiredAndReleased(t, lockPath)
}
