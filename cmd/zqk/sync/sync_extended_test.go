package sync

import (
	"bytes"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newCapturingSyncCmd() (*cobra.Command, *bytes.Buffer) {
	cmd := NewSyncCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	return cmd, buf
}

func executeSyncTest(t *testing.T, args ...string) (string, error) {
	t.Helper()
	isolateSyncTest(t)
	cmd, buf := newCapturingSyncCmd()
	cmd.SetArgs(args)
	err := cmd.Execute()
	return buf.String(), err
}

func TestSyncCmd_GitHub_WithToken(t *testing.T) {
	out, err := executeSyncTest(t, "github", "--repo", "zqk-os/zqk", "--token", "ghp_fake123", "--direction", "pull")
	require.NoError(t, err)
	assert.Contains(t, out, "GitHub synchronization completed for zqk-os/zqk")
}

func TestSyncCmd_GitHub_EnvToken(t *testing.T) {
	t.Setenv(EnvGitHubToken, "ghp_env_token_456")
	out, err := executeSyncTest(t, "github", "--repo", "zqk-os/zqk", "--direction", "push")
	require.NoError(t, err)
	assert.Contains(t, out, "GitHub synchronization completed for zqk-os/zqk")
}

func TestSyncCmd_GitHub_MissingToken(t *testing.T) {
	t.Setenv(EnvGitHubToken, "")
	_, err := executeSyncTest(t, "github", "--repo", "zqk-os/zqk")
	require.Error(t, err)
	assert.Contains(t, err.Error(), MsgErrGitHubTokenRequired)
}

func TestSyncCmd_Linear_WithKey(t *testing.T) {
	out, err := executeSyncTest(t, "linear", "--team", "ENG", "--api-key", "lin_key_123", "--direction", "bidirectional")
	require.NoError(t, err)
	assert.Contains(t, out, "Linear synchronization completed for team ENG")
}

func TestSyncCmd_Linear_EnvKey(t *testing.T) {
	t.Setenv(EnvLinearAPIKey, "lin_key_env_456")
	out, err := executeSyncTest(t, "linear", "--team", "ENG")
	require.NoError(t, err)
	assert.Contains(t, out, "Linear synchronization completed for team ENG")
}

func TestSyncCmd_Linear_MissingKey(t *testing.T) {
	t.Setenv(EnvLinearAPIKey, "")
	_, err := executeSyncTest(t, "linear", "--team", "ENG")
	require.Error(t, err)
	assert.Contains(t, err.Error(), MsgErrLinearAPIKeyRequired)
}

func TestSyncCmd_Status_Empty(t *testing.T) {
	out, err := executeSyncTest(t, "status")
	require.NoError(t, err)
	assert.Contains(t, out, MsgSyncStatusHeader)
	assert.Contains(t, out, "GitHub Issues Synchronized: 0")
	assert.Contains(t, out, "Linear Tickets Synchronized: 0")
	assert.Contains(t, out, "Total Synced Work Units:    0")
}

func TestPrintSyncDryRun(t *testing.T) {
	cmd, buf := newCapturingSyncCmd()
	printSyncDryRun(cmd, MsgDryRunGitHubFmt, "my-org/my-repo", "pull")
	out := buf.String()
	assert.Contains(t, out, "[DRY-RUN] GitHub Issue synchronization simulation for repository: my-org/my-repo")
	assert.Contains(t, out, "Direction: pull")
	assert.Contains(t, out, "Verified connection and payload schema compatibility")
}
