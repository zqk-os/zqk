package validate

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"go/ast"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/crypto"
	"github.com/zqk-os/zqk/pkg/execwrap"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func setupGitRepo(t *testing.T, root string) {
	cInit := execwrap.Command("git", "init")
	cInit.Dir = root
	require.NoError(t, cInit.Run())

	cCfg1 := execwrap.Command("git", "config", "user.name", "Tester")
	cCfg1.Dir = root
	require.NoError(t, cCfg1.Run())

	cCfg2 := execwrap.Command("git", "config", "user.email", "test@example.com")
	cCfg2.Dir = root
	require.NoError(t, cCfg2.Run())

	readme := filepath.Join(root, "README.md")
	require.NoError(t, fileutil.WriteFile(readme, []byte("# Repo\n"), 0644))

	cAdd := execwrap.Command("git", "add", "README.md")
	cAdd.Dir = root
	require.NoError(t, cAdd.Run())

	cCommit := execwrap.Command("git", "commit", "-m", "initial")
	cCommit.Dir = root
	require.NoError(t, cCommit.Run())
}

func prepareQuickCmd(t *testing.T, cmd *cobra.Command) (string, *bytes.Buffer) {
	t.Helper()
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	setupGitRepo(t, proj.Root)
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
	return proj.Root, &buf
}

func prepareIsolatedCmd(t *testing.T, cmd *cobra.Command) (testkit.IsolatedTempProject, *bytes.Buffer) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{SeedSchemaPlane: true})
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
	return proj, &buf
}

func TestSteward_Loop_CanceledContext(t *testing.T) {
	cmd := NewStewardCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	tmp := t.TempDir()

	baseCtx, cancel := context.WithCancel(context.Background())
	cancel()
	baseCtx = pkgctx.WithCommandOutputWriter(baseCtx, &buf)
	baseCtx = pkgctx.WithSecurityContext(baseCtx, pkgctx.NewSystemSecurityContext())
	cmd.SetContext(cli.WithStorageProvider(baseCtx, &stewardMockStorage{}))
	initCtx := pkgctx.NewCliInitializationContext(cli.ResolveProjectRoot, ".")
	cctx, err := cli.GetContextFromCommand(cmd, initCtx)
	require.NoError(t, err)
	cctx.ProjectRoot = tmp
	cli.SetContext(cmd, cctx)

	err = runStewardLoop(cmd)
	require.NoError(t, err)
}

type stewardMockStorage struct {
	storage.ObjectStorageProvider
	plans    []map[string]any
	promoted map[string]string
}

func (s *stewardMockStorage) List(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter storage.ListFilter) (*storage.QueryResult, error) {
	if filter.Kind == objects.KindPriorityPlan {
		return &storage.QueryResult{Objects: s.plans}, nil
	}
	return &storage.QueryResult{Objects: nil}, nil
}

func (s *stewardMockStorage) Update(ctx context.Context, secCtx *storage.SecurityContext, id string, data map[string]any) error {
	if s.promoted == nil {
		s.promoted = make(map[string]string)
	}
	if st, ok := data[objects.FieldKeyStatus].(string); ok {
		s.promoted[id] = st
	}
	return nil
}

