package validate

import (
	"bytes"
	"context"
	"go/ast"
	"go/token"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/execwrap"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestNewValidateCmd_Hierarchy(t *testing.T) {
	cmd := NewValidateCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "validate", cmd.Name())

	subcommands := []string{
		"agent",
		"build-prompt",
		"completion",
		"matrix",
		"object",
		"steward",
		"tree",
		"workflow",
	}

	for _, sub := range subcommands {
		subCmd, _, err := cmd.Find([]string{sub})
		require.NoError(t, err, "subcommand %s should be discoverable", sub)
		require.NotNil(t, subCmd, "subcommand %s should not be nil", sub)
	}
}

func TestValidate_Matrix(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{SeedSchemaPlane: true})

	// 1. Matrix validate in valid project root
	cmd := NewMatrixValidateCmd()
	var buf bytes.Buffer
	baseCtx := pkgctx.WithCommandOutputWriter(context.Background(), &buf)
	baseCtx = pkgctx.WithSecurityContext(baseCtx, pkgctx.NewSystemSecurityContext())
	cmd.SetContext(cli.WithStorageProvider(baseCtx, proj.FileStorage))
	initCtx := pkgctx.NewCliInitializationContext(cli.ResolveProjectRoot, ".")
	cctx, err := cli.GetContextFromCommand(cmd, initCtx)
	require.NoError(t, err)
	cctx.ProjectRoot = proj.Root
	cli.SetContext(cmd, cctx)

	// Execute with empty matrix name
	_ = cmd.Execute()

	// 2. Matrix validate with unknown matrix name
	cmd2 := NewMatrixValidateCmd()
	var buf2 bytes.Buffer
	baseCtx2 := pkgctx.WithCommandOutputWriter(context.Background(), &buf2)
	baseCtx2 = pkgctx.WithSecurityContext(baseCtx2, pkgctx.NewSystemSecurityContext())
	cmd2.SetContext(cli.WithStorageProvider(baseCtx2, proj.FileStorage))
	cctx2, err := cli.GetContextFromCommand(cmd2, initCtx)
	require.NoError(t, err)
	cctx2.ProjectRoot = proj.Root
	cli.SetContext(cmd2, cctx2)
	cmd2.SetArgs([]string{"--name", "non-existent-matrix"})

	err = cmd2.Execute()
	assert.Error(t, err)

	// 3. Matrix validate when project root is missing
	cmd3 := NewMatrixValidateCmd()
	var buf3 bytes.Buffer
	baseCtx3 := pkgctx.WithCommandOutputWriter(context.Background(), &buf3)
	cmd3.SetContext(baseCtx3)
	emptyCtx := cli.ContextForProjectRoot("")
	cli.SetContext(cmd3, emptyCtx)

	t.Setenv("ZQK_PROJECT_ROOT", "")
	t.Setenv("ZQK_TEST_ROOT", "")

	origWd, err := os.Getwd()
	require.NoError(t, err)
	tempEmpty := t.TempDir()
	require.NoError(t, os.Chdir(tempEmpty))
	t.Cleanup(func() {
		_ = os.Chdir(origWd)
	})

	err = cmd3.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "project root not found")
}

func TestValidate_BuildPrompt(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{SeedSchemaPlane: true})

	// 1. Success execution with full prompt options
	cmd := NewBuildPromptCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	baseCtx := pkgctx.WithCommandOutputWriter(context.Background(), &buf)
	baseCtx = pkgctx.WithSecurityContext(baseCtx, pkgctx.NewSystemSecurityContext())
	cmd.SetContext(cli.WithStorageProvider(baseCtx, proj.FileStorage))
	initCtx := pkgctx.NewCliInitializationContext(cli.ResolveProjectRoot, ".")
	cctx, err := cli.GetContextFromCommand(cmd, initCtx)
	require.NoError(t, err)
	cctx.ProjectRoot = proj.Root
	cli.SetContext(cmd, cctx)

	cmd.SetArgs([]string{
		"--task-title", "Implement Verification Feature",
		"--plan-title", "Cycle 75 Verification",
		"--plan-id", "PRI-1792008359177241000-141f7d25",
		"--target-agent", "Worker-01",
		"--capability", "golang",
		"--include-tdd",
	})

	err = cmd.Execute()
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Implement Verification Feature")

	// 2. Failure when --task-title is omitted
	cmdMissing := NewBuildPromptCmd()
	var bufMissing bytes.Buffer
	cmdMissing.SetOut(&bufMissing)
	cmdMissing.SetErr(&bufMissing)
	baseCtxMissing := pkgctx.WithCommandOutputWriter(context.Background(), &bufMissing)
	cmdMissing.SetContext(cli.WithStorageProvider(baseCtxMissing, proj.FileStorage))
	cctxMissing, err := cli.GetContextFromCommand(cmdMissing, initCtx)
	require.NoError(t, err)
	cctxMissing.ProjectRoot = proj.Root
	cli.SetContext(cmdMissing, cctxMissing)

	err = cmdMissing.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "requires --task-title")
}

