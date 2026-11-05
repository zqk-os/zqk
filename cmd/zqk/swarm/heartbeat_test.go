package swarm

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
)

func TestSwarmHeartbeat_Hierarchy(t *testing.T) {
	t.Parallel()

	cmd := NewSwarmCmd()
	require.NotNil(t, cmd)

	subcommands := cmd.Commands()
	subNames := make(map[string]bool)
	for _, sub := range subcommands {
		subNames[sub.Name()] = true
	}

	assert.True(t, subNames["heartbeat"], "swarm command must have heartbeat subcommand")
}

func TestSwarmHeartbeat_OnceExecution(t *testing.T) {
	setupTestProjectRoot(t)

	cmd := NewHeartbeatCmd()
	cmd.SetArgs([]string{"--once"})
	out := bytes.NewBuffer(nil)
	cmd.SetContext(pkgctx.WithCommandOutputWriter(t.Context(), out))
	cmd.SetOut(out)
	cmd.SetErr(out)

	err := cmd.Execute()
	require.NoError(t, err)

	got := out.String()
	assert.Contains(t, got, "SWARM HEARTBEAT")
	assert.Contains(t, got, "Hygiene Sweep")
	assert.Contains(t, got, "Alignment Pulse")
	assert.Contains(t, got, "Reactive Wakeup")
}

func TestSwarmHeartbeat_JSONFormat(t *testing.T) {
	setupTestProjectRoot(t)

	cmd := NewHeartbeatCmd()
	cmd.SetArgs([]string{"--once", "-f", "json"})
	out := bytes.NewBuffer(nil)
	cmd.SetContext(pkgctx.WithCommandOutputWriter(t.Context(), out))
	cmd.SetOut(out)
	cmd.SetErr(out)

	err := cmd.Execute()
	require.NoError(t, err)

	var payload map[string]any
	err = json.Unmarshal(out.Bytes(), &payload)
	require.NoError(t, err, "output should be valid JSON: %s", out.String())

	assert.Contains(t, payload, "hygiene")
	assert.Contains(t, payload, "alignment")
	assert.Contains(t, payload, "wakeup")
}