func TestSteward_Loop_PreflightAndPromotion(t *testing.T) {
	origInterval := stewardTickerInterval
	stewardTickerInterval = 5 * time.Millisecond
	defer func() { stewardTickerInterval = origInterval }()

	cmd := NewStewardCmd()
	proj, buf := prepareIsolatedCmd(t, cmd)

	mockStore := &stewardMockStorage{
		ObjectStorageProvider: proj.FileStorage,
		plans: []map[string]any{
			{
				objects.FieldKeyKind:   objects.KindPriorityPlan,
				objects.FieldKeyID:     "PRI-NOBLI-001",
				objects.FieldKeyTitle:  "Plan Without BLIs",
				objects.FieldKeyStatus: objects.ObjectStatusGrooming,
			},
			{
				objects.FieldKeyKind:            objects.KindPriorityPlan,
				objects.FieldKeyID:              "PRI-WITHBLI-002",
				objects.FieldKeyTitle:           "Plan With BLIs",
				objects.FieldKeyStatus:          objects.ObjectStatusGrooming,
				objects.FieldKeyBacklogItemRefs: []string{"BLI-STEWARD-001"},
			},
		},
		promoted: make(map[string]string),
	}

	baseCtx := pkgctx.WithCommandOutputWriter(context.Background(), buf)
	baseCtx = pkgctx.WithSecurityContext(baseCtx, pkgctx.NewSystemSecurityContext())
	cmd.SetContext(cli.WithStorageProvider(baseCtx, mockStore))
	initCtx := pkgctx.NewCliInitializationContext(cli.ResolveProjectRoot, ".")
	cctx, err := cli.GetContextFromCommand(cmd, initCtx)
	require.NoError(t, err)
	cctx.ProjectRoot = proj.Root
	cli.SetContext(cmd, cctx)

	ctx, cancel := context.WithCancel(cmd.Context())
	defer cancel()
	cmd.SetContext(ctx)

	done := make(chan error, 1)
	goroutinelabels.NewGoroutine("test_steward_loop", "run steward loop").StartSimple(func() {
		done <- runStewardLoop(cmd)
	})

	require.Eventually(t, func() bool {
		return mockStore.promoted["PRI-WITHBLI-002"] == objects.ObjectStatusActive
	}, 1*time.Second, 10*time.Millisecond)

	cancel()
	err = <-done
	require.NoError(t, err)

	assert.NotEqual(t, objects.ObjectStatusActive, mockStore.promoted["PRI-NOBLI-001"])

	out := buf.String()
	assert.Contains(t, out, "Plan PRI-NOBLI-001 is grooming but lacks backlog items")
	assert.Contains(t, out, "Plan PRI-WITHBLI-002 has backlog items. Preflight passed")
}

func TestWorkflow_PriorityPlanAdherence_StringRefs(t *testing.T) {
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	emptyStore := newMemoryWorkflowStore()
	msg, ok := checkPriorityPlanAdherence(ctx, secCtx, emptyStore, "BLI-1")
	assert.False(t, ok)
	assert.Contains(t, msg, "No active priority plan found")

	planStr := map[string]any{
		objects.FieldKeyKind:            objects.KindPriorityPlan,
		objects.FieldKeyID:              "PRI-1",
		objects.FieldKeyStatus:          objects.ObjectStatusActive,
		objects.FieldKeyBacklogItemRefs: []string{"BLI-1", "BLI-2"},
	}
	storeStr := newMemoryWorkflowStore(planStr)
	msg, ok = checkPriorityPlanAdherence(ctx, secCtx, storeStr, "BLI-OTHER")
	assert.False(t, ok)
	assert.Contains(t, msg, "is not in the active priority plan")

	msg, ok = checkPriorityPlanAdherence(ctx, secCtx, storeStr, "BLI-2")
	assert.False(t, ok)
	assert.Contains(t, msg, "is not the first non-completed item")

	msg, ok = checkPriorityPlanAdherence(ctx, secCtx, storeStr, "BLI-1")
	assert.True(t, ok)
	assert.Empty(t, msg)
}

func TestWorkflow_PriorityPlanAdherence_AnyRefs(t *testing.T) {
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	planAny := map[string]any{
		objects.FieldKeyKind:            objects.KindPriorityPlan,
		objects.FieldKeyID:              "PRI-2",
		objects.FieldKeyStatus:          objects.ObjectStatusActive,
		objects.FieldKeyBacklogItemRefs: []any{"BLI-A", "BLI-B"},
	}
	storeAny := newMemoryWorkflowStore(planAny)
	msg, ok := checkPriorityPlanAdherence(ctx, secCtx, storeAny, "BLI-A")
	assert.True(t, ok)
	assert.Empty(t, msg)

	msg, ok = checkPriorityPlanAdherence(ctx, secCtx, storeAny, "BLI-UNKNOWN")
	assert.False(t, ok)
	assert.Contains(t, msg, "is not in the active priority plan")
}

