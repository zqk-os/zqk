package agent

import (
	"bytes"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/zqk-os/zqk/pkg/agentidle"
	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestScoreboardCmd(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv(zqkenv.TestRoot().Name(), tempDir)

	// cli.WithProcessor requires a valid project structure
	err := fileutil.MkdirAll(filepath.Join(tempDir, paths.ProcessDir), 0755)
	require.NoError(t, err)

	storePath := datacell.AgentIdleStorePath(tempDir)
	store, err := agentidle.NewFileStore(storePath)
	require.NoError(t, err)

	// Add some dummy data
	err = store.Accumulate("agent-1", "task-a", 15*time.Second)
	require.NoError(t, err)
	err = store.Accumulate("agent-2", "task-b", 5*time.Second)
	require.NoError(t, err)

	cmd := NewScoreboardCmd()
	cmd.SetArgs([]string{})

	b := bytes.NewBufferString("")
	cmd.SetOut(b)

	err = cmd.Execute()
	require.NoError(t, err)
}
