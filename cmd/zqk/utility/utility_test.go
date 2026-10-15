package utility

import (
	"context"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/storage"
)

func initTestCmdContext(t *testing.T, cmd *cobra.Command, root string, store storage.ObjectStorageProvider) {
	t.Helper()
	stdCtx := cmd.Context()
	if stdCtx == nil {
		stdCtx = context.Background()
	}
	if store != nil {
		stdCtx = cli.WithStorageProvider(stdCtx, store)
	}
	cmd.SetContext(stdCtx)
	initCtx := pkgctx.NewCliInitializationContext(func(s string) string { return root }, root)
	cliCtx, err := cli.GetContextFromCommand(cmd, initCtx)
	require.NoError(t, err)
	cli.SetContext(cmd, cliCtx)
}

func TestNewUtilityCmd(t *testing.T) {
	cmd := NewUtilityCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "utility", cmd.Use)

	subcommands := make(map[string]bool)
	for _, c := range cmd.Commands() {
		subcommands[c.Name()] = true
	}

	assert.True(t, subcommands["version"], "should have version subcommand")
	assert.True(t, subcommands["migrate"], "should have migrate subcommand")
	assert.True(t, subcommands["validate-yaml"], "should have validate-yaml subcommand")
	assert.True(t, subcommands["fix-hashes"], "should have fix-hashes subcommand")
	assert.True(t, subcommands["fix-registration"], "should have fix-registration subcommand")
	assert.True(t, subcommands["scenario-builder"], "should have scenario-builder subcommand")
}