func TestWorkflow_GitChecks_NonGit(t *testing.T) {
	tempEmpty := t.TempDir()
	cleanMsg, isClean := checkGitClean(tempEmpty)
	assert.False(t, isClean)
	assert.Contains(t, cleanMsg, "Git status check failed")

	branch, branchMsg, ok := checkBranchNaming(tempEmpty)
	assert.False(t, ok)
	assert.Empty(t, branch)
	assert.Contains(t, branchMsg, "Failed to get current branch")

	baseMsg, baseOk := checkBranchBase(tempEmpty)
	assert.True(t, baseOk)
	assert.Empty(t, baseMsg)
}

func TestWorkflow_BranchNaming_GitRepo(t *testing.T) {
	repoDir := t.TempDir()
	setupGitRepo(t, repoDir)

	activeItem, branchMsg, ok := checkBranchNaming(repoDir)
	if activeItem == "main" || activeItem == "master" {
		assert.False(t, ok)
		assert.Contains(t, branchMsg, "Currently on main branch")
	}
}

func TestTree_PrintTree_DepthAndConnectors(t *testing.T) {
	grandChild := &TreeNode{
		ID:     "CRIT-001",
		Type:   objects.KindCriteria,
		Title:  "Leaf Criterion",
		Status: objects.ObjectStatusActive,
	}
	child1 := &TreeNode{
		ID:       "BLI-001",
		Type:     objects.KindBacklogItem,
		Title:    "First BLI",
		Status:   objects.ObjectStatusActive,
		Children: []*TreeNode{grandChild},
	}
	child2 := &TreeNode{
		ID:       "BLI-002",
		Type:     objects.KindBacklogItem,
		Title:    "Second BLI",
		Status:   objects.ObjectStatusActive,
		Children: []*TreeNode{grandChild},
	}
	root := &TreeNode{
		ID:       "PRI-ROOT",
		Type:     objects.KindPriorityPlan,
		Title:    "Root Plan",
		Status:   objects.ObjectStatusActive,
		Children: []*TreeNode{child1, child2},
	}

	var buf1 strings.Builder
	printTree(&buf1, root, "", true, 0, 0)
	out1 := buf1.String()
	assert.Contains(t, out1, "(+1 rolled up dependencies)")

	var buf2 strings.Builder
	printTree(&buf2, root, "", true, 0, 1)
	out2 := buf2.String()
	assert.Contains(t, out2, "BLI-001")
	assert.Contains(t, out2, "BLI-002")
}

func TestTree_Resolve_NotFound(t *testing.T) {
	resolver := NewDependencyTreeResolver(newMemoryWorkflowStore())
	node, err := resolver.Resolve(context.Background(), "PRI-NONEXISTENT")
	require.Error(t, err)
	assert.Nil(t, node)
}

func TestAgent_FindChangedGoFiles_NonGit(t *testing.T) {
	tempEmpty := t.TempDir()
	files, err := findChangedGoFiles(tempEmpty)
	assert.Error(t, err)
	assert.Nil(t, files)

	hasDiff, err := gitDiffContains(tempEmpty, "something")
	assert.Error(t, err)
	assert.False(t, hasDiff)
}

func TestAgent_GetImpactedPackages(t *testing.T) {
	root := "/test/project"
	files := []string{
		"/test/project/cmd/zqk/main.go",
		"/test/project/pkg/util/util.go",
		"/test/project/cmd/zqk/sub/sub.go",
		"/test/project/cmd/zqk/main2.go",
	}
	pkgs := getImpactedPackages(root, files)
	assert.Equal(t, []string{"./cmd/zqk", "./cmd/zqk/sub", "./pkg/util"}, pkgs)
}

func TestAgent_VerifyVettingMatrix_MissingRegistry(t *testing.T) {
	tempEmpty := t.TempDir()
	regDir := filepath.Join(tempEmpty, "docs", "quality")
	require.NoError(t, fileutil.MkdirAll(regDir, 0755))
	badYAML := "schema_version: \"2.0.0\"\nmatrices: [\n"
	require.NoError(t, fileutil.WriteFile(filepath.Join(regDir, "matrix_registry.yaml"), []byte(badYAML), 0644))
	err := verifyVettingMatrix(tempEmpty, []string{"foo.go"})
	assert.Error(t, err)
}

