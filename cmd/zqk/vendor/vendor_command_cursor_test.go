package vendor

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
)

func TestVendorRootCommand_DiscoveryAndHierarchy(t *testing.T) {
	cmd := NewVendorCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "vendor", cmd.Use)
	assert.NotEmpty(t, cmd.Short)

	// Discover cursor subcommand directly
	cursorCmd, _, err := cmd.Find([]string{"cursor"})
	require.NoError(t, err)
	require.NotNil(t, cursorCmd)
	assert.Equal(t, "cursor", cursorCmd.Use)

	// Subcommand discovery across children
	var foundCursor bool
	for _, child := range cmd.Commands() {
		if child.Name() == "cursor" {
			foundCursor = true
			break
		}
	}
	assert.True(t, foundCursor, "expected cursor subcommand registered in NewVendorCmd commands")
}

func TestVendorCursorCommand_HierarchyAndSubcommands(t *testing.T) {
	cursorCmd := NewCursorCmd()
	require.NotNil(t, cursorCmd)
	assert.Equal(t, "cursor", cursorCmd.Use)
	assert.NotEmpty(t, cursorCmd.Short)

	// Subcommand: adapter
	adapterCmd, _, err := cursorCmd.Find([]string{"adapter"})
	require.NoError(t, err)
	require.NotNil(t, adapterCmd)
	assert.Equal(t, "adapter", adapterCmd.Use)

	// Subcommand: paste-applescript
	pasteCmd, _, err := cursorCmd.Find([]string{"paste-applescript"})
	require.NoError(t, err)
	require.NotNil(t, pasteCmd)
	assert.Equal(t, "paste-applescript", pasteCmd.Use)

	// Alias: paste-script
	aliasCmd, _, err := cursorCmd.Find([]string{"paste-script"})
	require.NoError(t, err)
	require.NotNil(t, aliasCmd)
	assert.Equal(t, "paste-applescript", aliasCmd.Use)
	assert.Equal(t, pasteCmd, aliasCmd)
}

func TestCursorAdapterCommand_PropertiesAndContract(t *testing.T) {
	cmd := NewCursorAdapterCmd()
	require.NotNil(t, cmd)

	assert.Equal(t, "adapter", cmd.Use)
	assert.Equal(t, "Cursor-specific MCP adapter (stdio ↔ daemon)", cmd.Short)
	assert.Equal(t, "Speaks a clean full MCP contract on stdio for Cursor while privately maintaining keepalive, reconnect, and event subscription against the TCP MCP daemon.", cmd.Long)
	assert.NotNil(t, cmd.RunE, "adapter command contract requires RunE execution function")

	// Verify TCP flag contract
	tcpFlag := cmd.Flags().Lookup("tcp")
	require.NotNil(t, tcpFlag, "adapter command must expose --tcp flag")
	assert.Equal(t, "127.0.0.1:8443", tcpFlag.DefValue)
	assert.Contains(t, tcpFlag.Usage, "TCP address")

	// Verify excluded common flags
	assert.Nil(t, cmd.Flags().Lookup("columns"), "columns flag must be excluded")
	assert.Nil(t, cmd.Flags().Lookup("ignore-scheduler-down"), "ignore-scheduler-down flag must be excluded")
	assert.Nil(t, cmd.Flags().Lookup("format"), "format flag must be excluded")
	assert.Nil(t, cmd.Flags().Lookup("output"), "output flag must be excluded")
}

func TestCursorPasteApplescriptCommand_PropertiesAndAliases(t *testing.T) {
	cmd := NewCursorPasteApplescriptCmd()
	require.NotNil(t, cmd)

	assert.Equal(t, "paste-applescript", cmd.Use)
	assert.Contains(t, cmd.Aliases, "paste-script")
	assert.Equal(t, "Print AppleScript for Cursor chat paste automation", cmd.Short)
	assert.Equal(t, "Prints the AppleScript used for Cursor IDE prompt paste automation. Respects ZQK_CURSOR_PASTE_PREFIX_STEPS.", cmd.Long)
	assert.NotNil(t, cmd.RunE, "paste-applescript command requires RunE execution function")
}

func TestCursorPasteApplescriptCommand_ExecutionCaptureStdout(t *testing.T) {
	t.Run("DefaultExecution", func(t *testing.T) {
		cmd := NewCursorPasteApplescriptCmd()
		var buf bytes.Buffer
		ctx := pkgctx.WithCommandOutputWriter(context.Background(), &buf)
		cmd.SetContext(ctx)
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)
		cmd.SetArgs([]string{})

		err := cmd.Execute()
		require.NoError(t, err)

		out := buf.String()
		assert.NotEmpty(t, out)
		assert.Contains(t, out, "Cursor", "generated AppleScript must target Cursor application")
	})

	t.Run("CustomPrefixStepsEnvironment", func(t *testing.T) {
		t.Setenv("ZQK_CURSOR_PASTE_PREFIX_STEPS", "cmd_shift_e,escape,option_cmd_e")
		cmd := NewCursorPasteApplescriptCmd()
		var buf bytes.Buffer
		ctx := pkgctx.WithCommandOutputWriter(context.Background(), &buf)
		cmd.SetContext(ctx)
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)
		cmd.SetArgs([]string{})

		err := cmd.Execute()
		require.NoError(t, err)

		out := buf.String()
		assert.NotEmpty(t, out)
		assert.Contains(t, out, "Cursor")
	})

	t.Run("InvalidPrefixStepsEnvironment", func(t *testing.T) {
		t.Setenv("ZQK_CURSOR_PASTE_PREFIX_STEPS", "invalid_token_sequence_xyz")
		cmd := NewCursorPasteApplescriptCmd()
		var buf bytes.Buffer
		ctx := pkgctx.WithCommandOutputWriter(context.Background(), &buf)
		cmd.SetContext(ctx)
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)
		cmd.SetArgs([]string{})

		err := cmd.Execute()
		require.Error(t, err, "invalid prefix step tokens must return an error")
	})

	t.Run("HierarchyDispatchPrimary", func(t *testing.T) {
		root := NewVendorCmd()
		var buf bytes.Buffer
		ctx := pkgctx.WithCommandOutputWriter(context.Background(), &buf)
		root.SetContext(ctx)
		root.SetOut(&buf)
		root.SetErr(&buf)
		root.SetArgs([]string{"cursor", "paste-applescript"})

		err := root.Execute()
		require.NoError(t, err)

		out := buf.String()
		assert.NotEmpty(t, out)
		assert.Contains(t, out, "Cursor")
	})

	t.Run("HierarchyDispatchAlias", func(t *testing.T) {
		root := NewVendorCmd()
		var buf bytes.Buffer
		ctx := pkgctx.WithCommandOutputWriter(context.Background(), &buf)
		root.SetContext(ctx)
		root.SetOut(&buf)
		root.SetErr(&buf)
		root.SetArgs([]string{"cursor", "paste-script"})

		err := root.Execute()
		require.NoError(t, err)

		out := buf.String()
		assert.NotEmpty(t, out)
		assert.Contains(t, out, "Cursor")
	})
}
