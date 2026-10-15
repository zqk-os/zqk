package workflow

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/vds"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestVDSCmd_Structure(t *testing.T) {
	cmd := NewVDSCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "vds", cmd.Use)

	subcommands := cmd.Commands()
	names := make(map[string]bool)
	for _, sc := range subcommands {
		names[sc.Name()] = true
	}
	assert.True(t, names["checklist"], "expected checklist subcommand")
	assert.True(t, names["evaluate"], "expected evaluate subcommand")
	assert.True(t, names["project"], "expected project subcommand")
	assert.True(t, names["init"], "expected init subcommand")
}

func TestResolveVDSProjectRoot(t *testing.T) {
	// Case 1: cli context has ProjectRoot
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	cmd := &cobra.Command{}
	initCtx := pkgctx.NewCliInitializationContext(func(s string) string { return proj.Root }, proj.Root)
	cliCtx, err := cli.GetContextFromCommand(cmd, initCtx)
	require.NoError(t, err)
	cli.SetContext(cmd, cliCtx)

	root, err := resolveVDSProjectRoot(cmd)
	require.NoError(t, err)
	assert.Equal(t, proj.Root, root)

	// Case 2: empty context fallback
	emptyCmd := &cobra.Command{}
	rootFallback, err := resolveVDSProjectRoot(emptyCmd)
	// Fallback uses ResolveProjectRoot(".") or returns error
	if err == nil {
		assert.NotEmpty(t, rootFallback)
	}
}

func TestPersistVDSReport(t *testing.T) {
	tmpDir := t.TempDir()
	rep := &vds.Report{
		Verdict: "PASS",
		Tone:    "neutral",
	}

	err := persistVDSReport(tmpDir, rep)
	require.NoError(t, err)

	reportPath := filepath.Join(tmpDir, paths.ProjectDataDir, paths.StateDir, "vds", "last_evaluate.json")
	data, err := fileutil.ReadFile(reportPath)
	require.NoError(t, err)

	var loaded vds.Report
	err = json.Unmarshal(data, &loaded)
	require.NoError(t, err)
	assert.Equal(t, "PASS", loaded.Verdict)
	assert.Equal(t, "neutral", loaded.Tone)
}

func TestEmitVDS(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().String("format", "json", "")

	// 1. JSON format
	payload := map[string]any{"status": "ok"}
	err := emitVDS(cmd, nil, payload, "agent brief text")
	assert.NoError(t, err)

	// 2. FormatAgentPrompt with agent brief
	cmdAgent := &cobra.Command{}
	cmdAgent.Flags().String("format", string(cli.FormatAgentPrompt), "")
	err = emitVDS(cmdAgent, nil, payload, "agent brief output text")
	assert.NoError(t, err)

	// 3. FormatAgentPrompt with empty agent brief falls back to FormatOutput
	cmdEmpty := &cobra.Command{}
	cmdEmpty.Flags().String("format", string(cli.FormatAgentPrompt), "")
	err = emitVDS(cmdEmpty, nil, payload, "   ")
	assert.NoError(t, err)
}

type mockVDSStorage struct {
	storage.ObjectStorageProvider
	items []map[string]any
}

func (m *mockVDSStorage) List(ctx context.Context, sec *pkgctx.SecurityContext, stCtx *pkgctx.StorageContext, filter storage.ListFilter) (*storage.QueryResult, error) {
	var out []map[string]any
	for _, item := range m.items {
		if k, ok := item[objects.FieldKeyKind].(string); ok && k == filter.Kind {
			out = append(out, item)
		}
	}
	return &storage.QueryResult{Objects: out}, nil
}

