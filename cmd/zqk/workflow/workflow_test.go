package workflow

import (
	"context"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestNewWorkflowCmd(t *testing.T) {
	cmd := NewWorkflowCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "workflow", cmd.Use)

	names := make(map[string]bool)
	for _, c := range cmd.Commands() {
		names[c.Name()] = true
	}
	assert.True(t, names["next"])
	assert.True(t, names["whats-next"])
	assert.True(t, names["check"])
	assert.True(t, names["vds"])
	assert.True(t, names["add"])
	assert.True(t, names["coach"])
	assert.True(t, names["gen-trace-pipeline"])
	assert.True(t, names["link"])
	assert.True(t, names["do"])
}

func TestProcessorContexts(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	store, err := storage.NewFileObjectStorageForTest(proj.Root)
	require.NoError(t, err)
	testkit.RegisterStorageTestCleanup(t, proj.Root, store)

	cmd := &cobra.Command{}
	cmd.SetContext(cli.WithStorageProvider(context.Background(), store))

	initCtx := pkgctx.NewCliInitializationContext(func(s string) string { return proj.Root }, proj.Root)
	cliCtx, err := cli.GetContextFromCommand(cmd, initCtx)
	require.NoError(t, err)
	cli.SetContext(cmd, cliCtx)

	proc, err := cli.NewProcessor(cmd)
	require.NoError(t, err)

	opCtx, sp, sec := processorContexts(proc)
	assert.NotNil(t, opCtx)
	assert.NotNil(t, sp)
	assert.NotNil(t, sec)
}
