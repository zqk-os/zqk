package whatsnext

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

func TestProjectStewardFocus_branchAhead(t *testing.T) {
	t.Parallel()
	got := ProjectStewardFocus(KernelAmbience{Available: true, ObjectComplianceOK: true, BranchAhead: 2}, "")
	if !strings.Contains(got, "branch_ahead=2") || !strings.Contains(got, "push seated") {
		t.Fatalf("expected push-ahead steward, got %q", got)
	}
}

func TestProjectStewardFocus_GhostRefSignal(t *testing.T) {
	t.Parallel()
	amb := KernelAmbience{Available: true, ObjectComplianceOK: true, GhostRefCount: 42}
	got := ProjectStewardFocus(amb, "")
	if !strings.Contains(got, "42 GhostRefs") || !strings.Contains(got, "heal-dangling") {
		t.Fatalf("expected ghost ref signal, got %q", got)
	}
}

func TestProjectStewardFocus_blockingAndDrafts(t *testing.T) {
	t.Parallel()
	amb := KernelAmbience{Available: true, BlockingIssues: 12, DraftPlaneTotal: 69, ObjectComplianceOK: false}
	got := ProjectStewardFocus(amb, "ack_then_continue")
	for _, want := range []string{"12 blocking", "draft plane 69", "inbox unacked", "POL-AGENT-DRAFT-SWEEP-TPM-001"} {
		if !strings.Contains(got, want) {
			t.Fatalf("focus %q missing %q", got, want)
		}
	}
}

func TestProjectStewardFocus_peerDraftNoSweep(t *testing.T) {
	t.Parallel()
	amb := KernelAmbience{
		Available:          true,
		DraftPlaneTotal:    3,
		ObjectComplianceOK: true,
		SeatMode:           SeatModePeerExecution,
	}
	got := ProjectStewardFocus(amb, "")
	if !strings.Contains(got, "sweep apply forbidden without RBAC") {
		t.Fatalf("peer focus must forbid sweep apply: %q", got)
	}
	if strings.Contains(got, "classify then sweep theater") {
		t.Fatalf("peer must not get legacy sweep theater cue: %q", got)
	}
}

func TestProjectStewardFocus_clean(t *testing.T) {
	t.Parallel()
	got := ProjectStewardFocus(KernelAmbience{Available: true, ObjectComplianceOK: true}, "")
	if !strings.Contains(got, "advance active PRI") {
		t.Fatalf("got %q", got)
	}
}

func TestPrependStewardProjection_idempotent(t *testing.T) {
	t.Parallel()
	once := PrependStewardProjection("continue", "12 blocking issues")
	if !strings.HasPrefix(once, stewardProjectionPrefix) {
		t.Fatalf("prefix: %q", once)
	}
	if !strings.Contains(once, "employed_workflows") {
		t.Fatalf("expected workflow ambient cue: %q", once)
	}
	twice := PrependStewardProjection(once, "12 blocking issues")
	if strings.Count(twice, stewardProjectionPrefix) != 1 {
		t.Fatalf("not idempotent: %q", twice)
	}
}

func TestProjectStewardFocus_employedWorkflows(t *testing.T) {
	t.Parallel()
	amb := KernelAmbience{
		Available:          true,
		ObjectComplianceOK: true,
		EmployedWorkflows: []EmployedWorkflowHint{
			{ID: "WFL-TPM-AGY-MESH-001", Title: "mesh", Source: "active"},
			{ID: "WFL-SUBAGENT-DISPATCH", Title: "subagent", Source: "related"},
		},
	}
	got := ProjectStewardFocus(amb, "")
	for _, want := range []string{"employed workflows (2)", "WFL-TPM-AGY-MESH-001", "WFL-SUBAGENT-DISPATCH"} {
		if !strings.Contains(got, want) {
			t.Fatalf("focus %q missing %q", got, want)
		}
	}
}