func TestValidate_Object(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{SeedSchemaPlane: true})

	// Seed test objects
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	// Object 1: valid string machine hints
	objWithHints := map[string]any{
		objects.FieldKeyKind:         objects.KindBacklogItem,
		objects.FieldKeyID:           "BLI-TEST-HINTS-01",
		objects.FieldKeyTitle:        "Task with hints",
		objects.FieldKeyMachineHints: "CMD: echo 'hints validated'\nNOTE: ignore line",
	}
	require.NoError(t, proj.FileStorage.Create(ctx, secCtx, objWithHints))

	// Object 2: valid slice machine hints
	objWithSliceHints := map[string]any{
		objects.FieldKeyKind:  objects.KindBacklogItem,
		objects.FieldKeyID:    "BLI-TEST-HINTS-02",
		objects.FieldKeyTitle: "Task with slice hints",
		objects.FieldKeyMachineHints: []any{
			"CMD: echo 'slice validated'",
		},
	}
	require.NoError(t, proj.FileStorage.Create(ctx, secCtx, objWithSliceHints))

	// Object 3: missing machine hints
	objNoHints := map[string]any{
		objects.FieldKeyKind:  objects.KindBacklogItem,
		objects.FieldKeyID:    "BLI-TEST-NO-HINTS",
		objects.FieldKeyTitle: "Task without hints",
	}
	require.NoError(t, proj.FileStorage.Create(ctx, secCtx, objNoHints))

	// Object 4: unsupported type machine hints
	objBadHints := map[string]any{
		objects.FieldKeyKind:         objects.KindBacklogItem,
		objects.FieldKeyID:           "BLI-TEST-BAD-HINTS",
		objects.FieldKeyTitle:        "Task with bad hints",
		objects.FieldKeyMachineHints: 12345,
	}
	require.NoError(t, proj.FileStorage.Create(ctx, secCtx, objBadHints))

	// Test 1: Successful verification with string hints
	cmd1 := NewValidateObjectCmd()
	var buf1 bytes.Buffer
	baseCtx1 := pkgctx.WithCommandOutputWriter(context.Background(), &buf1)
	baseCtx1 = pkgctx.WithSecurityContext(baseCtx1, secCtx)
	cmd1.SetContext(cli.WithStorageProvider(baseCtx1, proj.FileStorage))
	initCtx := pkgctx.NewCliInitializationContext(cli.ResolveProjectRoot, ".")
	cctx1, err := cli.GetContextFromCommand(cmd1, initCtx)
	require.NoError(t, err)
	cctx1.ProjectRoot = proj.Root
	cli.SetContext(cmd1, cctx1)
	cmd1.SetArgs([]string{"BLI-TEST-HINTS-01"})

	err = cmd1.Execute()
	require.NoError(t, err)

	// Test 2: Successful verification with slice hints
	cmd2 := NewValidateObjectCmd()
	var buf2 bytes.Buffer
	baseCtx2 := pkgctx.WithCommandOutputWriter(context.Background(), &buf2)
	baseCtx2 = pkgctx.WithSecurityContext(baseCtx2, secCtx)
	cmd2.SetContext(cli.WithStorageProvider(baseCtx2, proj.FileStorage))
	cctx2, err := cli.GetContextFromCommand(cmd2, initCtx)
	require.NoError(t, err)
	cctx2.ProjectRoot = proj.Root
	cli.SetContext(cmd2, cctx2)
	cmd2.SetArgs([]string{"BLI-TEST-HINTS-02"})

	err = cmd2.Execute()
	require.NoError(t, err)

	// Test 3: Missing machine_hints error
	cmd3 := NewValidateObjectCmd()
	var buf3 bytes.Buffer
	baseCtx3 := pkgctx.WithCommandOutputWriter(context.Background(), &buf3)
	cmd3.SetContext(cli.WithStorageProvider(baseCtx3, proj.FileStorage))
	cctx3, err := cli.GetContextFromCommand(cmd3, initCtx)
	require.NoError(t, err)
	cctx3.ProjectRoot = proj.Root
	cli.SetContext(cmd3, cctx3)
	cmd3.SetArgs([]string{"BLI-TEST-NO-HINTS"})

	err = cmd3.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not contain machine_hints")

	// Test 4: Unsupported type machine_hints error
	cmd4 := NewValidateObjectCmd()
	var buf4 bytes.Buffer
	baseCtx4 := pkgctx.WithCommandOutputWriter(context.Background(), &buf4)
	cmd4.SetContext(cli.WithStorageProvider(baseCtx4, proj.FileStorage))
	cctx4, err := cli.GetContextFromCommand(cmd4, initCtx)
	require.NoError(t, err)
	cctx4.ProjectRoot = proj.Root
	cli.SetContext(cmd4, cctx4)
	cmd4.SetArgs([]string{"BLI-TEST-BAD-HINTS"})

	err = cmd4.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported type")

	// Test 5: Non-existent object error
	cmd5 := NewValidateObjectCmd()
	var buf5 bytes.Buffer
	baseCtx5 := pkgctx.WithCommandOutputWriter(context.Background(), &buf5)
	cmd5.SetContext(cli.WithStorageProvider(baseCtx5, proj.FileStorage))
	cctx5, err := cli.GetContextFromCommand(cmd5, initCtx)
	require.NoError(t, err)
	cctx5.ProjectRoot = proj.Root
	cli.SetContext(cmd5, cctx5)
	cmd5.SetArgs([]string{"BLI-DOES-NOT-EXIST"})

	err = cmd5.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read object")
}