func TestAgent_VerifyVettingMatrix_NoCodebaseVetting(t *testing.T) {
	tmp := t.TempDir()
	err := verifyVettingMatrix(tmp, []string{filepath.Join(tmp, "foo.go")})
	assert.NoError(t, err)
}

func TestAgent_VerifyVettingMatrix_CSVValidation(t *testing.T) {
	tmp := t.TempDir()
	regDir := filepath.Join(tmp, "docs", "quality")
	require.NoError(t, fileutil.MkdirAll(regDir, 0755))

	csvBad := filepath.Join(regDir, "codebase_vetting.csv")
	require.NoError(t, fileutil.WriteFile(csvBad, []byte("file_path,other_col\nfoo.go,yes\n"), 0644))
	regContent := fmt.Sprintf("schema_version: \"2.0.0\"\nmatrices:\n  codebase_vetting:\n    csv: %s\n", csvBad)
	require.NoError(t, fileutil.WriteFile(filepath.Join(regDir, "matrix_registry.yaml"), []byte(regContent), 0644))

	err := verifyVettingMatrix(tmp, []string{filepath.Join(tmp, "foo.go")})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing required columns")

	csvUnvetted := "file_path,fully_vetted,fully_refactored_dry\nfoo.go,no,yes\n"
	require.NoError(t, fileutil.WriteFile(csvBad, []byte(csvUnvetted), 0644))
	err = verifyVettingMatrix(tmp, []string{filepath.Join(tmp, "foo.go")})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not marked as fully_vetted=yes")

	csvVetted := "file_path,fully_vetted,fully_refactored_dry\nfoo.go,yes,yes\n"
	require.NoError(t, fileutil.WriteFile(csvBad, []byte(csvVetted), 0644))
	err = verifyVettingMatrix(tmp, []string{filepath.Join(tmp, "foo.go")})
	require.NoError(t, err)
}

func TestAgent_RunValidateAgent_NoCodeChanges(t *testing.T) {
	cmd := NewValidateAgentCmd()
	_, _ = prepareQuickCmd(t, cmd)

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no code changes detected")
}

func TestAgent_RunValidateAgent_WithChangedFiles(t *testing.T) {
	cmd := NewValidateAgentCmd()
	root, buf := prepareQuickCmd(t, cmd)

	srcFile := filepath.Join(root, "example.go")
	require.NoError(t, fileutil.WriteFile(srcFile, []byte("package main\n\nfunc main() {}\n"), 0644))

	t.Setenv("ZQK_TASK_ARTIFACTS", "pkg/different")
	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no changes detected in required target artifacts")

	t.Setenv("ZQK_TASK_ARTIFACTS", "example.go")
	err = cmd.Execute()
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Validation analysis complete")
}

