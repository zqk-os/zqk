package ops

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

type mockStorageProviderOps struct {
	storage.ObjectStorageProvider
	listFunc func(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter storage.ListFilter) (*storage.QueryResult, error)
	readFunc func(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error)
}

func (m *mockStorageProviderOps) List(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter storage.ListFilter) (*storage.QueryResult, error) {
	if m.listFunc != nil {
		return m.listFunc(ctx, secCtx, storageCtx, filter)
	}
	return &storage.QueryResult{}, nil
}

func (m *mockStorageProviderOps) Read(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
	if m.readFunc != nil {
		return m.readFunc(ctx, secCtx, id)
	}
	return nil, nil
}

func setupOpsIsolatedWorkspace(t *testing.T) (string, func()) {
	t.Helper()
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		Kind: "cmd.ops.test",
	})
	testRoot := proj.Root

	origDir, err := fileutil.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}

	if err := fileutil.Chdir(testRoot); err != nil {
		t.Fatalf("failed to chdir to test project root: %v", err)
	}

	origProjectRoot := zqkenv.ProjectRoot().Get()
	origTestRoot := zqkenv.TestRoot().Get()
	t.Setenv(zqkenv.ProjectRoot().Name(), testRoot)
	t.Setenv(zqkenv.TestRoot().Name(), testRoot)

	cleanup := func() {
		_ = fileutil.Chdir(origDir)
		if origProjectRoot != "" {
			t.Setenv(zqkenv.ProjectRoot().Name(), origProjectRoot)
		}
		if origTestRoot != "" {
			t.Setenv(zqkenv.TestRoot().Name(), origTestRoot)
		}
	}
	t.Cleanup(cleanup)

	return testRoot, cleanup
}

func TestOps_RootCommandTree(t *testing.T) {
	cmd := NewOpsCmd()
	if cmd == nil {
		t.Fatal("expected NewOpsCmd to return non-nil command")
	}
	if cmd.Use != "ops" {
		t.Errorf("expected command Use to be 'ops', got %q", cmd.Use)
	}

	subCommands := cmd.Commands()
	var hasLevelUp bool
	for _, sub := range subCommands {
		if strings.HasPrefix(sub.Use, "level-up") {
			hasLevelUp = true
			break
		}
	}
	if !hasLevelUp {
		t.Errorf("expected 'level-up' subcommand to be present under ops")
	}

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("failed to execute ops --help: %v", err)
	}
}

func TestOps_LoadSkillEvaluatorPrompt(t *testing.T) {
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	// 1. Nil storage returns fallback
	resNil := loadSkillEvaluatorPrompt(ctx, nil, secCtx)
	if resNil != skillEvaluatorFallbackPrompt {
		t.Errorf("expected fallback prompt for nil storage, got %q", resNil)
	}

	// 2. Storage with list error returns fallback
	mockErr := &mockStorageProviderOps{
		listFunc: func(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter storage.ListFilter) (*storage.QueryResult, error) {
			return nil, fmt.Errorf("list error")
		},
	}
	resErr := loadSkillEvaluatorPrompt(ctx, mockErr, secCtx)
	if resErr != skillEvaluatorFallbackPrompt {
		t.Errorf("expected fallback prompt on list error, got %q", resErr)
	}

	// 3. Storage with non-matching template returns fallback
	mockOther := &mockStorageProviderOps{
		listFunc: func(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter storage.ListFilter) (*storage.QueryResult, error) {
			return &storage.QueryResult{
				Objects: []map[string]any{
					{
						objects.FieldKeyTitle:      "Other Template",
						objects.FieldKeyPromptBody: "Some body",
					},
				},
			}, nil
		},
	}
	resOther := loadSkillEvaluatorPrompt(ctx, mockOther, secCtx)
	if resOther != skillEvaluatorFallbackPrompt {
		t.Errorf("expected fallback prompt for non-matching template, got %q", resOther)
	}

	// 4. Storage with matching template returns body
	mockMatch := &mockStorageProviderOps{
		listFunc: func(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter storage.ListFilter) (*storage.QueryResult, error) {
			return &storage.QueryResult{
				Objects: []map[string]any{
					{
						objects.FieldKeyTitle:      "Skill Evaluator Guidelines",
						objects.FieldKeyPromptBody: "Custom skill evaluator body prompt.",
					},
				},
			}, nil
		},
	}
	resMatch := loadSkillEvaluatorPrompt(ctx, mockMatch, secCtx)
	if resMatch != "Custom skill evaluator body prompt." {
		t.Errorf("expected custom prompt body, got %q", resMatch)
	}
}

func TestOps_RunLevelUp_ErrorsAndExecution(t *testing.T) {
	testRoot, _ := setupOpsIsolatedWorkspace(t)
	ctx := pkgctx.NewSystemContext()

	// 1. Storage not initialized
	cmdNoStorage := &cobra.Command{Use: "level-up"}
	errNoStorage := runLevelUp(cmdNoStorage, []string{"SKILL-001"})
	if errNoStorage == nil || !strings.Contains(errNoStorage.Error(), "storage provider not initialized") {
		t.Errorf("expected storage provider not initialized error, got %v", errNoStorage)
	}

	// Setup mock storage and register for projectRoot
	mockStore := &mockStorageProviderOps{
		readFunc: func(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
			switch id {
			case "SKILL-NOT-FOUND":
				return nil, fmt.Errorf("not found")
			case "SKILL-EMPTY-INSTR":
				return map[string]any{
					objects.FieldKeyInstructions: "",
				}, nil
			case "SKILL-VALID":
				return map[string]any{
					objects.FieldKeyInstructions: "High quality instructions for agent.",
				}, nil
			default:
				return nil, fmt.Errorf("unknown skill")
			}
		},
	}
	cli.RegisterStorageForProjectRoot(testRoot, mockStore)

	// 2. Skill not found
	cmdNotFound := NewLevelUpCmd()
	cmdNotFound.SetContext(ctx)
	cmdNotFound.SetArgs([]string{"SKILL-NOT-FOUND"})
	errNotFound := cmdNotFound.Execute()
	if errNotFound == nil || !strings.Contains(errNotFound.Error(), "failed to fetch agent_skill") {
		t.Errorf("expected failed to fetch error, got %v", errNotFound)
	}

	// 3. Skill with empty instructions
	cmdEmpty := NewLevelUpCmd()
	cmdEmpty.SetContext(ctx)
	cmdEmpty.SetArgs([]string{"SKILL-EMPTY-INSTR"})
	errEmpty := cmdEmpty.Execute()
	if errEmpty == nil || !strings.Contains(errEmpty.Error(), "does not contain valid instructions") {
		t.Errorf("expected invalid instructions error, got %v", errEmpty)
	}

	// 4. Valid skill execution
	cmdValid := NewLevelUpCmd()
	cmdValid.SetContext(ctx)
	var outBuf bytes.Buffer
	cmdValid.SetOut(&outBuf)
	cmdValid.SetErr(&outBuf)
	cmdValid.SetArgs([]string{"SKILL-VALID"})
	_ = cmdValid.Execute()
}