func TestValidate_Steward(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{SeedSchemaPlane: true})

	cmd := NewStewardCmd()
	var buf bytes.Buffer
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately so loop returns cleanly

	baseCtx := pkgctx.WithCommandOutputWriter(ctx, &buf)
	baseCtx = pkgctx.WithSecurityContext(baseCtx, pkgctx.NewSystemSecurityContext())
	cmd.SetContext(cli.WithStorageProvider(baseCtx, proj.FileStorage))
	initCtx := pkgctx.NewCliInitializationContext(cli.ResolveProjectRoot, ".")
	cctx, err := cli.GetContextFromCommand(cmd, initCtx)
	require.NoError(t, err)
	cctx.ProjectRoot = proj.Root
	cli.SetContext(cmd, cctx)

	err = cmd.Execute()
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Roadmap Steward")
}

func TestValidate_Workflow(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{SeedSchemaPlane: true})

	cmd := NewCheckCmd()
	var buf bytes.Buffer
	baseCtx := pkgctx.WithCommandOutputWriter(context.Background(), &buf)
	baseCtx = pkgctx.WithSecurityContext(baseCtx, pkgctx.NewSystemSecurityContext())
	cmd.SetContext(cli.WithStorageProvider(baseCtx, proj.FileStorage))
	initCtx := pkgctx.NewCliInitializationContext(cli.ResolveProjectRoot, ".")
	cctx, err := cli.GetContextFromCommand(cmd, initCtx)
	require.NoError(t, err)
	cctx.ProjectRoot = proj.Root
	cli.SetContext(cmd, cctx)

	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	// Execute workflow check
	_ = cmd.Execute()
	assert.NotEmpty(t, buf.String())
}

