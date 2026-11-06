package object

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"gopkg.in/yaml.v3"
)

func setupTestProjectForDraftPromote(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	// Setup required process dirs
	for _, sub := range []string{
		paths.ProcessInternalObjectSpecsDir,
		paths.ProcessInternalLifecyclesDir,
		paths.ProcessDir,
		filepath.Join(paths.ProcessDir, "requirements"),
		filepath.Join(paths.ProcessDir, "criteria"),
		filepath.Join(paths.ProcessDir, "backlog_items"),
		storage.ObjectDraftPlaneRoot(root),
	} {
		if err := fileutil.MkdirAll(filepath.Join(root, sub), paths.DirPerm755); err != nil {
			t.Fatal(err)
		}
	}

	return root
}

func TestPromoteObjectIDs_ContextCancellationGracefulStop(t *testing.T) {
	// Not t.Parallel(): SetupTestEnvironment uses process-global ZQK_TEST_ROOT.
	_ = SetupTestEnvironment(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-canceled context

	cmd := &cobra.Command{}
	cmd.SetContext(ctx)

	proc, err := cli.NewProcessor(cmd)
	if err != nil {
		t.Fatalf("failed to create processor: %v", err)
	}

	// When context is canceled, promoteObjectIDs should exit without infinite loops or unbounded panic
	err = promoteObjectIDs(cmd, proc, []string{"BLI-1", "BLI-2", "BLI-3"})
	if err == nil {
		t.Fatal("expected error on canceled context, got nil")
	}
	if !strings.Contains(err.Error(), "context") {
		t.Fatalf("expected context cancellation error, got: %v", err)
	}
}

func TestDraftPromote_AdaptiveBatchingDataStructures(t *testing.T) {
	t.Parallel()

	// Verify batch size and error bound constants
	if defaultDraftPromoteBatchSize != 10 {
		t.Fatalf("expected defaultDraftPromoteBatchSize=10, got %d", defaultDraftPromoteBatchSize)
	}
	if maxBoundedDraftErrors != 50 {
		t.Fatalf("expected maxBoundedDraftErrors=50, got %d", maxBoundedDraftErrors)
	}
}

func TestDraftPromote_TimeoutRuleRegistered(t *testing.T) {
	t.Parallel()

	root := paths.FindNearestProjectRoot(".")
	configPath := filepath.Join(root, "config", "command_timeouts.yaml")
	content, err := fileutil.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read command_timeouts.yaml: %v", err)
	}

	text := string(content)
	if !strings.Contains(text, "object draft promote") {
		t.Fatal("command_timeouts.yaml must contain rule for 'object draft promote'")
	}
	if !strings.Contains(text, "child_max_timeout_exempt: true") {
		t.Fatal("command_timeouts.yaml rule for 'object draft promote' must be child_max_timeout_exempt")
	}
}

func TestDraftPromote_PartialBatchPrunesSucceededDraft(t *testing.T) {
	// Not t.Parallel(): SetupTestEnvironment sets process-global ZQK_TEST_ROOT.
	env := SetupTestEnvironment(t)
	projectRoot := env.TestRoot

	stor, err := storage.NewFileObjectStorage(projectRoot)
	if err != nil {
		t.Fatalf("failed to init storage: %v", err)
	}

	secCtx := pkgctx.NewSystemSecurityContext()
	ctx := context.Background()

	// Item A: Valid criteria draft that will succeed promotion and materialize to CAS.
	idA := "CRIT-BATCH-SUCCEED-001"
	objA := map[string]any{
		objects.FieldKeyID:          idA,
		objects.FieldKeyKind:        objects.KindCriteria,
		objects.FieldKeyTitle:       "Valid Draft Acceptance Criteria",
		objects.FieldKeyDescription: "Substantive description for acceptance criteria verification.",
		"category":                  "acceptance",
		objects.FieldKeyStatus:      "conceptual",
	}
	if err := stor.Create(ctx, secCtx, objA); err != nil {
		t.Fatalf("failed to create draft item A: %v", err)
	}

	// Item B: Invalid criteria draft (missing required valid category / invalid transition) that fails promotion.
	idB := "CRIT-BATCH-FAIL-002"
	objB := map[string]any{
		objects.FieldKeyID:          idB,
		objects.FieldKeyKind:        objects.KindCriteria,
		objects.FieldKeyTitle:       "Invalid Draft Criteria Missing Category",
		objects.FieldKeyDescription: "Short", // fails min length (10 chars) for CAS membrane
		"category":                  "",
		objects.FieldKeyStatus:      "conceptual",
	}
	// Write directly to draft plane to simulate pre-existing unpromotable draft
	draftPathB := storage.ObjectDraftPlanePath(projectRoot, objects.KindCriteria, idB)
	_ = fileutil.MkdirAll(filepath.Dir(draftPathB), 0755)
	dataB, err := yaml.Marshal(objB)
	if err != nil {
		t.Fatalf("marshal objB: %v", err)
	}
	if err := fileutil.WriteFile(draftPathB, dataB, 0644); err != nil {
		t.Fatalf("failed to write draft item B: %v", err)
	}

	draftPathA := storage.ObjectDraftPlanePath(projectRoot, objects.KindCriteria, idA)
	if !storage.ObjectDraftPlaneExists(projectRoot, objects.KindCriteria, idA) {
		t.Fatalf("expected item A draft file to exist before promote")
	}
	if !storage.ObjectDraftPlaneExists(projectRoot, objects.KindCriteria, idB) {
		t.Fatalf("expected item B draft file to exist before promote")
	}

	cmd := NewDraftPromoteCmd()
	cmd.SetArgs([]string{"--all", "--dry-run=false"})
	promoteErr := cmd.Execute()
	if promoteErr == nil {
		t.Fatal("expected promote to report error due to item B failure, got nil")
	}

	// Verify Item A: successfully promoted to CAS, and its draft file is REMOVED from disk.
	if !storage.CasBackedObjectID(projectRoot, objects.KindCriteria, idA) {
		t.Fatalf("expected item A to be materialized in CAS")
	}
	if storage.ObjectDraftPlaneExists(projectRoot, objects.KindCriteria, idA) {
		t.Fatalf("expected item A draft file to be pruned from draft plane")
	}
	if _, statErr := fileutil.Stat(draftPathA); !fileutil.IsNotExist(statErr) {
		t.Fatalf("expected item A draft file at %s to be deleted from disk, but it still exists", draftPathA)
	}

	// Verify Item B: promotion failed, so it was NOT materialized in CAS, and draft file REMAINS on disk for correction.
	if storage.CasBackedObjectID(projectRoot, objects.KindCriteria, idB) {
		t.Fatalf("expected item B NOT to be materialized in CAS")
	}
	if !storage.ObjectDraftPlaneExists(projectRoot, objects.KindCriteria, idB) {
		t.Fatalf("expected item B draft file to remain on draft plane for correction")
	}
	if _, statErr := fileutil.Stat(draftPathB); statErr != nil {
		t.Fatalf("expected item B draft file to exist at %s, got: %v", draftPathB, statErr)
	}
}

