package system

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewTestDriveCmd_Structure(t *testing.T) {
	t.Parallel()

	cmd := NewTestDriveCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "test-drive", cmd.Use)
	assert.Contains(t, cmd.Aliases, "td")
	assert.Contains(t, cmd.Aliases, "testdrive")
	assert.NotNil(t, cmd.Flags().Lookup("intent"))
	assert.NotNil(t, cmd.Flags().Lookup("scenario"))
	assert.NotNil(t, cmd.Flags().Lookup("speed"))
	assert.NotNil(t, cmd.Flags().Lookup("non-interactive"))
	assert.NotNil(t, cmd.Flags().Lookup("dry-run"))
}

func TestRunTestDrive_TableDriven(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		args        []string
		input       string
		expectError bool
		expectOut   []string
	}{
		{
			name: "Non-interactive default run",
			args: []string{"--non-interactive", "--speed", "instant"},
			expectOut: []string{
				"ZQK KNOWLEDGE KERNEL — DEVELOPER SWARM TEST-DRIVE",
				"[Stage 1/4] Seed Planting",
				"[Stage 2/4] Intent Crystallization",
				"[Stage 3/4] Swarm Execution",
				"[Stage 4/4] Cryptographic Verification Receipt",
				"TEST-DRIVE COMPLETED SUCCESSFULLY",
			},
		},
		{
			name: "Non-interactive custom intent",
			args: []string{
				"--non-interactive",
				"--speed", "instant",
				"--intent", "Build a high-performance vector cache with cosine similarity",
			},
			expectOut: []string{
				"Build a high-performance vector cache with cosine similarity",
				"TEST-DRIVE COMPLETED SUCCESSFULLY",
			},
		},
		{
			name: "Non-interactive CLI scenario",
			args: []string{
				"--non-interactive",
				"--speed", "instant",
				"--scenario", "cli-tool",
			},
			expectOut: []string{
				"Build a high-performance CLI utility",
				"TEST-DRIVE COMPLETED SUCCESSFULLY",
			},
		},
		{
			name: "Interactive stdin input",
			args: []string{
				"--speed", "instant",
			},
			input: "Build a resilient distributed lock with ETCD\n",
			expectOut: []string{
				"Build a resilient distributed lock with ETCD",
				"TEST-DRIVE COMPLETED SUCCESSFULLY",
			},
		},
		{
			name: "Intent too short fails validation",
			args: []string{
				"--non-interactive",
				"--speed", "instant",
				"--intent", "hi",
			},
			expectError: true,
		},
		{
			name: "Intent exceeds max length fails validation",
			args: []string{
				"--non-interactive",
				"--speed", "instant",
				"--intent", strings.Repeat("A", 600),
			},
			expectError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cmd := NewTestDriveCmd()
			var outBuf bytes.Buffer
			cmd.SetOut(&outBuf)
			cmd.SetErr(&outBuf)
			if tc.input != "" {
				cmd.SetIn(bytes.NewBufferString(tc.input))
			} else {
				cmd.SetIn(bytes.NewBufferString("\n"))
			}
			cmd.SetArgs(tc.args)

			err := cmd.Execute()
			if tc.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				out := outBuf.String()
				for _, expected := range tc.expectOut {
					assert.Contains(t, out, expected)
				}
			}
		})
	}
}