func TestTruncateAndFixtureHelpers(t *testing.T) {
	t.Parallel()
	if truncateRunes("abcd", 3) != "ab…" {
		t.Fatalf("truncate")
	}
	if !isFixtureWorkflow(map[string]any{objects.FieldKeyTitle: "Fixture: workflow #1"}, "WFL-001") {
		t.Fatal("expected fixture")
	}
	if !isCoreOperationalWorkflow("WFL-TPM-AGY-MESH-001") {
		t.Fatal("core mesh")
	}
}

func TestEnrichStrategicAlignment_andSeatMode(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	dir := filepath.Join(root, paths.ProjectDataDir, "state", "ambient")
	if err := fileutil.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	payload := map[string]any{
		"overall_alignment_score": 91.5,
		"goals_count":             12,
		"goal_work": map[string]any{
			"items_with_goals":    90,
			"items_without_goals": 10,
		},
	}
	b, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteFile(filepath.Join(dir, "align-latest.json"), b, 0o644); err != nil {
		t.Fatal(err)
	}
	amb := KernelAmbience{Available: true, ObjectComplianceOK: true}
	EnrichStrategicAlignment(&amb, root)
	if amb.StrategicAlignment == nil || !amb.StrategicAlignment.Available || amb.StrategicAlignment.AlignmentScore != 91.5 {
		t.Fatalf("align=%+v", amb.StrategicAlignment)
	}
	ApplySeatOperatingMode(&amb, "peer-tpm-01", "")
	if amb.SeatMode != SeatModeTPMProcessAdmin {
		t.Fatalf("seat=%q", amb.SeatMode)
	}
	if !strings.Contains(amb.StewardFocus, "TPM seat") || !strings.Contains(amb.StewardFocus, "align score") {
		t.Fatalf("focus=%q", amb.StewardFocus)
	}
	if !strings.Contains(amb.StewardFocus, "do not remint align") {
		t.Fatalf("fresh align cache must not tell TPM to remint: %q", amb.StewardFocus)
	}
	ApplySeatOperatingMode(&amb, "peer-agent-1", "")
	if amb.SeatMode != SeatModePeerExecution {
		t.Fatalf("peer seat=%q", amb.SeatMode)
	}
	got := PrependStewardProjectionForSeat("continue", amb.StewardFocus, amb.SeatMode)
	if !strings.Contains(got, "Peer: run zqk workflow whats-next") {
		t.Fatalf("prepend=%q", got)
	}
}

func TestLoadKernelAmbience_fromCacheAndDrafts(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	logDir := filepath.Join(root, paths.ProjectDataDir, "logs")
	if err := fileutil.MkdirAll(logDir, 0o755); err != nil {
		t.Fatal(err)
	}
	payload := map[string]any{
		objects.FieldKeySummary: map[string]any{
			"total_objects": 100, "total_issues": 5, "blocking_issues": 2,
			"warnings": 1, "pending_autofix_batches": 0,
		},
	}
	b, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteFile(filepath.Join(logDir, "system-check.json"), b, 0o644); err != nil {
		t.Fatal(err)
	}
	draft := filepath.Join(root, paths.ProjectDataDir, paths.ObjectDraftsDir, "backlog_item", "aa")
	if err := fileutil.MkdirAll(draft, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteFile(filepath.Join(draft, "BLI-test.yaml"), []byte("id: BLI-test\nkind: backlog_item\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	amb := LoadKernelAmbience(root)
	if !amb.Available || amb.BlockingIssues != 2 {
		t.Fatalf("amb=%+v", amb)
	}
	if amb.DraftPlaneTotal != 1 {
		t.Fatalf("drafts=%d", amb.DraftPlaneTotal)
	}
	if amb.StewardFocus == "" || !strings.Contains(amb.StewardFocus, "2 blocking") {
		t.Fatalf("focus=%q", amb.StewardFocus)
	}
}