func TestDraftPromote_CrossPlaneRequirementPruned(t *testing.T) {
	// Not t.Parallel(): SetupTestEnvironment sets process-global ZQK_TEST_ROOT.
	env := SetupTestEnvironment(t)
	projectRoot := env.TestRoot

	stor, err := storage.NewFileObjectStorage(projectRoot)
	if err != nil {
		t.Fatalf("failed to init storage: %v", err)
	}

	secCtx := pkgctx.NewSystemSecurityContext()
	ctx := context.Background()

	// Requirement A: Valid requirement that promotes and crosses CAS membrane
	idReq := "REQ-BATCH-SUCCEED-001"
	objReq := map[string]any{
		objects.FieldKeyID:          idReq,
		objects.FieldKeyKind:        objects.KindRequirement,
		objects.FieldKeyTitle:       "Valid Draft Requirement",
		objects.FieldKeyDescription: "Substantive requirement description satisfying all invariants.",
		"priority":                  "p1",
		objects.FieldKeyStatus:      "conceptual",
	}
	if err := stor.Create(ctx, secCtx, objReq); err != nil {
		t.Fatalf("failed to create draft requirement: %v", err)
	}

	// Item B: Failing draft that aborts batch promotion
	idB := "CRIT-BATCH-FAIL-003"
	objB := map[string]any{
		objects.FieldKeyID:          idB,
		objects.FieldKeyKind:        objects.KindCriteria,
		objects.FieldKeyTitle:       "Failing Draft Criteria",
		objects.FieldKeyDescription: "Short",
		"category":                  "",
		objects.FieldKeyStatus:      "conceptual",
	}
	draftPathB := storage.ObjectDraftPlanePath(projectRoot, objects.KindCriteria, idB)
	_ = fileutil.MkdirAll(filepath.Dir(draftPathB), 0755)
	dataB, err := yaml.Marshal(objB)
	if err != nil {
		t.Fatalf("marshal objB: %v", err)
	}
	if err := fileutil.WriteFile(draftPathB, dataB, 0644); err != nil {
		t.Fatalf("failed to write draft item B: %v", err)
	}

	draftPathReq := storage.ObjectDraftPlanePath(projectRoot, objects.KindRequirement, idReq)
	if !storage.ObjectDraftPlaneExists(projectRoot, objects.KindRequirement, idReq) {
		t.Fatalf("expected requirement draft file to exist before promote")
	}

	cmd := NewDraftPromoteCmd()
	cmd.SetArgs([]string{"--all", "--dry-run=false"})
	promoteErr := cmd.Execute()
	if promoteErr == nil {
		t.Fatal("expected promote error due to item B failure, got nil")
	}

	// Requirement must be materialized in CAS
	if !storage.CasBackedObjectID(projectRoot, objects.KindRequirement, idReq) {
		t.Fatalf("expected requirement %s to be in CAS", idReq)
	}

	// Requirement draft file must be pruned from disk
	if storage.ObjectDraftPlaneExists(projectRoot, objects.KindRequirement, idReq) {
		t.Fatalf("expected requirement draft file to be pruned from draft plane")
	}
	if _, statErr := fileutil.Stat(draftPathReq); !fileutil.IsNotExist(statErr) {
		t.Fatalf("expected requirement draft file at %s to be removed, but still exists", draftPathReq)
	}

	// Cross-plane lookup must confirm REQ is not draft plane only
	if storage.IsDraftPlaneOnly(projectRoot, objects.KindRequirement, idReq) {
		t.Fatalf("IsDraftPlaneOnly must report false for promoted requirement %s", idReq)
	}
}