func TestVDSTitleLookup(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	store := &mockVDSStorage{
		items: []map[string]any{
			{
				objects.FieldKeyID:          "GOAL-TITLE-001",
				objects.FieldKeyKind:        objects.KindGoal,
				objects.FieldKeyTitle:       "Alpha Goal",
				objects.FieldKeyStatus:      objects.ObjectStatusActive,
				objects.FieldKeyNamespaceID: "zqk:kernel",
			},
			{
				objects.FieldKeyID:          "GOAL-TITLE-002",
				objects.FieldKeyKind:        objects.KindGoal,
				objects.FieldKeyTitle:       "Beta Goal",
				objects.FieldKeyStatus:      objects.ObjectStatusActive,
				objects.FieldKeyNamespaceID: "zqk:kernel",
			},
		},
	}

	ctx := pkgctx.NewSystemContext()

	// Setup processor with storage
	cmd := &cobra.Command{}
	cmdStandardCtx := cli.WithStorageProvider(context.Background(), store)
	cmd.SetContext(cmdStandardCtx)

	initCtx := pkgctx.NewCliInitializationContext(func(s string) string { return proj.Root }, proj.Root)
	cliCtx, err := cli.GetContextFromCommand(cmd, initCtx)
	require.NoError(t, err)
	cli.SetContext(cmd, cliCtx)

	proc, err := cli.NewProcessor(cmd)
	require.NoError(t, err)
	lookup := vdsTitleLookup(proc)
	require.NotNil(t, lookup)

	// Exact match
	id, err := lookup(ctx, objects.KindGoal, "Alpha Goal")
	require.NoError(t, err)
	assert.Equal(t, "GOAL-TITLE-001", id)

	// Second match
	id, err = lookup(ctx, objects.KindGoal, "Beta Goal")
	require.NoError(t, err)
	assert.Equal(t, "GOAL-TITLE-002", id)

	// Non-existent title (returns first object as fallback when not matched)
	id, err = lookup(ctx, objects.KindGoal, "NonExistent Goal")
	require.NoError(t, err)
	assert.Equal(t, "GOAL-TITLE-001", id)

	// Empty result
	emptyStore := &mockVDSStorage{}
	cmdEmpty := &cobra.Command{}
	cmdEmpty.SetContext(cli.WithStorageProvider(context.Background(), emptyStore))
	cliCtxEmpty, _ := cli.GetContextFromCommand(cmdEmpty, initCtx)
	cli.SetContext(cmdEmpty, cliCtxEmpty)
	procEmpty, _ := cli.NewProcessor(cmdEmpty)
	lookupEmpty := vdsTitleLookup(procEmpty)
	idEmpty, err := lookupEmpty(ctx, objects.KindGoal, "No match")
	require.NoError(t, err)
	assert.Empty(t, idEmpty)

	// Nil storage in proc
	emptyProc := &cli.Processor{}
	nilLookup := vdsTitleLookup(emptyProc)
	assert.Nil(t, nilLookup)
}

func setupVDSTestProject(t *testing.T, root string) {
	qualityDir := filepath.Join(root, "docs", "quality")
	require.NoError(t, fileutil.MkdirAll(qualityDir, paths.DirPerm755))

	spinePath := filepath.Join(qualityDir, "verifiable_decomposition_spine_profile.yaml")
	spineYaml := `schema_version: 1
profile_id: verifiable_decomposition_spine_v1
customization_profile_path: docs/quality/verifiable_decomposition_customization.yaml
stages:
  - id: intent_capture
    label: Intent capture
    purpose: Persist goals
  - id: design
    label: Design
    purpose: Requirements
completion:
  done_values: ["yes", "na"]
  hard_gates: ["independent_verify"]
chunk_contract:
  required_fields: ["chunk_id", "stage", "claim", "rubric_ref", "dsl_checks", "evidence_refs", "independent_verify"]
`
	require.NoError(t, fileutil.WriteFile(spinePath, []byte(spineYaml), paths.FilePerm644))

	custPath := filepath.Join(qualityDir, "verifiable_decomposition_customization.yaml")
	custYaml := `schema_version: 1
profile_id: verifiable_decomposition_customization_v1
project_id: test
vendor_providers:
  default: cursor
  providers: []
`
	require.NoError(t, fileutil.WriteFile(custPath, []byte(custYaml), paths.FilePerm644))
}

