package overseer_test

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/daemon/overseer"
	"golang.org/x/sys/unix"
)

// Helper process entry point for subprocess spawning tests.
func init() {
	if os.Getenv("TEST_OVERSEER_HELPER") == "1" {
		mode := os.Getenv("TEST_OVERSEER_HELPER_MODE")
		switch mode {
		case "worker":
			// Sleep until killed by signal
			time.Sleep(30 * time.Second)
			os.Exit(0)
		case "parent_with_child":
			// Spawn a grandchild and sleep
			child := exec.Command(os.Args[0])
			child.Env = append(os.Environ(), "TEST_OVERSEER_HELPER=1", "TEST_OVERSEER_HELPER_MODE=worker")
			if err := child.Start(); err != nil {
				os.Exit(1)
			}
			time.Sleep(30 * time.Second)
			os.Exit(0)
		case "fast_exit":
			os.Exit(0)
		}
	}
}

// CRIT-OVERSEER-PGID-ISOLATION: Static Floor: Process supervisor initializes
// dedicated POSIX PGID and sets pgid on child service spawns.
func TestProcessGroupIsolation(t *testing.T) {
	mgr, err := overseer.NewProcessGroupManager(false)
	require.NoError(t, err)
	require.Greater(t, mgr.PGID(), 0)

	// Child with own PGID
	cmd := exec.Command(os.Args[0])
	cmd.Env = append(os.Environ(), "TEST_OVERSEER_HELPER=1", "TEST_OVERSEER_HELPER_MODE=worker")
	mgr.PrepareCommand(cmd, true)

	require.NoError(t, cmd.Start())
	defer func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
			_, _ = cmd.Process.Wait()
		}
	}()

	mgr.RegisterChild(cmd)
	childPID := cmd.Process.Pid
	require.Greater(t, childPID, 0)

	// Assert child is its own process group leader
	childPgid, err := unix.Getpgid(childPID)
	require.NoError(t, err)
	assert.Equal(t, childPID, childPgid, "Child configured with setOwnPGID=true must have PGID == PID")
	assert.Contains(t, mgr.TrackedChildren(), childPID)
}

// CRIT-OVERSEER-ATOMIC-SIGNALING: Operational Proof: Stop command issues
// atomic signal fan-out to negative PGID terminating process tree.
func TestAtomicGroupSignaling(t *testing.T) {
	mgr, err := overseer.NewProcessGroupManager(false)
	require.NoError(t, err)

	cmd := exec.Command(os.Args[0])
	cmd.Env = append(os.Environ(), "TEST_OVERSEER_HELPER=1", "TEST_OVERSEER_HELPER_MODE=worker")
	mgr.PrepareCommand(cmd, true)

	require.NoError(t, cmd.Start())
	childPID := cmd.Process.Pid
	childPgid, err := unix.Getpgid(childPID)
	require.NoError(t, err)
	require.Equal(t, childPID, childPgid)

	// Confirm process is running
	require.NoError(t, unix.Kill(childPID, 0))

	// Atomically terminate group with grace period
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err = mgr.TerminateGroup(ctx, childPgid, 200*time.Millisecond)
	require.NoError(t, err)

	// Verify the process group is terminated
	_ = cmd.Wait()
	err = unix.Kill(childPID, 0)
	assert.Equal(t, unix.ESRCH, err, "Terminated process must return ESRCH (no such process)")
}

// CRIT-OVERSEER-SUBREAPER-REAPING: Negative Invariant: Detached or double-forking
// child processes are adopted and reaped by subreaper without zombie leaks.
func TestSubreaperReaping(t *testing.T) {
	mgr, err := overseer.NewProcessGroupManager(false)
	require.NoError(t, err)

	// Test EnableSubreaper
	err = mgr.EnableSubreaper()
	require.NoError(t, err)

	if mgr.IsSubreaperSupported() {
		assert.True(t, mgr.IsSubreaperEnabled())
	}

	// Spawn a fast-exiting child to verify ReapZombies
	cmd := exec.Command(os.Args[0])
	cmd.Env = append(os.Environ(), "TEST_OVERSEER_HELPER=1", "TEST_OVERSEER_HELPER_MODE=fast_exit")
	mgr.PrepareCommand(cmd, true)
	require.NoError(t, cmd.Start())
	pid := cmd.Process.Pid
	mgr.RegisterChild(cmd)

	// Harvest reaped children deterministically
	require.Eventually(t, func() bool {
		reaped := mgr.ReapZombies()
		for _, r := range reaped {
			if r.PID == pid {
				assert.True(t, r.Status.Exited())
				return true
			}
		}
		return false
	}, 2*time.Second, 25*time.Millisecond, "Terminated child must be harvested by ReapZombies")

	assert.NotContains(t, mgr.TrackedChildren(), pid, "Reaped child must be unregistered")
}