func TestValidate_Agent(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{SeedSchemaPlane: true})
	t.Cleanup(func() {
		_ = filepath.Walk(proj.Root, func(path string, info os.FileInfo, err error) error {
			if err == nil {
				_ = os.Chmod(path, 0777)
			}
			return nil
		})
	})

	// Initialize git in proj.Root so git status/diff works cleanly
	gitInit := execwrap.Command("git", "init")
	gitInit.Dir = proj.Root
	_ = gitInit.Run()

	gitConfigUser := execwrap.Command("git", "config", "user.name", "Test User")
	gitConfigUser.Dir = proj.Root
	_ = gitConfigUser.Run()

	gitConfigEmail := execwrap.Command("git", "config", "user.email", "test@example.com")
	gitConfigEmail.Dir = proj.Root
	_ = gitConfigEmail.Run()

	gitAdd := execwrap.Command("git", "add", ".")
	gitAdd.Dir = proj.Root
	_ = gitAdd.Run()

	gitCommit := execwrap.Command("git", "commit", "-m", "initial commit", "--allow-empty")
	gitCommit.Dir = proj.Root
	_ = gitCommit.Run()

	// Create dummy go file and dummy dirs to satisfy validation traversal
	pkgDir := filepath.Join(proj.Root, "pkg", "sample")
	require.NoError(t, fileutil.MkdirAll(pkgDir, paths.DirPerm755))
	dummyGo := `package sample
func Dummy() int {
	return 42
}
`
	require.NoError(t, fileutil.WriteFile(filepath.Join(pkgDir, "sample.go"), []byte(dummyGo), paths.FilePerm644))

	cmd := NewValidateAgentCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	baseCtx := pkgctx.WithCommandOutputWriter(context.Background(), &buf)
	baseCtx = pkgctx.WithSecurityContext(baseCtx, pkgctx.NewSystemSecurityContext())
	cmd.SetContext(cli.WithStorageProvider(baseCtx, proj.FileStorage))
	initCtx := pkgctx.NewCliInitializationContext(cli.ResolveProjectRoot, ".")
	cctx, err := cli.GetContextFromCommand(cmd, initCtx)
	require.NoError(t, err)
	cctx.ProjectRoot = proj.Root
	cli.SetContext(cmd, cctx)

	// Test validate agent execution (returns error or output on uncommitted files)
	_ = cmd.Execute()

	// Test with stamp flag and local-fallback with public key configured
	t.Setenv("ZQK_AGENT_PUB_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	cmdStamp := NewValidateAgentCmd()
	var bufStamp bytes.Buffer
	cmdStamp.SetOut(&bufStamp)
	cmdStamp.SetErr(&bufStamp)
	baseCtxStamp := pkgctx.WithCommandOutputWriter(context.Background(), &bufStamp)
	baseCtxStamp = pkgctx.WithSecurityContext(baseCtxStamp, pkgctx.NewSystemSecurityContext())
	cmdStamp.SetContext(cli.WithStorageProvider(baseCtxStamp, proj.FileStorage))
	cctxStamp, err := cli.GetContextFromCommand(cmdStamp, initCtx)
	require.NoError(t, err)
	cctxStamp.ProjectRoot = proj.Root
	cli.SetContext(cmdStamp, cctxStamp)
	cmdStamp.SetArgs([]string{"--stamp", "invalid-stamp-payload", "--local-fallback"})

	err = cmdStamp.Execute()
	assert.Error(t, err)
}

func TestValidate_Tree_CLI(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{SeedSchemaPlane: true})
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	planObj := map[string]any{
		objects.FieldKeyKind:   objects.KindPriorityPlan,
		objects.FieldKeyID:     "PRI-TREE-001",
		objects.FieldKeyTitle:  "Tree Plan",
		objects.FieldKeyStatus: objects.ObjectStatusInProgress,
	}
	require.NoError(t, proj.FileStorage.Create(ctx, secCtx, planObj))

	// 1. Text tree output
	cmdText := NewTreeCmd()
	var bufText bytes.Buffer
	baseCtxText := pkgctx.WithCommandOutputWriter(context.Background(), &bufText)
	baseCtxText = pkgctx.WithSecurityContext(baseCtxText, secCtx)
	cmdText.SetContext(cli.WithStorageProvider(baseCtxText, proj.FileStorage))
	initCtx := pkgctx.NewCliInitializationContext(cli.ResolveProjectRoot, ".")
	cctxText, err := cli.GetContextFromCommand(cmdText, initCtx)
	require.NoError(t, err)
	cctxText.ProjectRoot = proj.Root
	cli.SetContext(cmdText, cctxText)
	cmdText.SetArgs([]string{"--priority-plan", "PRI-TREE-001"})

	err = cmdText.Execute()
	require.NoError(t, err)
	assert.Contains(t, bufText.String(), "PRI-TREE-001")

	// 2. JSON tree output
	cmdJSON := NewTreeCmd()
	var bufJSON bytes.Buffer
	baseCtxJSON := pkgctx.WithCommandOutputWriter(context.Background(), &bufJSON)
	baseCtxJSON = pkgctx.WithSecurityContext(baseCtxJSON, secCtx)
	cmdJSON.SetContext(cli.WithStorageProvider(baseCtxJSON, proj.FileStorage))
	cctxJSON, err := cli.GetContextFromCommand(cmdJSON, initCtx)
	require.NoError(t, err)
	cctxJSON.ProjectRoot = proj.Root
	cctxJSON.Format = cli.FormatJSON
	cli.SetContext(cmdJSON, cctxJSON)
	cmdJSON.SetArgs([]string{"--priority-plan", "PRI-TREE-001", "-f", "json"})

	err = cmdJSON.Execute()
	require.NoError(t, err)
	assert.Contains(t, bufJSON.String(), `"id": "PRI-TREE-001"`)
}

