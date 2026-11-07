package newcmd

import (
	"bytes"
	"context"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/coordination"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testenvroot"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func setupRelationalFlagsTestEnv(t *testing.T) string {
	t.Helper()
	testRoot := t.TempDir()
	t.Setenv(zqkenv.TestRoot().Name(), testRoot)
	repoRoot := zqkenv.ProjectRoot().Get()
	if repoRoot == "" {
		if root, err := findModuleRoot(); err == nil {
			repoRoot = root
		}
	}
	if err := testenvroot.BootstrapRoot(testRoot, repoRoot); err != nil {
		t.Fatalf("BootstrapRoot: %v", err)
	}
	t.Cleanup(func() {
		if c, ok := coordination.GetCoordinator().(*coordination.Coordinator); ok && c != nil {
			_ = c.Drain(5 * time.Second)
		}
		if teardownErr := storage.RunProjectTestTeardown(storage.TempProjectTeardown(testRoot, nil)); teardownErr != nil {
			t.Logf("teardown error: %v", teardownErr)
		}
		zqkDir := filepath.Join(testRoot, paths.ProjectDataDir)
		for i := 0; i < 20; i++ {
			if _, err := fileutil.Stat(zqkDir); err != nil {
				break
			}
			_ = fileutil.RemoveAll(zqkDir)
			if _, err := fileutil.Stat(zqkDir); err != nil {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
	})
	return testRoot
}

func readLatestDraft(t *testing.T, testRoot, kind string) map[string]any {
	t.Helper()
	draftFiles, err := storage.ListObjectDraftPlaneIDs(testRoot, kind)
	if err != nil || len(draftFiles) == 0 {
		t.Fatalf("expected drafted %s on draft plane, got err=%v count=%d", kind, err, len(draftFiles))
	}
	lastID := draftFiles[len(draftFiles)-1]
	data, err := fileutil.ReadFile(storage.ObjectDraftPlanePath(testRoot, kind, lastID))
	if err != nil {
		t.Fatalf("failed to read drafted %s (%s): %v", kind, lastID, err)
	}
	var obj map[string]any
	if err := yaml.Unmarshal(data, &obj); err != nil {
		t.Fatalf("unmarshal drafted %s: %v", kind, err)
	}
	return obj
}

func toStringSlice(val any) []string {
	if val == nil {
		return nil
	}
	switch v := val.(type) {
	case []string:
		return v
	case []any:
		var result []string
		for _, it := range v {
			if s, ok := it.(string); ok {
				result = append(result, s)
			}
		}
		return result
	default:
		return nil
	}
}

func TestNew_RelationalFlags_BacklogItemWithReq(t *testing.T) {
	testRoot := setupRelationalFlagsTestEnv(t)

	var buf bytes.Buffer
	ctx := pkgctx.WithCommandOutputWriter(context.Background(), &buf)

	cmd := NewNewCmd()
	cmd.SetContext(ctx)
	cmd.SetArgs([]string{"backlog_item", "--title", "Feature Work", "--req", "REQ-001,REQ-002"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("new backlog_item failed: %v", err)
	}

	obj := readLatestDraft(t, testRoot, objects.KindBacklogItem)
	reqRefs := toStringSlice(obj[objects.FieldKeyRequirementRefs])
	if !slices.Contains(reqRefs, "REQ-001") || !slices.Contains(reqRefs, "REQ-002") {
		t.Fatalf("expected requirement_refs to contain REQ-001 and REQ-002, got %v", reqRefs)
	}
}

func TestNew_RelationalFlags_RequirementWithGoal(t *testing.T) {
	testRoot := setupRelationalFlagsTestEnv(t)

	var buf bytes.Buffer
	ctx := pkgctx.WithCommandOutputWriter(context.Background(), &buf)

	cmd := NewNewCmd()
	cmd.SetContext(ctx)
	cmd.SetArgs([]string{"requirement", "--title", "Core Architecture Spec", "--goal", "GOAL-ROOT-001"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("new requirement failed: %v", err)
	}

	obj := readLatestDraft(t, testRoot, objects.KindRequirement)
	goalRefs := toStringSlice(obj[objects.FieldKeyGoalRefs])
	if !slices.Contains(goalRefs, "GOAL-ROOT-001") {
		t.Fatalf("expected goal_refs to contain GOAL-ROOT-001, got %v", goalRefs)
	}
}

func TestNew_RelationalFlags_TestCaseWithCriteriaAndBli(t *testing.T) {
	testRoot := setupRelationalFlagsTestEnv(t)

	var buf bytes.Buffer
	ctx := pkgctx.WithCommandOutputWriter(context.Background(), &buf)

	cmd := NewNewCmd()
	cmd.SetContext(ctx)
	cmd.SetArgs([]string{"test_case", "--title", "Integration Verification Test", "--criteria", "CRIT-001", "--bli", "BLI-001"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("new test_case failed: %v", err)
	}

	obj := readLatestDraft(t, testRoot, objects.KindTestCase)
	critRefs := toStringSlice(obj[objects.FieldKeyCriteriaRefs])
	if !slices.Contains(critRefs, "CRIT-001") {
		t.Fatalf("expected criteria_refs to contain CRIT-001, got %v", critRefs)
	}
	bliRefs := toStringSlice(obj[objects.FieldKeyBacklogItemRefs])
	if !slices.Contains(bliRefs, "BLI-001") {
		t.Fatalf("expected backlog_item_refs to contain BLI-001, got %v", bliRefs)
	}
}

func TestNew_RelationalFlags_NewObjectSubcommand(t *testing.T) {
	testRoot := setupRelationalFlagsTestEnv(t)

	var buf bytes.Buffer
	ctx := pkgctx.WithCommandOutputWriter(context.Background(), &buf)

	cmd := NewNewCmd()
	cmd.SetContext(ctx)
	cmd.SetArgs([]string{
		"object", "backlog_item",
		"--title", "Draft BLI via new object",
		"--plan", "PRI-001",
		"--req", "REQ-100",
		"--goal", "GOAL-100",
		"--criteria", "CRIT-100",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("new object backlog_item failed: %v", err)
	}

	obj := readLatestDraft(t, testRoot, objects.KindBacklogItem)
	if planRef, _ := obj[objects.FieldKeyPriorityPlanRef].(string); planRef != "PRI-001" {
		t.Fatalf("expected priority_plan_ref 'PRI-001', got %q", planRef)
	}
	reqRefs := toStringSlice(obj[objects.FieldKeyRequirementRefs])
	if !slices.Contains(reqRefs, "REQ-100") {
		t.Fatalf("expected requirement_refs to contain REQ-100, got %v", reqRefs)
	}
	goalRefs := toStringSlice(obj[objects.FieldKeyGoalRefs])
	if !slices.Contains(goalRefs, "GOAL-100") {
		t.Fatalf("expected goal_refs to contain GOAL-100, got %v", goalRefs)
	}
	critRefs := toStringSlice(obj[objects.FieldKeyCriteriaRefs])
	if !slices.Contains(critRefs, "CRIT-100") {
		t.Fatalf("expected criteria_refs to contain CRIT-100, got %v", critRefs)
	}
}

func TestNew_RelationalFlags_InheritFromPriorityPlan(t *testing.T) {
	testRoot := setupRelationalFlagsTestEnv(t)

	// Seed a priority_plan in storage
	planID := "PRI-TEST-INHERIT-001"
	seedPlan := map[string]any{
		objects.FieldKeyKind:           objects.KindPriorityPlan,
		objects.FieldKeyID:             planID,
		objects.FieldKeyTitle:          "Test Priority Plan",
		objects.FieldKeyDescription:    "Valid priority plan description for testing inheritance",
		objects.FieldKeyStatus:         "in_progress",
		objects.FieldKeyWorkstreamRefs: []string{"WS-INHERIT-001"},
		objects.FieldKeyGoalRefs:       []string{"GOAL-INHERIT-001"},
		objects.FieldKeyNamespaceID:    "zqk:kernel",
		objects.FieldKeySchemaVersion:  "2.0.0",
	}

	sp, err := storage.NewFileObjectStorage(testRoot)
	if err != nil {
		t.Fatalf("create storage: %v", err)
	}
	secCtx := pkgctx.NewSystemSecurityContext()
	ctx := pkgctx.WithPromoteOnCreate(context.Background())
	if err := sp.Create(ctx, secCtx, seedPlan); err != nil {
		t.Fatalf("seed priority plan: %v", err)
	}

	var buf bytes.Buffer
	cmdCtx := pkgctx.WithCommandOutputWriter(context.Background(), &buf)

	cmd := NewNewCmd()
	cmd.SetContext(cmdCtx)
	cmd.SetArgs([]string{"backlog_item", "--title", "Child BLI", "--plan", planID})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("new backlog_item failed: %v", err)
	}

	obj := readLatestDraft(t, testRoot, objects.KindBacklogItem)
	if planRef, _ := obj[objects.FieldKeyPriorityPlanRef].(string); planRef != planID {
		t.Fatalf("expected priority_plan_ref %q, got %q", planID, planRef)
	}
	wsRefs := toStringSlice(obj[objects.FieldKeyWorkstreamRefs])
	if !slices.Contains(wsRefs, "WS-INHERIT-001") {
		t.Fatalf("expected inherited workstream_refs to contain WS-INHERIT-001, got %v", wsRefs)
	}
	goalRefs := toStringSlice(obj[objects.FieldKeyGoalRefs])
	if !slices.Contains(goalRefs, "GOAL-INHERIT-001") {
		t.Fatalf("expected inherited goal_refs to contain GOAL-INHERIT-001, got %v", goalRefs)
	}
}

func TestNew_RelationalFlags_InvariantsForbiddenFieldsFiltered(t *testing.T) {
	testRoot := setupRelationalFlagsTestEnv(t)

	var buf bytes.Buffer
	ctx := pkgctx.WithCommandOutputWriter(context.Background(), &buf)

	// Goal must NOT have priority_plan_ref or criteria_refs
	cmd := NewNewCmd()
	cmd.SetContext(ctx)
	cmd.SetArgs([]string{"goal", "--title", "Timeless Strategic Compass", "--plan", "PRI-001", "--criteria", "CRIT-001"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("new goal failed: %v", err)
	}

	goalObj := readLatestDraft(t, testRoot, objects.KindGoal)
	if _, hasPlan := goalObj[objects.FieldKeyPriorityPlanRef]; hasPlan {
		t.Fatalf("goal must not declare priority_plan_ref")
	}
	if _, hasCrit := goalObj[objects.FieldKeyCriteriaRefs]; hasCrit {
		t.Fatalf("goal must not declare criteria_refs")
	}

	// Requirement must NOT have priority_plan_ref
	buf.Reset()
	cmd = NewNewCmd()
	cmd.SetContext(ctx)
	cmd.SetArgs([]string{"requirement", "--title", "Timeless Feature Contract", "--plan", "PRI-001"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("new requirement failed: %v", err)
	}

	reqObj := readLatestDraft(t, testRoot, objects.KindRequirement)
	if _, hasPlan := reqObj[objects.FieldKeyPriorityPlanRef]; hasPlan {
		t.Fatalf("requirement must not declare priority_plan_ref")
	}
}

const (
	relFlagsTestReqID  = "REQ-RELFLAGS-001"
	relFlagsTestGoalID = "GOAL-RELFLAGS-001"
)

// Negative boundary: the --req/--goal lineage flags must not bypass the
// per-kind forbidden-field filter. Criteria refuse requirement_refs
// (requirements own criteria_refs), so `zqk new criteria --req` must not
// persist requirement_refs onto the draft.
func TestNewCriteria_ReqFlagDoesNotPersistForbiddenRequirementRefs(t *testing.T) {
	testRoot := setupRelationalFlagsTestEnv(t)

	var buf bytes.Buffer
	ctx := pkgctx.WithCommandOutputWriter(context.Background(), &buf)

	cmd := NewNewCmd()
	cmd.SetContext(ctx)
	cmd.SetArgs([]string{objects.KindCriteria, "--title", "Forbidden Ref Crit", "--req", relFlagsTestReqID, "--goal", relFlagsTestGoalID})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("new criteria failed: %v", err)
	}

	obj := readLatestDraft(t, testRoot, objects.KindCriteria)
	if refs, has := obj[objects.FieldKeyRequirementRefs]; has {
		t.Fatalf("criteria must not declare requirement_refs, got %v", refs)
	}
	if goalRefs := toStringSlice(obj[objects.FieldKeyGoalRefs]); !slices.Contains(goalRefs, relFlagsTestGoalID) {
		t.Fatalf("expected criteria goal_refs to contain %s, got %v", relFlagsTestGoalID, goalRefs)
	}
}

// Operational proof: lineage flags persist on test_case drafts via the
// generic `new object` path, without duplicate refs.
func TestNewObjectTestCase_ReqFlagPersistsRequirementRefs(t *testing.T) {
	testRoot := setupRelationalFlagsTestEnv(t)

	var buf bytes.Buffer
	ctx := pkgctx.WithCommandOutputWriter(context.Background(), &buf)

	cmd := NewNewCmd()
	cmd.SetContext(ctx)
	cmd.SetArgs([]string{"object", objects.KindTestCase, "--title", "Lineage TC", "--req", relFlagsTestReqID})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("new object test_case failed: %v", err)
	}

	obj := readLatestDraft(t, testRoot, objects.KindTestCase)
	reqRefs := toStringSlice(obj[objects.FieldKeyRequirementRefs])
	if len(reqRefs) != 1 || reqRefs[0] != relFlagsTestReqID {
		t.Fatalf("expected requirement_refs=[%s] (no duplicates), got %v", relFlagsTestReqID, reqRefs)
	}
}