func initTestCmdContext(t *testing.T, cmd *cobra.Command, root string, store storage.ObjectStorageProvider) {
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

func TestResolveVDSContext(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)
	setupVDSTestProject(t, proj.Root)

	cmd := NewVDSChecklistCmd()
	initTestCmdContext(t, cmd, proj.Root, nil)

	var flags clipkg.FlagBag
	vdsCtx, err := resolveVDSContext(cmd, nil, &flags)
	require.NoError(t, err)
	require.NotNil(t, vdsCtx)
	assert.Equal(t, proj.Root, vdsCtx.root)
	assert.NotNil(t, vdsCtx.spine)

	// Test resolveVDSExecutionContext
	execCtx, err := resolveVDSExecutionContext(cmd, &flags)
	require.NoError(t, err)
	require.NotNil(t, execCtx)
}

func TestRunVDSInit(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	cmd := NewVDSInitCmd()
	initTestCmdContext(t, cmd, proj.Root, nil)

	// First init - creates file
	err := runVDSInit(cmd, nil)
	require.NoError(t, err)

	chunksPath := filepath.Join(proj.Root, "docs", "quality", "vds_chunks.yaml")
	assert.True(t, fileutil.Exists(chunksPath))

	// Second init without force - should succeed but not recreate
	err = runVDSInit(cmd, nil)
	require.NoError(t, err)

	// Third init with force
	require.NoError(t, cmd.Flags().Set("force", "true"))
	err = runVDSInit(cmd, nil)
	require.NoError(t, err)
}

func TestRunVDSChecklist(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)
	setupVDSTestProject(t, proj.Root)

	cmd := NewVDSChecklistCmd()
	initTestCmdContext(t, cmd, proj.Root, nil)

	err := runVDSChecklist(cmd, nil)
	require.NoError(t, err)
}

func TestRunVDSEvaluate(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)
	setupVDSTestProject(t, proj.Root)

	// 1. Passing chunk
	qualityDir := filepath.Join(proj.Root, "docs", "quality")
	require.NoError(t, fileutil.MkdirAll(qualityDir, paths.DirPerm755))
	chunksPath := filepath.Join(qualityDir, "vds_chunks.yaml")
	chunksYaml := `chunks:
  - chunk_id: "CHK-001"
    stage: "intent_capture"
    claim: "Test Claim"
    rubric_ref: "docs/rubric.md"
    dsl_checks:
      - "path_exists:docs/quality/vds_chunks.yaml"
    evidence_refs:
      - "docs/quality/vds_chunks.yaml"
    independent_verify: "yes"
`
	require.NoError(t, fileutil.WriteFile(chunksPath, []byte(chunksYaml), paths.FilePerm644))

	cmd := NewVDSEvaluateCmd()
	initTestCmdContext(t, cmd, proj.Root, nil)
	require.NoError(t, cmd.Flags().Set("persist", "true"))
	require.NoError(t, cmd.Flags().Set("apply-verify", "true"))

	err := runVDSEvaluate(cmd, nil)
	require.NoError(t, err)

	// Verify report persisted
	reportPath := filepath.Join(proj.Root, paths.ProjectDataDir, paths.StateDir, "vds", "last_evaluate.json")
	assert.True(t, fileutil.Exists(reportPath))

	// 2. Failing chunk to exercise error path
	failingYaml := `chunks:
  - chunk_id: "CHK-FAIL"
    stage: "intent_capture"
    claim: "Failing Claim"
    rubric_ref: "docs/rubric.md"
    dsl_checks: []
    evidence_refs: []
    independent_verify: "yes"
`
	require.NoError(t, fileutil.WriteFile(chunksPath, []byte(failingYaml), paths.FilePerm644))
	errFail := runVDSEvaluate(cmd, nil)
	assert.Error(t, errFail)
	assert.Contains(t, errFail.Error(), "vds evaluate: FAIL")
}

func TestRunVDSProject(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)
	setupVDSTestProject(t, proj.Root)

	cmd := NewVDSProjectCmd()
	initTestCmdContext(t, cmd, proj.Root, nil)
	require.NoError(t, cmd.Flags().Set("list", "true"))

	// List only
	err := runVDSProject(cmd, nil)
	require.NoError(t, err)

	// Single provider check
	require.NoError(t, cmd.Flags().Set("list", "false"))
	require.NoError(t, cmd.Flags().Set("check", "true"))
	_ = runVDSProject(cmd, nil)

	// All providers check
	require.NoError(t, cmd.Flags().Set("all", "true"))
	_ = runVDSProject(cmd, nil)
}