func TestValidate_VerifyCompletion(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{SeedSchemaPlane: true})
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	// 1. Unsupported kind
	reqObj := map[string]any{
		objects.FieldKeyKind:          objects.KindRequirement,
		objects.FieldKeyID:            "REQ-COMPLETION-001",
		objects.FieldKeyTitle:         "Requirement",
		objects.FieldKeyStatus:        objects.ObjectStatusProposed,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}
	require.NoError(t, proj.FileStorage.Create(ctx, secCtx, reqObj))

	cmdReq := NewVerifyCompletionCmd()
	var bufReq bytes.Buffer
	baseCtxReq := pkgctx.WithCommandOutputWriter(context.Background(), &bufReq)
	cmdReq.SetContext(cli.WithStorageProvider(baseCtxReq, proj.FileStorage))
	initCtx := pkgctx.NewCliInitializationContext(cli.ResolveProjectRoot, ".")
	cctxReq, err := cli.GetContextFromCommand(cmdReq, initCtx)
	require.NoError(t, err)
	cctxReq.ProjectRoot = proj.Root
	cli.SetContext(cmdReq, cctxReq)
	cmdReq.SetArgs([]string{"REQ-COMPLETION-001"})

	err = cmdReq.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "verification not supported")

	// 2. Object not found
	cmdNotFound := NewVerifyCompletionCmd()
	var bufNotFound bytes.Buffer
	baseCtxNotFound := pkgctx.WithCommandOutputWriter(context.Background(), &bufNotFound)
	cmdNotFound.SetContext(cli.WithStorageProvider(baseCtxNotFound, proj.FileStorage))
	cctxNotFound, err := cli.GetContextFromCommand(cmdNotFound, initCtx)
	require.NoError(t, err)
	cctxNotFound.ProjectRoot = proj.Root
	cli.SetContext(cmdNotFound, cctxNotFound)
	cmdNotFound.SetArgs([]string{"TST-NON-EXISTENT"})

	err = cmdNotFound.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")

	// 3. Test case missing verification_hash
	tcNoHash := map[string]any{
		objects.FieldKeyKind:   objects.KindTestCase,
		objects.FieldKeyID:     "TST-NO-HASH",
		objects.FieldKeyTitle:  "Test Case without hash",
		objects.FieldKeyStatus: objects.ObjectStatusDraft,
	}
	require.NoError(t, proj.FileStorage.Create(ctx, secCtx, tcNoHash))

	cmdNoHash := NewVerifyCompletionCmd()
	var bufNoHash bytes.Buffer
	baseCtxNoHash := pkgctx.WithCommandOutputWriter(context.Background(), &bufNoHash)
	cmdNoHash.SetContext(cli.WithStorageProvider(baseCtxNoHash, proj.FileStorage))
	cctxNoHash, err := cli.GetContextFromCommand(cmdNoHash, initCtx)
	require.NoError(t, err)
	cctxNoHash.ProjectRoot = proj.Root
	cli.SetContext(cmdNoHash, cctxNoHash)
	cmdNoHash.SetArgs([]string{"TST-NO-HASH"})

	err = cmdNoHash.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "has no verification_hash")

	// 4. Test case missing verified_artifact_refs
	tcNoArtifacts := map[string]any{
		objects.FieldKeyKind:             objects.KindTestCase,
		objects.FieldKeyID:               "TST-NO-ARTIFACTS",
		objects.FieldKeyTitle:            "Test Case without artifacts",
		objects.FieldKeyVerificationHash: "abc123hash",
		objects.FieldKeyStatus:           objects.ObjectStatusDraft,
	}
	require.NoError(t, proj.FileStorage.Create(ctx, secCtx, tcNoArtifacts))

	cmdNoArtifacts := NewVerifyCompletionCmd()
	var bufNoArtifacts bytes.Buffer
	baseCtxNoArtifacts := pkgctx.WithCommandOutputWriter(context.Background(), &bufNoArtifacts)
	cmdNoArtifacts.SetContext(cli.WithStorageProvider(baseCtxNoArtifacts, proj.FileStorage))
	cctxNoArtifacts, err := cli.GetContextFromCommand(cmdNoArtifacts, initCtx)
	require.NoError(t, err)
	cctxNoArtifacts.ProjectRoot = proj.Root
	cli.SetContext(cmdNoArtifacts, cctxNoArtifacts)
	cmdNoArtifacts.SetArgs([]string{"TST-NO-ARTIFACTS"})

	err = cmdNoArtifacts.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "has no verified_artifact_refs")

	// 5. Backlog item missing verification_hash
	bliNoHash := map[string]any{
		objects.FieldKeyKind:          objects.KindBacklogItem,
		objects.FieldKeyID:            "BLI-NO-HASH",
		objects.FieldKeyTitle:         "Backlog item without hash",
		objects.FieldKeyStatus:        objects.ObjectStatusPlanned,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}
	require.NoError(t, proj.FileStorage.Create(ctx, secCtx, bliNoHash))

	cmdBliNoHash := NewVerifyCompletionCmd()
	var bufBliNoHash bytes.Buffer
	baseCtxBliNoHash := pkgctx.WithCommandOutputWriter(context.Background(), &bufBliNoHash)
	cmdBliNoHash.SetContext(cli.WithStorageProvider(baseCtxBliNoHash, proj.FileStorage))
	cctxBliNoHash, err := cli.GetContextFromCommand(cmdBliNoHash, initCtx)
	require.NoError(t, err)
	cctxBliNoHash.ProjectRoot = proj.Root
	cli.SetContext(cmdBliNoHash, cctxBliNoHash)
	cmdBliNoHash.SetArgs([]string{"BLI-NO-HASH"})

	err = cmdBliNoHash.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "has no verification_hash")

	// 6. Backlog item with hash but no test cases
	bliNoTC := map[string]any{
		objects.FieldKeyKind:             objects.KindBacklogItem,
		objects.FieldKeyID:               "BLI-NO-TC",
		objects.FieldKeyTitle:            "Backlog item without TC",
		objects.FieldKeyVerificationHash: "abc123hash",
		objects.FieldKeyStatus:           objects.ObjectStatusPlanned,
		objects.FieldKeySchemaVersion:    objects.DefaultSchemaVersion,
	}
	require.NoError(t, proj.FileStorage.Create(ctx, secCtx, bliNoTC))

	cmdBliNoTC := NewVerifyCompletionCmd()
	var bufBliNoTC bytes.Buffer
	baseCtxBliNoTC := pkgctx.WithCommandOutputWriter(context.Background(), &bufBliNoTC)
	cmdBliNoTC.SetContext(cli.WithStorageProvider(baseCtxBliNoTC, proj.FileStorage))
	cctxBliNoTC, err := cli.GetContextFromCommand(cmdBliNoTC, initCtx)
	require.NoError(t, err)
	cctxBliNoTC.ProjectRoot = proj.Root
	cli.SetContext(cmdBliNoTC, cctxBliNoTC)
	cmdBliNoTC.SetArgs([]string{"BLI-NO-TC"})

	err = cmdBliNoTC.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "has no test cases")
}

