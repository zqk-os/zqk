package reports

// BLI-177483 inventory: setupTestProject in reports_test.go → testkit.PrepareIsolatedTempProject (RegisterTempProjectTeardown).

import (
	"errors"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

// createQuickTestCommand builds a cobra command with context and storage for testing quick report RunE.
// Uses ZQK_TEST_ROOT from setupTestProject so project root and storage point at the same test dir.
func createQuickTestCommand(t *testing.T, projectRoot string, storageProvider storage.ObjectStorageProvider) *cobra.Command {
	t.Helper()
	cmd := NewQuickCmd()
	// Set non-nil Go context so NewProcessor can get logger (GetLoggerFromContext(cmd.Context()))
	baseCtx := pkgctx.NewSystemContext()
	cmd.SetContext(cli.WithStorageProvider(baseCtx, storageProvider))

	initCtx := pkgctx.NewCliInitializationContext(cli.ResolveProjectRoot, ".")
	ctx, err := cli.GetContextFromCommand(cmd, initCtx)
	if err != nil {
		t.Fatalf("failed to create context: %v", err)
	}
	ctx.ProjectRoot = projectRoot
	ctx.Format = cli.FormatJSON
	cli.SetContext(cmd, ctx)
	return cmd
}

func TestRunQuick_UnknownPreset(t *testing.T) {
	// Not t.Parallel(): setupTestProject uses t.Setenv(ZQK_TEST_ROOT).
	projectRoot, storageProvider := setupTestProject(t)
	cmd := createQuickTestCommand(t, projectRoot, storageProvider)
	_ = cmd.Flags().Set("preset", "invalid")

	err := runQuick(cmd, nil)
	if err == nil {
		t.Fatal("expected error for unknown preset")
	}
	if err.Error() == emptyValue {
		t.Error("error message should be non-empty")
	}
}

func TestRunQuickQuestions_Empty(t *testing.T) {
	// Not t.Parallel(): setupTestProject uses t.Setenv(ZQK_TEST_ROOT).
	projectRoot, storageProvider := setupTestProject(t)
	cmd := createQuickTestCommand(t, projectRoot, storageProvider)
	_ = cmd.Flags().Set("preset", presetQuestions)
	_ = cmd.Flags().Set("format", quickFormatJSON)

	err := runQuick(cmd, nil)
	if err != nil {
		t.Fatalf("runQuick questions (empty): %v", err)
	}
}

func TestRunQuickQuestions_WithData(t *testing.T) {
	// Not t.Parallel(): setupTestProject uses t.Setenv(ZQK_TEST_ROOT).
	projectRoot, storageProvider := setupTestProject(t)
	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()

	// Create open question (unanswered)
	q := map[string]any{
		objects.FieldKeyID:            "QUE-TEST-1",
		objects.FieldKeyKind:          quickKindQuestion,
		objects.FieldKeySchemaVersion: quickSchemaV2,
		objects.FieldKeyStatus:        quickStatusOpen,
		objects.FieldKeyTitle:         "Test question",
		objects.FieldKeyQuestionText:  "What is the answer?",
		objects.FieldKeyUpdatedBy:     "ACC-TEST",
	}
	storage.CreateCASVisible(t, storageProvider, ctx, secCtx, q, "answered")

	cmd := createQuickTestCommand(t, projectRoot, storageProvider)
	_ = cmd.Flags().Set("preset", presetQuestions)
	_ = cmd.Flags().Set("format", quickFormatJSON)

	err := runQuick(cmd, nil)
	if err != nil {
		t.Fatalf("runQuick questions: %v", err)
	}
}

func TestRunQuickQuestions_StrictWithP0_ReturnsError(t *testing.T) {
	// Not t.Parallel(): setupTestProject uses t.Setenv(ZQK_TEST_ROOT).
	projectRoot, storageProvider := setupTestProject(t)
	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()

	q := map[string]any{
		objects.FieldKeyID:            "QUE-P0-1",
		objects.FieldKeyKind:          quickKindQuestion,
		objects.FieldKeySchemaVersion: quickSchemaV2,
		objects.FieldKeyStatus:        quickStatusOpen,
		objects.FieldKeyTitle:         "P0 question",
		objects.FieldKeyQuestionText:  "Critical?",
		objects.FieldKeyPriorityTier:  quickPriorityP0,
		objects.FieldKeyUpdatedBy:     "ACC-TEST",
	}
	storage.CreateCASVisible(t, storageProvider, ctx, secCtx, q, "answered")

	cmd := createQuickTestCommand(t, projectRoot, storageProvider)
	_ = cmd.Flags().Set("preset", presetQuestions)
	_ = cmd.Flags().Set("strict", "true")
	_ = cmd.Flags().Set("format", quickFormatJSON)

	err := runQuick(cmd, nil)
	if err == nil {
		t.Fatal("expected non-nil error when --strict and P0 question present")
	}
	var exitErr errExitCode
	if !errors.As(err, &exitErr) {
		t.Errorf("expected errExitCode, got %T: %v", err, err)
	}
}

func TestRunQuickMilestonesOverdue_Empty(t *testing.T) {
	// Not t.Parallel(): setupTestProject uses t.Setenv(ZQK_TEST_ROOT).
	projectRoot, storageProvider := setupTestProject(t)
	cmd := createQuickTestCommand(t, projectRoot, storageProvider)
	_ = cmd.Flags().Set("preset", presetMilestonesOverdue)
	_ = cmd.Flags().Set("format", quickFormatJSON)

	err := runQuick(cmd, nil)
	if err != nil {
		t.Fatalf("runQuick milestones-overdue (empty): %v", err)
	}
}

func TestRunQuickMilestonesOverdue_WithData(t *testing.T) {
	// Not t.Parallel(): setupTestProject uses t.Setenv(ZQK_TEST_ROOT).
	projectRoot, storageProvider := setupTestProject(t)
	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()

	// Milestone with target_date in the past (not complete)
	past := time.Now().UTC().AddDate(0, 0, -14).Format("2006-01-02")
	m := map[string]any{
		objects.FieldKeyID:            "MIL-TEST-1",
		objects.FieldKeyKind:          quickKindMilestone,
		objects.FieldKeySchemaVersion: quickSchemaV2,
		objects.FieldKeyStatus:        quickStatusInProgress,
		objects.FieldKeyTitle:         "Overdue milestone",
		objects.FieldKeyTargetDate:    past,
		objects.FieldKeyUpdatedBy:     "ACC-TEST",
		objects.FieldKeyCreatedAt:     "2026-01-01T00:00:00Z",
		objects.FieldKeyUpdatedAt:     "2026-01-01T00:00:00Z",
		objects.FieldKeyCreatedBy:     "ACC-TEST",
	}
	storage.CreateCASVisible(t, storageProvider, ctx, secCtx, m, quickStatusInProgress)

	cmd := createQuickTestCommand(t, projectRoot, storageProvider)
	_ = cmd.Flags().Set("preset", presetMilestonesOverdue)
	_ = cmd.Flags().Set("format", quickFormatJSON)

	err := runQuick(cmd, nil)
	if err != nil {
		t.Fatalf("runQuick milestones-overdue: %v", err)
	}
}

func TestErrExitCode_Error(t *testing.T) {
	t.Parallel()
	var e errExitCode = 1
	if e.Error() != "critical SLA exceeded" {
		t.Errorf("errExitCode.Error() = %q, want %q", e.Error(), "critical SLA exceeded")
	}
}

func TestNewQuickCmd_RequiresPreset(t *testing.T) {
	t.Parallel()
	cmd := NewQuickCmd()
	f := cmd.Flags().Lookup("preset")
	if f == nil {
		t.Fatal("expected --preset flag")
	}
	strict := cmd.Flags().Lookup("strict")
	if strict == nil {
		t.Fatal("expected --strict flag")
	}
	_ = f
}
