package managedcmd

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManagedCommand_LifecycleAndCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd := ManagedCommand(t, ctx, "sleep", "10")
	require.NotNil(t, cmd)
	assert.NotNil(t, cmd.Cancel)
	assert.Equal(t, 2*time.Second, cmd.WaitDelay)

	err := cmd.Start()
	require.NoError(t, err)

	// Cancel context triggers cmd.Cancel
	cancel()

	err = cmd.Wait()
	assert.Error(t, err)
}

func TestKillManagedProcessGroup(t *testing.T) {
	// Test nil cmd
	assert.NoError(t, KillManagedProcessGroup(nil))

	// Test cmd without process
	dummyCmd := ManagedCommand(t, context.Background(), "true")
	assert.NoError(t, KillManagedProcessGroup(dummyCmd))

	// Test running cmd
	ctx := context.Background()
	cmd := ManagedCommand(t, ctx, "sleep", "10")
	err := cmd.Start()
	require.NoError(t, err)

	err = KillManagedProcessGroup(cmd)
	assert.NoError(t, err)
	_ = cmd.Wait()
}

func TestWireCLISubprocessForIsolatedProject(t *testing.T) {
	tmpDir := t.TempDir()
	cmd := ManagedCommand(t, context.Background(), "true")
	WireCLISubprocessForIsolatedProject(cmd, tmpDir)
	assert.Equal(t, tmpDir, cmd.Dir)
	assert.NotEmpty(t, cmd.Env)
}