func TestValidate_Agent_InternalHelpers(t *testing.T) {
	// 1. isFieldKeyStatus
	selExpr := &ast.SelectorExpr{
		X:   &ast.Ident{Name: "objects"},
		Sel: &ast.Ident{Name: "FieldKeyStatus"},
	}
	assert.True(t, isFieldKeyStatus(selExpr))

	otherSel := &ast.SelectorExpr{
		X:   &ast.Ident{Name: "other"},
		Sel: &ast.Ident{Name: "FieldKeyStatus"},
	}
	assert.False(t, isFieldKeyStatus(otherSel))
	assert.False(t, isFieldKeyStatus(&ast.Ident{Name: "status"}))

	// 2. isStatusKey
	assert.True(t, isStatusKey(&ast.Ident{Name: "Status"}))
	assert.True(t, isStatusKey(&ast.Ident{Name: "status"}))
	assert.False(t, isStatusKey(&ast.Ident{Name: "other"}))
	assert.True(t, isStatusKey(&ast.BasicLit{Kind: token.STRING, Value: `"status"`}))
	assert.False(t, isStatusKey(&ast.BasicLit{Kind: token.STRING, Value: `"other"`}))
	assert.False(t, isStatusKey(&ast.BasicLit{Kind: token.INT, Value: "123"}))

	// 3. getImpactedPackages
	pkgs := getImpactedPackages("/root", []string{"/root/pkg/a/foo.go", "/root/pkg/b/bar.go"})
	assert.Len(t, pkgs, 2)
	assert.Contains(t, pkgs, "./pkg/a")
	assert.Contains(t, pkgs, "./pkg/b")

	// 4. gitDiffContains
	contains, _ := gitDiffContains(".", "non-existent-diff-pattern-12345")
	assert.False(t, contains)
}