func TestAgent_RunValidateAgent_CryptoStamps(t *testing.T) {
	cmd1 := NewValidateAgentCmd()
	root, _ := prepareQuickCmd(t, cmd1)

	// Case 1: Missing pubkey
	t.Setenv("ZQK_AGENT_PUB_KEY", "")
	cmd1.SetArgs([]string{"--stamp", "some-stamp"})
	err := cmd1.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be set to verify cryptographic stamp")

	// Case 2: Valid hex pubkey
	pub, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	privHex := hex.EncodeToString(priv)
	pubHex := hex.EncodeToString(pub)

	stamp, err := crypto.GenerateStamp("commit123", "agent-x", privHex)
	require.NoError(t, err)

	cmd2 := NewValidateAgentCmd()
	var buf2 bytes.Buffer
	cmd2.SetOut(&buf2)
	cmd2.SetErr(&buf2)
	baseCtx := pkgctx.WithCommandOutputWriter(context.Background(), &buf2)
	baseCtx = pkgctx.WithSecurityContext(baseCtx, pkgctx.NewSystemSecurityContext())
	cmd2.SetContext(baseCtx)
	initCtx := pkgctx.NewCliInitializationContext(cli.ResolveProjectRoot, ".")
	cctx2, err := cli.GetContextFromCommand(cmd2, initCtx)
	require.NoError(t, err)
	cctx2.ProjectRoot = root
	cli.SetContext(cmd2, cctx2)

	t.Setenv("ZQK_AGENT_PUB_KEY", pubHex)
	cmd2.SetArgs([]string{"--stamp", stamp})
	err = cmd2.Execute()
	require.Error(t, err)
	assert.Contains(t, buf2.String(), "Verified cryptographic stamp for agent agent-x")

	// Case 3: Base64 with local fallback
	cmd3 := NewValidateAgentCmd()
	var buf3 bytes.Buffer
	cmd3.SetOut(&buf3)
	cmd3.SetErr(&buf3)
	baseCtx3 := pkgctx.WithCommandOutputWriter(context.Background(), &buf3)
	baseCtx3 = pkgctx.WithSecurityContext(baseCtx3, pkgctx.NewSystemSecurityContext())
	cmd3.SetContext(baseCtx3)
	cctx3, err := cli.GetContextFromCommand(cmd3, initCtx)
	require.NoError(t, err)
	cctx3.ProjectRoot = root
	cli.SetContext(cmd3, cctx3)

	b64Pub := base64.StdEncoding.EncodeToString(pub)
	stamp2, err := crypto.GenerateStamp("commit456", "agent-y", privHex)
	require.NoError(t, err)

	t.Setenv("ZQK_AGENT_PUB_KEY", b64Pub)
	cmd3.SetArgs([]string{"--stamp", stamp2, "--local-fallback"})
	err = cmd3.Execute()
	require.Error(t, err)
	assert.Contains(t, buf3.String(), "Verified cryptographic stamp for agent agent-y")
}

func TestCommandCtorAST_Branches(t *testing.T) {
	err := checkCommandCtorPattern("test.go", nil)
	require.NoError(t, err)

	wrapped := commandCtorWrapsAnother(nil)
	assert.Empty(t, wrapped)

	wrapped = commandCtorWrapsAnother(&ast.FuncDecl{
		Name: &ast.Ident{Name: "NewFooCmd"},
	})
	assert.Empty(t, wrapped)

	name := callExprName(&ast.CallExpr{Fun: &ast.BasicLit{Value: "123"}})
	assert.Empty(t, name)
}

