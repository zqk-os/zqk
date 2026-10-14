package explain

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zqk-os/zqk/pkg/acronyms"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/testkit"
)

func TestExplainCmd_SyncAndJSONFormats(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{SeedSchemaPlane: true})

	// 1. Test --sync flag with valid storage provider
	cmdSync := NewExplainCmd()
	var syncBuf bytes.Buffer
	baseCtx := pkgctx.WithCommandOutputWriter(context.Background(), &syncBuf)
	baseCtx = pkgctx.WithSecurityContext(baseCtx, pkgctx.NewSystemSecurityContext())
	cmdSync.SetContext(cli.WithStorageProvider(baseCtx, proj.FileStorage))
	initCtx := pkgctx.NewCliInitializationContext(cli.ResolveProjectRoot, ".")
	cctx, err := cli.GetContextFromCommand(cmdSync, initCtx)
	require.NoError(t, err)
	cctx.ProjectRoot = proj.Root
	cli.SetContext(cmdSync, cctx)
	cmdSync.SetArgs([]string{"--sync"})

	err = cmdSync.Execute()
	require.NoError(t, err)
	assert.Contains(t, syncBuf.String(), "Successfully synchronized")
	assert.Contains(t, syncBuf.String(), acronyms.KernelAcronymsSchemeID)

	// 2. Test list all in JSON format
	cmdListJSON := NewExplainCmd()
	var listJSONBuf bytes.Buffer
	baseCtx = pkgctx.WithCommandOutputWriter(context.Background(), &listJSONBuf)
	cmdListJSON.SetContext(cli.WithStorageProvider(baseCtx, proj.FileStorage))
	cmdListJSON.SetArgs([]string{"-f", "json"})

	err = cmdListJSON.Execute()
	require.NoError(t, err)
	assert.Contains(t, listJSONBuf.String(), `"code": "BLI"`)
	assert.Contains(t, listJSONBuf.String(), `"full_name": "Backlog Item"`)

	// 3. Test single item in JSON format
	cmdItemJSON := NewExplainCmd()
	var itemJSONBuf bytes.Buffer
	baseCtx = pkgctx.WithCommandOutputWriter(context.Background(), &itemJSONBuf)
	cmdItemJSON.SetContext(cli.WithStorageProvider(baseCtx, proj.FileStorage))
	cmdItemJSON.SetArgs([]string{"BLI", "-f", "json"})

	err = cmdItemJSON.Execute()
	require.NoError(t, err)
	assert.Contains(t, itemJSONBuf.String(), `"code": "BLI"`)
	assert.Contains(t, itemJSONBuf.String(), `"full_name": "Backlog Item"`)

	// 4. Test single item with related refs in standard format
	cmdItemRel := NewExplainCmd()
	var itemRelBuf bytes.Buffer
	baseCtx = pkgctx.WithCommandOutputWriter(context.Background(), &itemRelBuf)
	cmdItemRel.SetContext(cli.WithStorageProvider(baseCtx, proj.FileStorage))
	cmdItemRel.SetArgs([]string{"BLI"})

	err = cmdItemRel.Execute()
	require.NoError(t, err)
	out := itemRelBuf.String()
	assert.Contains(t, out, "ACRONYM    : BLI")
	assert.Contains(t, out, "FULL NAME  : Backlog Item")
	assert.Contains(t, out, "CATEGORY   : work")
	assert.Contains(t, out, "DEFINITION :")
	assert.Contains(t, out, "CONTEXT    :")
}