func TestValidate_CountTotalDescendants(t *testing.T) {
	node := &TreeNode{
		Children: []*TreeNode{
			{
				Children: []*TreeNode{
					{},
					{},
				},
			},
			{},
		},
	}
	assert.Equal(t, 4, countTotalDescendants(node))
}

func TestValidate_CheckASTViolations(t *testing.T) {
	tempDir := t.TempDir()

	// 1. Non-existent file
	err := checkASTViolations(filepath.Join(tempDir, "does-not-exist.go"))
	assert.NoError(t, err)

	// 2. Parse error
	invalidGo := filepath.Join(tempDir, "invalid.go")
	require.NoError(t, fileutil.WriteFile(invalidGo, []byte("package invalid\nfunc () {"), paths.FilePerm644))
	err = checkASTViolations(invalidGo)
	assert.Error(t, err)

	// 3. Raw cobra.Command outside allowed packages
	rawCobra := filepath.Join(tempDir, "raw_cobra.go")
	cobraContent := `package internal
import "github.com/spf13/cobra"
func MakeCmd() *cobra.Command {
	return &cobra.Command{}
}
`
	require.NoError(t, fileutil.WriteFile(rawCobra, []byte(cobraContent), paths.FilePerm644))
	err = checkASTViolations(rawCobra)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "raw &cobra.Command instantiation")

	// 4. Raw string literal assigned to objects.FieldKeyStatus
	assignStatus := filepath.Join(tempDir, "assign_status.go")
	assignContent := `package sample
import "github.com/zqk-os/zqk/pkg/objects"
func SetStatus() {
	objects.FieldKeyStatus = "in_progress"
}
`
	require.NoError(t, fileutil.WriteFile(assignStatus, []byte(assignContent), paths.FilePerm644))
	err = checkASTViolations(assignStatus)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "raw string literal assignment to objects.FieldKeyStatus")

	// 5. Raw string literal assigned to Status field in map
	keyValStatus := filepath.Join(tempDir, "keyval_status.go")
	keyValContent := `package sample
func MakeMap() map[string]any {
	return map[string]any{
		"status": "in_progress",
	}
}
`
	require.NoError(t, fileutil.WriteFile(keyValStatus, []byte(keyValContent), paths.FilePerm644))
	err = checkASTViolations(keyValStatus)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "raw string literal assigned to Status field")

	// 6. Clean file
	cleanGo := filepath.Join(tempDir, "clean.go")
	cleanContent := `package sample
func Clean() int {
	return 100
}
`
	require.NoError(t, fileutil.WriteFile(cleanGo, []byte(cleanContent), paths.FilePerm644))
	err = checkASTViolations(cleanGo)
	assert.NoError(t, err)
}

func TestValidate_VerifyVettingMatrix(t *testing.T) {
	tempDir := t.TempDir()

	// 1. Missing registry file
	err := verifyVettingMatrix(tempDir, []string{"pkg/foo/bar.go"})
	assert.NoError(t, err) // Missing registry yaml returns nil or skip

	// 2. Registry with missing CSV
	regDir := filepath.Join(tempDir, "docs", "quality")
	require.NoError(t, fileutil.MkdirAll(regDir, paths.DirPerm755))
	regContent := `matrices:
  codebase_vetting:
    csv: "docs/quality/codebase_vetting.csv"
`
	require.NoError(t, fileutil.WriteFile(filepath.Join(regDir, "matrix_registry.yaml"), []byte(regContent), paths.FilePerm644))
	err = verifyVettingMatrix(tempDir, []string{filepath.Join(tempDir, "pkg/foo/bar.go")})
	assert.NoError(t, err) // missing CSV file skips validation

	// 3. CSV with invalid header
	csvPath := filepath.Join(regDir, "codebase_vetting.csv")
	require.NoError(t, fileutil.WriteFile(csvPath, []byte("colA,colB,colC\n"), paths.FilePerm644))
	err = verifyVettingMatrix(tempDir, []string{filepath.Join(tempDir, "pkg/foo/bar.go")})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing required columns")

	// 4. CSV with changed file not vetted
	validCSV := `file_path,fully_vetted,fully_refactored_dry
pkg/foo/bar.go,no,no
`
	require.NoError(t, fileutil.WriteFile(csvPath, []byte(validCSV), paths.FilePerm644))
	err = verifyVettingMatrix(tempDir, []string{filepath.Join(tempDir, "pkg/foo/bar.go")})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not marked as fully_vetted=yes")

	// 5. CSV with changed file fully vetted
	vettedCSV := `file_path,fully_vetted,fully_refactored_dry
pkg/foo/bar.go,yes,yes
`
	require.NoError(t, fileutil.WriteFile(csvPath, []byte(vettedCSV), paths.FilePerm644))
	err = verifyVettingMatrix(tempDir, []string{filepath.Join(tempDir, "pkg/foo/bar.go")})
	assert.NoError(t, err)
}