func TestDraftPromote_PartialBatchFailure_PrunesSucceededDrafts(t *testing.T) {
	// Not t.Parallel(): SetupTestEnvironment sets process-global ZQK_TEST_ROOT.
	env := SetupTestEnvironment(t)
	projectRoot := env.TestRoot

	stor, err := storage.NewFileObjectStorage(projectRoot)
	if err != nil {
		t.Fatalf("failed to init storage: %v", err)
	}

	secCtx := pkgctx.NewSystemSecurityContext()
	ctx := context.Background()

	// Draft A: Valid criteria draft that will succeed promotion and materialize to CAS.
	idA := "CRIT-PARTIAL-SUCCEED-001"
	objA := map[string]any{
		objects.FieldKeyID:          idA,
		objects.FieldKeyKind:        objects.KindCriteria,
		objects.FieldKeyTitle:       "Valid Resilient Draft Acceptance Criteria",
		objects.FieldKeyDescription: "Substantive description for resilient acceptance criteria verification.",
		"category":                  "functional",
		objects.FieldKeyStatus:      "conceptual",
	}
	if err := stor.Create(ctx, secCtx, objA); err != nil {
		t.Fatalf("failed to create draft item A: %v", err)
	}

	// Draft B: Invalid criteria draft failing schema/preconditions (missing category, short description)
	idB := "CRIT-PARTIAL-FAIL-002"
	objB := map[string]any{
		objects.FieldKeyID:          idB,
		objects.FieldKeyKind:        objects.KindCriteria,
		objects.FieldKeyTitle:       "Invalid Criteria Missing Category and Short Description",
		objects.FieldKeyDescription: "Short",
		"category":                  "",
		objects.FieldKeyStatus:      "conceptual",
	}
	draftPathB := storage.ObjectDraftPlanePath(projectRoot, objects.KindCriteria, idB)
	_ = fileutil.MkdirAll(filepath.Dir(draftPathB), 0755)
	dataB, err := yaml.Marshal(objB)
	if err != nil {
		t.Fatalf("marshal objB: %v", err)
	}
	if err := fileutil.WriteFile(draftPathB, dataB, 0644); err != nil {
		t.Fatalf("failed to write draft item B: %v", err)
	}

	draftPathA := storage.ObjectDraftPlanePath(projectRoot, objects.KindCriteria, idA)
	if !storage.ObjectDraftPlaneExists(projectRoot, objects.KindCriteria, idA) {
		t.Fatalf("expected draft A file to exist before promote: %s", draftPathA)
	}
	if !storage.ObjectDraftPlaneExists(projectRoot, objects.KindCriteria, idB) {
		t.Fatalf("expected draft B file to exist before promote: %s", draftPathB)
	}

	// Execute draft promote across the batch
	cmd := NewDraftPromoteCmd()
	cmd.SetArgs([]string{"--all", "--dry-run=false"})
	promoteErr := cmd.Execute()
	if promoteErr == nil {
		t.Fatal("expected draft promote to return error due to draft B failure, got nil")
	}

	// Assert Draft A succeeded: materialized in CAS, removed from draft plane disk
	if !storage.CasBackedObjectID(projectRoot, objects.KindCriteria, idA) {
		t.Fatalf("expected draft A (%s) to be materialized in CAS storage", idA)
	}
	if storage.ObjectDraftPlaneExists(projectRoot, objects.KindCriteria, idA) {
		t.Fatalf("expected draft A (%s) to be pruned from draft plane", idA)
	}
	if _, statErr := fileutil.Stat(draftPathA); !fileutil.IsNotExist(statErr) {
		t.Fatalf("expected draft A file at %s to be deleted from disk, but it still exists", draftPathA)
	}

	// Assert Draft B failed: not in CAS, remains on draft plane disk for repair
	if storage.CasBackedObjectID(projectRoot, objects.KindCriteria, idB) {
		t.Fatalf("expected draft B (%s) NOT to be materialized in CAS storage", idB)
	}
	if !storage.ObjectDraftPlaneExists(projectRoot, objects.KindCriteria, idB) {
		t.Fatalf("expected draft B (%s) draft file to remain on draft plane for correction", idB)
	}
	if _, statErr := fileutil.Stat(draftPathB); statErr != nil {
		t.Fatalf("expected draft B file to exist at %s, got error: %v", draftPathB, statErr)
	}
}
