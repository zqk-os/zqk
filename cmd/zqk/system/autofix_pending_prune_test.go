package system

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

func TestIssueStillAppliesToObject_InvalidLifecycle(t *testing.T) {
	t.Parallel()
	issue := Issue{
		Tier:     1,
		Category: objects.KindLifecycle,
		Message:  "Invalid lifecycle status 'archived' for kind 'question'",
	}
	if !issueStillAppliesToObject("question", map[string]any{objects.FieldKeyStatus: "archived"}, issue) {
		t.Fatal("invalid archived should still apply")
	}
	if issueStillAppliesToObject("question", map[string]any{objects.FieldKeyStatus: "resolved"}, issue) {
		t.Fatal("resolved must not keep applying invalid-lifecycle snapshot")
	}
}

func TestIssueStillAppliesToObject_EmptyRefAndRequired(t *testing.T) {
	t.Parallel()
	emptyRef := Issue{Message: "team_configuration_ref: reference cannot be empty"}
	if !issueStillAppliesToObject("priority_plan", map[string]any{objects.FieldKeyTeamConfigurationRef: ""}, emptyRef) {
		t.Fatal("empty ref should still apply")
	}
	if issueStillAppliesToObject("priority_plan", map[string]any{objects.FieldKeyTeamConfigurationRef: "TC-1"}, emptyRef) {
		t.Fatal("filled ref must be stale")
	}

	required := Issue{Message: "title: Field title is required"}
	if !issueStillAppliesToObject("question", map[string]any{}, required) {
		t.Fatal("missing title should still apply")
	}
	if issueStillAppliesToObject("question", map[string]any{objects.FieldKeyTitle: "hi"}, required) {
		t.Fatal("present title must be stale")
	}
}

func TestIssueStillAppliesToObject_MissingRefUnlinked(t *testing.T) {
	t.Parallel()
	issue := Issue{
		Message: "Referenced object PRI-GONE (kind: priority_plan) in field priority_plan_ref does not exist in object cache",
	}
	if !issueStillAppliesToObject("backlog_item", map[string]any{
		objects.FieldKeyPriorityPlanRef: "PRI-GONE",
	}, issue) {
		t.Fatal("still linked missing ref should apply")
	}
	if issueStillAppliesToObject("backlog_item", map[string]any{
		objects.FieldKeyPriorityPlanRef: "PRI-OTHER",
	}, issue) {
		t.Fatal("unlinked / replaced ref must be stale")
	}
	if issueStillAppliesToObject("backlog_item", map[string]any{}, issue) {
		t.Fatal("absent field must be stale")
	}
}

func TestIssueStillAppliesToObject_EnumResolved(t *testing.T) {
	t.Parallel()
	issue := Issue{Message: "status: Field status must be one of: active, inactive, suspended"}
	if issueStillAppliesToObject("organization", map[string]any{objects.FieldKeyStatus: "active"}, issue) {
		t.Fatal("valid enum value must be stale")
	}
	if !issueStillAppliesToObject("organization", map[string]any{objects.FieldKeyStatus: "approved"}, issue) {
		t.Fatal("invalid enum value should still apply")
	}
}

func TestFilterStaleAutoFixBatchIssues(t *testing.T) {
	t.Parallel()
	issues := []AutoFixBatchIssue{
		{
			ObjectID:   "QUE-1",
			ObjectKind: "question",
			Issue: Issue{
				Category: objects.KindLifecycle,
				Message:  "Invalid lifecycle status 'archived' for kind 'question'",
			},
		},
		{
			ObjectID:   "QUE-1",
			ObjectKind: "question",
			Issue: Issue{
				Category: "instance_validation",
				Message:  "title: Field title is required",
			},
		},
	}
	live, stale := filterStaleAutoFixBatchIssues("question", map[string]any{
		objects.FieldKeyStatus: "resolved",
		objects.FieldKeyTitle:  "ok",
	}, issues)
	if len(stale) != 2 {
		t.Fatalf("stale=%d want 2 (lifecycle + required both resolved)", len(stale))
	}
	if len(live) != 0 {
		t.Fatalf("live=%d want 0", len(live))
	}
}