func TestValidate_Workflow_FullAdherence(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{SeedSchemaPlane: true})
	t.Cleanup(func() {
		_ = filepath.Walk(proj.Root, func(path string, info os.FileInfo, err error) error {
			if err == nil {
				_ = os.Chmod(path, 0777)
			}
			return nil
		})
	})
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	// Initialize git repo with main branch
	gitInit := execwrap.Command("git", "init", "-b", "main")
	gitInit.Dir = proj.Root
	require.NoError(t, gitInit.Run())

	gitConfigUser := execwrap.Command("git", "config", "user.name", "Test User")
	gitConfigUser.Dir = proj.Root
	_ = gitConfigUser.Run()

	gitConfigEmail := execwrap.Command("git", "config", "user.email", "test@example.com")
	gitConfigEmail.Dir = proj.Root
	_ = gitConfigEmail.Run()

	gitignorePath := filepath.Join(proj.Root, ".gitignore")
	require.NoError(t, fileutil.WriteFile(gitignorePath, []byte(".zqk/\n"), paths.FilePerm644))

	gitAdd := execwrap.Command("git", "add", ".")
	gitAdd.Dir = proj.Root
	_ = gitAdd.Run()

	gitCommit := execwrap.Command("git", "commit", "-m", "initial main commit", "--allow-empty")
	gitCommit.Dir = proj.Root
	require.NoError(t, gitCommit.Run())

	// Create feature branch
	gitBranch := execwrap.Command("git", "checkout", "-b", "feat/BLI-WORKFLOW-001")
	gitBranch.Dir = proj.Root
	require.NoError(t, gitBranch.Run())

	// Create active priority plan with BLI-WORKFLOW-001
	planObj := map[string]any{
		objects.FieldKeyKind:            objects.KindPriorityPlan,
		objects.FieldKeyID:              "PRI-WORKFLOW-001",
		objects.FieldKeyTitle:           "Workflow Adherence Plan",
		objects.FieldKeyStatus:          objects.ObjectStatusActive,
		objects.FieldKeyBacklogItemRefs: []any{"BLI-WORKFLOW-001"},
		objects.FieldKeySchemaVersion:   objects.DefaultSchemaVersion,
	}
	storage.CreateCASVisible(t, proj.FileStorage, ctx, secCtx, planObj, objects.ObjectStatusActive)

	// Stage and commit all generated files so git state is clean
	gitAdd2 := execwrap.Command("git", "add", ".")
	gitAdd2.Dir = proj.Root
	_ = gitAdd2.Run()

	gitCommit2 := execwrap.Command("git", "commit", "-m", "plan commit", "--allow-empty")
	gitCommit2.Dir = proj.Root
	_ = gitCommit2.Run()

	cmd := NewCheckCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	baseCtx := pkgctx.WithCommandOutputWriter(context.Background(), &buf)
	baseCtx = pkgctx.WithSecurityContext(baseCtx, secCtx)
	cmd.SetContext(cli.WithStorageProvider(baseCtx, proj.FileStorage))
	initCtx := pkgctx.NewCliInitializationContext(cli.ResolveProjectRoot, ".")
	cctx, err := cli.GetContextFromCommand(cmd, initCtx)
	require.NoError(t, err)
	cctx.ProjectRoot = proj.Root
	cli.SetContext(cmd, cctx)

	err = cmd.Execute()
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Workflow Adherence Score:")
}