func TestCompletion_VerifyTestCase_MissingProjectRoot(t *testing.T) {
	t.Setenv("ZQK_PROJECT_ROOT", "")
	t.Setenv("ZQK_TEST_ROOT", "")
	obj := map[string]any{
		objects.FieldKeyVerificationHash:     "abc",
		objects.FieldKeyVerifiedArtifactRefs: []any{"file.txt"},
	}
	err := verifyTestCase(nil, nil, obj, "TEST-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "project root not set")
}

func TestCompletion_VerifyTestCase_ValidationFailures(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("ZQK_PROJECT_ROOT", tmp)

	err := verifyTestCase(nil, nil, map[string]any{}, "TEST-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "has no verification_hash")

	err = verifyTestCase(nil, nil, map[string]any{
		objects.FieldKeyVerificationHash: "somehash",
	}, "TEST-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "has no verified_artifact_refs")

	err = verifyTestCase(nil, nil, map[string]any{
		objects.FieldKeyVerificationHash:     "somehash",
		objects.FieldKeyVerifiedArtifactRefs: []any{"nonexistent.txt"},
	}, "TEST-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to open artifact")

	testFile := filepath.Join(tmp, "artifact.txt")
	require.NoError(t, fileutil.WriteFile(testFile, []byte("hello"), 0644))
	err = verifyTestCase(nil, nil, map[string]any{
		objects.FieldKeyVerificationHash:     "wronghash",
		objects.FieldKeyVerifiedArtifactRefs: []any{"artifact.txt"},
	}, "TEST-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "verification failed for test_case")
}

func TestCompletion_VerifyTestCase_Success(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("ZQK_PROJECT_ROOT", tmp)

	testFile := filepath.Join(tmp, "artifact.txt")
	require.NoError(t, fileutil.WriteFile(testFile, []byte("hello"), 0644))

	expectedHash, err := computeTestCaseHash(tmp, []string{"artifact.txt"})
	require.NoError(t, err)

	cmd := NewVerifyCompletionCmd()
	var buf bytes.Buffer
	baseCtx := pkgctx.WithCommandOutputWriter(context.Background(), &buf)
	cmd.SetContext(baseCtx)

	err = verifyTestCase(cmd, nil, map[string]any{
		objects.FieldKeyVerificationHash:     expectedHash,
		objects.FieldKeyVerifiedArtifactRefs: []any{"artifact.txt"},
	}, "TEST-1")
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Successfully verified test_case TEST-1")
}

func TestCompletion_Verification_Scenarios(t *testing.T) {
	cmd := NewVerifyCompletionCmd()
	proj, buf := prepareIsolatedCmd(t, cmd)
	require.NotNil(t, buf)
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	// 1. Unsupported kind
	goalObj := map[string]any{
		objects.FieldKeyKind:       objects.KindGoal,
		objects.FieldKeyID:         "GOAL-1",
		objects.FieldKeyTitle:      "Unsupported Goal Kind",
		objects.FieldKeyTarget:     "1",
		objects.FieldKeyMetric:     "count",
		objects.FieldKeyStatus:     objects.ObjectStatusActive,
		objects.FieldKeyTargetDate: "2026-12-31",
	}
	require.NoError(t, proj.FileStorage.Create(ctx, secCtx, goalObj))

	err := runVerifyCompletion(cmd, "GOAL-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "verification not supported for kind goal")

	err = runVerifyCompletion(cmd, "BLI-999999")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")

	// 2. BLI validation failures
	proc, err := cli.NewProcessor(cmd)
	require.NoError(t, err)

	err = verifyBacklogItem(cmd, proc, map[string]any{}, "BLI-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "has no verification_hash")

	err = verifyBacklogItem(cmd, proc, map[string]any{
		objects.FieldKeyVerificationHash: "abc",
	}, "BLI-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "has no test cases")

	// 3. BLI success verification
	t.Setenv("ZQK_PROJECT_ROOT", proj.Root)
	testFile := filepath.Join(proj.Root, "src.go")
	require.NoError(t, fileutil.WriteFile(testFile, []byte("package main"), 0644))

	tcHash, err := computeTestCaseHash(proj.Root, []string{"src.go"})
	require.NoError(t, err)

	tcObj := map[string]any{
		objects.FieldKeyKind:                 objects.KindTestCase,
		objects.FieldKeyID:                   "TEST-1",
		objects.FieldKeyTitle:                "Test Case for BLI",
		objects.FieldKeyStatus:               objects.ObjectStatusActive,
		objects.FieldKeyPathOrID:             "src.go",
		objects.FieldKeyScope:                "unit",
		objects.FieldKeyBacklogItemRefs:      []string{"BLI-1"},
		objects.FieldKeyVerificationHash:     tcHash,
		objects.FieldKeyVerifiedArtifactRefs: []any{"src.go"},
	}
	require.NoError(t, proj.FileStorage.Create(ctx, secCtx, tcObj))

	bliHasher := sha256.New()
	bliHasher.Write([]byte("TEST-1:" + tcHash + "\n"))
	expectedBLIHash := hex.EncodeToString(bliHasher.Sum(nil))

	bliObj := map[string]any{
		objects.FieldKeyKind:             objects.KindBacklogItem,
		objects.FieldKeyID:               "BLI-1",
		objects.FieldKeyTitle:            "Completed BLI",
		objects.FieldKeyStatus:           objects.ObjectStatusPlanned,
		objects.FieldKeyVerificationHash: expectedBLIHash,
	}
	require.NoError(t, proj.FileStorage.Create(ctx, secCtx, bliObj))

	err = verifyBacklogItem(cmd, proc, bliObj, "BLI-1")
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Successfully verified backlog_item BLI-1")
}

func TestObject_RunVerify_BranchesAndExecution(t *testing.T) {
	cmd := NewValidateObjectCmd()
	proj, buf := prepareIsolatedCmd(t, cmd)
	require.NotNil(t, buf)
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	noHints := map[string]any{
		objects.FieldKeyKind:   objects.KindBacklogItem,
		objects.FieldKeyID:     "BLI-NOHINTS-01",
		objects.FieldKeyTitle:  "No Hints",
		objects.FieldKeyStatus: objects.ObjectStatusPlanned,
	}
	require.NoError(t, proj.FileStorage.Create(ctx, secCtx, noHints))

	err := runVerify(cmd, []string{"BLI-NOHINTS-01"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not contain machine_hints")

	badTypeHints := map[string]any{
		objects.FieldKeyKind:         objects.KindBacklogItem,
		objects.FieldKeyID:           "BLI-BADTYPE-01",
		objects.FieldKeyTitle:        "Bad Type Hints",
		objects.FieldKeyStatus:       objects.ObjectStatusPlanned,
		objects.FieldKeyMachineHints: 12345,
	}
	require.NoError(t, proj.FileStorage.Create(ctx, secCtx, badTypeHints))
	err = runVerify(cmd, []string{"BLI-BADTYPE-01"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported type")

	emptyHints := map[string]any{
		objects.FieldKeyKind:         objects.KindBacklogItem,
		objects.FieldKeyID:           "BLI-EMPTY-01",
		objects.FieldKeyTitle:        "Empty Hints",
		objects.FieldKeyStatus:       objects.ObjectStatusPlanned,
		objects.FieldKeyMachineHints: []string{},
	}
	require.NoError(t, proj.FileStorage.Create(ctx, secCtx, emptyHints))
	err = runVerify(cmd, []string{"BLI-EMPTY-01"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "machine_hints is empty")

	noCmdObj := map[string]any{
		objects.FieldKeyKind:         objects.KindBacklogItem,
		objects.FieldKeyID:           "BLI-NOCMD-01",
		objects.FieldKeyTitle:        "No CMD prefix",
		objects.FieldKeyStatus:       objects.ObjectStatusPlanned,
		objects.FieldKeyMachineHints: "plain text hint without prefix",
	}
	require.NoError(t, proj.FileStorage.Create(ctx, secCtx, noCmdObj))

	err = runVerify(cmd, []string{"BLI-NOCMD-01"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no executable command found")

	verifyObj := map[string]any{
		objects.FieldKeyKind:         objects.KindBacklogItem,
		objects.FieldKeyID:           "BLI-VERIFY-01",
		objects.FieldKeyTitle:        "Verify echo command",
		objects.FieldKeyStatus:       objects.ObjectStatusPlanned,
		objects.FieldKeyMachineHints: "VERIFY: echo ok",
	}
	require.NoError(t, proj.FileStorage.Create(ctx, secCtx, verifyObj))
	err = runVerify(cmd, []string{"BLI-VERIFY-01"})
	require.NoError(t, err)

	failObj := map[string]any{
		objects.FieldKeyKind:         objects.KindBacklogItem,
		objects.FieldKeyID:           "BLI-FAIL-01",
		objects.FieldKeyTitle:        "Failing command",
		objects.FieldKeyStatus:       objects.ObjectStatusPlanned,
		objects.FieldKeyMachineHints: []string{"CMD: false"},
	}
	require.NoError(t, proj.FileStorage.Create(ctx, secCtx, failObj))
	err = runVerify(cmd, []string{"BLI-FAIL-01"})
	require.Error(t, err)
}

func TestMatrix_Validate_AllOKFalse(t *testing.T) {
	cmd := NewMatrixValidateCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	tmp := t.TempDir()

	regDir := filepath.Join(tmp, ".zqk", "quality")
	require.NoError(t, fileutil.MkdirAll(regDir, 0755))

	csvBad := filepath.Join(regDir, "missing.csv")
	regContent := fmt.Sprintf("matrices:\n  broken_matrix:\n    csv: %s\n", csvBad)
	regPath := filepath.Join(regDir, "matrices.yaml")
	require.NoError(t, fileutil.WriteFile(regPath, []byte(regContent), 0644))

	cmd.SetArgs([]string{"--registry", regPath})
	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "one or more matrices failed checks")
}