func TestPrunePendingAutofixBatchesForObjectID(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	autofixDir := filepath.Join(root, paths.ProjectDataDir, paths.AutofixDir)
	if err := fileutil.MkdirAll(autofixDir, 0o755); err != nil {
		t.Fatal(err)
	}
	batch := AutoFixBatch{
		BatchID:   "AUTOFIX-test",
		CreatedAt: time.Now().UTC(),
		Status:    autoFixBatchStatusPending,
		Objects: []AutoFixBatchObject{
			{ObjectID: "QUE-keep", ObjectKind: "question", Issues: []Issue{{Message: "a"}}},
			{ObjectID: "QUE-drop", ObjectKind: "question", Issues: []Issue{{Message: "b"}}},
		},
	}
	path := filepath.Join(autofixDir, "AUTOFIX-test.json")
	raw, err := json.MarshalIndent(batch, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	rewritten, deleted := PrunePendingAutofixBatchesForObjectID(root, "QUE-drop")
	if rewritten != 1 || deleted != 0 {
		t.Fatalf("rewritten=%d deleted=%d want 1,0", rewritten, deleted)
	}
	var got AutoFixBatch
	data, err := fileutil.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Objects) != 1 || got.Objects[0].ObjectID != "QUE-keep" {
		t.Fatalf("objects=%v", got.Objects)
	}

	rewritten, deleted = PrunePendingAutofixBatchesForObjectID(root, "QUE-keep")
	if rewritten != 0 || deleted != 1 {
		t.Fatalf("rewritten=%d deleted=%d want 0,1", rewritten, deleted)
	}
	if _, err := fileutil.Stat(path); !fileutil.IsNotExist(err) {
		t.Fatalf("expected batch file removed, err=%v", err)
	}
}

func TestMaybeClearStaleAutofixBatchesAfterLiveGreen(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	autofixDir := filepath.Join(root, paths.ProjectDataDir, paths.AutofixDir)
	if err := fileutil.MkdirAll(autofixDir, 0o755); err != nil {
		t.Fatal(err)
	}
	batchPath := filepath.Join(autofixDir, "AUTOFIX-stale.json")
	if err := fileutil.WriteFile(batchPath, []byte(`{"batch_id":"x","objects":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := &cobra.Command{Use: "check"}
	cmd.Flags().String("ids-from-file", "", "")
	_ = cmd.ParseFlags([]string{"all"})

	cleared := maybeClearStaleAutofixBatchesAfterLiveGreen(cmd, root, []CheckResult{
		{ObjectID: "QUE-1", Issues: nil},
	})
	if cleared != 1 {
		t.Fatalf("cleared=%d want 1", cleared)
	}
	if _, err := fileutil.Stat(batchPath); !fileutil.IsNotExist(err) {
		t.Fatalf("expected pending batch removed after full green, err=%v", err)
	}

	// Scoped check must not clear.
	if err := fileutil.WriteFile(batchPath, []byte(`{"batch_id":"y"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	cmdScoped := &cobra.Command{Use: "check"}
	cmdScoped.Flags().String("ids-from-file", "", "")
	_ = cmdScoped.ParseFlags([]string{"question"})
	cleared = maybeClearStaleAutofixBatchesAfterLiveGreen(cmdScoped, root, []CheckResult{})
	if cleared != 0 {
		t.Fatalf("scoped cleared=%d want 0", cleared)
	}
	if _, err := fileutil.Stat(batchPath); err != nil {
		t.Fatalf("scoped check must leave pending batch: %v", err)
	}
}

func TestSystemCheckIsFullKernelScan(t *testing.T) {
	t.Parallel()
	cmdAll := &cobra.Command{Use: "check"}
	cmdAll.Flags().String("ids-from-file", "", "")
	_ = cmdAll.ParseFlags([]string{"all"})
	if !systemCheckIsFullKernelScan(cmdAll) {
		t.Fatal("check all should be full")
	}

	cmdIDs := &cobra.Command{Use: "check"}
	cmdIDs.Flags().String("ids-from-file", "", "")
	_ = cmdIDs.ParseFlags([]string{"--ids-from-file", "x.txt"})
	if systemCheckIsFullKernelScan(cmdIDs) {
		t.Fatal("ids-from-file must not be full")
	}

	cmdKind := &cobra.Command{Use: "check"}
	cmdKind.Flags().String("ids-from-file", "", "")
	_ = cmdKind.ParseFlags([]string{"question"})
	if systemCheckIsFullKernelScan(cmdKind) {
		t.Fatal("kind-scoped must not be full")
	}
}

func TestParseMustBeOneOfValues(t *testing.T) {
	t.Parallel()
	got := parseMustBeOneOfValues("status: Field status must be one of: active, inactive, suspended")
	if len(got) != 3 || got[0] != "active" || !strings.Contains(strings.Join(got, ","), "suspended") {
		t.Fatalf("got %v", got)
	}
}
