package agent

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestDispositionForOrchestrationStatus(t *testing.T) {
	t.Parallel()
	tests := []struct {
		status string
		want   orchDisposition
	}{
		{objects.ObjectStatusError, orchDispositionReplace},
		{objects.ObjectStatusFailed, orchDispositionReplace},
		{objects.ObjectStatusInProgress, orchDispositionReuse},
		{objects.ObjectStatusApproved, orchDispositionReuse},
		{objects.ObjectStatusProposed, orchDispositionReuse},
		{objects.ObjectStatusImplemented, orchDispositionSkipDone},
		{objects.ObjectStatusArchived, orchDispositionSkipDone},
		{objects.ObjectStatusPendingVerification, orchDispositionSkipDone},
		{objects.ObjectStatusComplete, orchDispositionSkipDone},
		{"", orchDispositionMint},
		{"unknown_live", orchDispositionReuse},
	}
	for _, tc := range tests {
		t.Run(tc.status, func(t *testing.T) {
			t.Parallel()
			if got := dispositionForOrchestrationStatus(tc.status); got != tc.want {
				t.Fatalf("disposition(%q)=%d want %d", tc.status, got, tc.want)
			}
		})
	}
}

func TestOrchDispatchSkipReason(t *testing.T) {
	t.Parallel()
	if got := orchDispatchSkipReason(nil); got != "" {
		t.Fatalf("empty surface must allow mint, got %q", got)
	}
	dead := []map[string]any{
		{objects.FieldKeyStatus: objects.ObjectStatusError},
		{objects.FieldKeyStatus: objects.ObjectStatusImplemented},
	}
	if got := orchDispatchSkipReason(dead); got != "no_live_atk" {
		t.Fatalf("terminal-only got %q", got)
	}
	live := []map[string]any{
		{objects.FieldKeyStatus: objects.ObjectStatusError},
		{objects.FieldKeyStatus: objects.ObjectStatusInProgress},
	}
	if got := orchDispatchSkipReason(live); got != "" {
		t.Fatalf("live ATK must allow, got %q", got)
	}
}

func TestPickExistingOrchestrationTask_reusesInProgressByPlanRef(t *testing.T) {
	t.Parallel()
	tasks := []map[string]any{
		{
			objects.FieldKeyID:              "ATK-OLD-DONE",
			objects.FieldKeyTitle:           "Execute Task: Wire gate",
			objects.FieldKeyPriorityPlanRef: "PIP-LEAD",
			objects.FieldKeyStatus:          objects.ObjectStatusImplemented,
			objects.FieldKeyUpdatedAt:       "2026-01-01T00:00:00Z",
		},
		{
			objects.FieldKeyID:              "ATK-LIVE",
			objects.FieldKeyTitle:           "Execute Task: Wire gate",
			objects.FieldKeyPriorityPlanRef: "PIP-LEAD",
			objects.FieldKeyStatus:          objects.ObjectStatusInProgress,
			objects.FieldKeyUpdatedAt:       "2026-08-26T00:00:00Z",
		},
	}
	id, status, disp := pickExistingOrchestrationTask(tasks, "Wire gate", "PIP-LEAD", "")
	if disp != orchDispositionReuse || id != "ATK-LIVE" || status != objects.ObjectStatusInProgress {
		t.Fatalf("got id=%s status=%s disp=%d", id, status, disp)
	}
}

func TestPickExistingOrchestrationTask_prefersApprovedOverLeftoverError(t *testing.T) {
	t.Parallel()
	tasks := []map[string]any{
		{
			objects.FieldKeyID:              "ATK-LEFTOVER-ERR",
			objects.FieldKeyTitle:           "Execute Task: Model code location",
			objects.FieldKeyPriorityPlanRef: "PRI-R20",
			objects.FieldKeyStatus:          objects.ObjectStatusError,
			objects.FieldKeyUpdatedAt:       "2026-08-26T10:37:00Z",
		},
		{
			objects.FieldKeyID:              "ATK-ASSIGNED",
			objects.FieldKeyTitle:           "Execute Task: Model code location",
			objects.FieldKeyPriorityPlanRef: "PRI-R20",
			objects.FieldKeyStatus:          objects.ObjectStatusApproved,
			objects.FieldKeyClaimedBy:       "antigravity-1",
			objects.FieldKeyUpdatedAt:       "2026-08-26T06:00:00Z",
		},
		{
			objects.FieldKeyID:              "ATK-UNCLAIMED-WIP",
			objects.FieldKeyTitle:           "Execute Task: Model code location",
			objects.FieldKeyPriorityPlanRef: "PRI-R20",
			objects.FieldKeyStatus:          objects.ObjectStatusInProgress,
			objects.FieldKeyUpdatedAt:       "2026-08-26T10:36:00Z",
		},
	}
	id, status, disp := pickExistingOrchestrationTask(tasks, "Model code location", "PRI-R20", "")
	if disp != orchDispositionReuse || id != "ATK-ASSIGNED" || status != objects.ObjectStatusApproved {
		t.Fatalf("got id=%s status=%s disp=%d", id, status, disp)
	}
}

func TestPickExistingOrchestrationTask_matchesPipelineOrPlanRef(t *testing.T) {
	t.Parallel()
	tasks := []map[string]any{
		{
			objects.FieldKeyID:              "ATK-ERR",
			objects.FieldKeyTitle:           "Wire gate",
			objects.FieldKeyPriorityPlanRef: "PIP-LEAD",
			objects.FieldKeyStatus:          objects.ObjectStatusError,
		},
	}
	id, _, disp := pickExistingOrchestrationTask(tasks, "Wire gate", "PIP-LEAD", "")
	if disp != orchDispositionReplace || id != "ATK-ERR" {
		t.Fatalf("plan_ref match: id=%s disp=%d", id, disp)
	}
	id, _, disp = pickExistingOrchestrationTask([]map[string]any{{
		objects.FieldKeyID:          "ATK-PIPE",
		objects.FieldKeyTitle:       "Execute Task: Wire gate",
		objects.FieldKeyPipelineRef: "PIP-LEAD",
		objects.FieldKeyStatus:      objects.ObjectStatusError,
	}}, "Wire gate", "PIP-LEAD", "")
	if disp != orchDispositionReplace || id != "ATK-PIPE" {
		t.Fatalf("pipeline_ref match: id=%s disp=%d", id, disp)
	}
}

func TestPickExistingOrchestrationTask_reusesExecuteBLITitleAndRef(t *testing.T) {
	t.Parallel()
	bli := "BLI-LAUNCH-REQ-HONESTY-001"
	tasks := []map[string]any{
		{
			objects.FieldKeyID:              "ATK-DRAFT-EXEC",
			objects.FieldKeyTitle:           "Execute " + bli,
			objects.FieldKeyBacklogItemRef:  bli,
			objects.FieldKeyPriorityPlanRef: "PRI-CEF-LINGERING-INTAKE-001",
			objects.FieldKeyStatus:          objects.ObjectStatusProposed,
		},
	}
	id, status, disp := pickExistingOrchestrationTask(tasks, "Requirements honesty", "PRI-CEF-LINGERING-INTAKE-001", bli)
	if disp != orchDispositionReuse || id != "ATK-DRAFT-EXEC" || status != objects.ObjectStatusProposed {
		t.Fatalf("got id=%s status=%s disp=%d", id, status, disp)
	}
}

func TestPickExistingOrchestrationTask_implementedBeatsErrorDebris(t *testing.T) {
	t.Parallel()
	tasks := []map[string]any{
		{
			objects.FieldKeyID:              "ATK-ERR",
			objects.FieldKeyTitle:           "Execute Task: Wire gate",
			objects.FieldKeyPriorityPlanRef: "PIP-LEAD",
			objects.FieldKeyStatus:          objects.ObjectStatusError,
			objects.FieldKeyUpdatedAt:       "2026-09-03T00:00:00Z",
		},
		{
			objects.FieldKeyID:              "ATK-DONE",
			objects.FieldKeyTitle:           "Execute Task: Wire gate",
			objects.FieldKeyPriorityPlanRef: "PIP-LEAD",
			objects.FieldKeyStatus:          objects.ObjectStatusImplemented,
			objects.FieldKeyUpdatedAt:       "2026-09-02T00:00:00Z",
		},
	}
	id, status, disp := pickExistingOrchestrationTask(tasks, "Wire gate", "PIP-LEAD", "")
	if disp != orchDispositionSkipDone || id != "ATK-DONE" || status != objects.ObjectStatusImplemented {
		t.Fatalf("got id=%s status=%s disp=%d", id, status, disp)
	}
}

func TestRecoverAgentTaskListFilters(t *testing.T) {
	t.Parallel()
	got := recoverAgentTaskListFilters("PRI-LEAD")
	if len(got) != 4 {
		t.Fatalf("len=%d want 4", len(got))
	}
	seen := map[string]bool{}
	for _, f := range got {
		if f.Kind != objects.KindAgentTask {
			t.Fatalf("kind=%s", f.Kind)
		}
		var key string
		if _, ok := f.Filters[objects.FieldKeyPriorityPlanRef]; ok {
			key = objects.FieldKeyPriorityPlanRef
		} else if _, ok := f.Filters[objects.FieldKeyPipelineRef]; ok {
			key = objects.FieldKeyPipelineRef
		} else {
			t.Fatal("missing plan key")
		}
		if f.Filters[key] != "PRI-LEAD" {
			t.Fatalf("plan=%v", f.Filters[key])
		}
		st, _ := f.Filters[objects.FieldKeyStatus].(string)
		if st != objects.ObjectStatusError && st != objects.ObjectStatusFailed {
			t.Fatalf("status=%s", st)
		}
		seen[key+"|"+st] = true
	}
	if !seen[objects.FieldKeyPriorityPlanRef+"|"+objects.ObjectStatusError] {
		t.Fatal("missing priority_plan_ref + error")
	}
	if seen[objects.FieldKeyPipelineRef+"|"+objects.ObjectStatusFailed] == false {
		t.Fatal("missing pipeline_ref + failed")
	}
}

func TestRecoverAgentTaskListFilters_notFailedOnly(t *testing.T) {
	t.Parallel()
	for _, f := range recoverAgentTaskListFilters("PRI-LEAD") {
		if f.Filters[objects.FieldKeyStatus] == objects.ObjectStatusFailed &&
			f.Filters[objects.FieldKeyPipelineRef] == "PRI-LEAD" &&
			len(f.Filters) == 2 {
			// allowed as one of four; the bug was this being the *only* filter
			continue
		}
	}
	var errorPlanRef bool
	for _, f := range recoverAgentTaskListFilters("PRI-LEAD") {
		if f.Filters[objects.FieldKeyStatus] == objects.ObjectStatusError &&
			f.Filters[objects.FieldKeyPriorityPlanRef] == "PRI-LEAD" {
			errorPlanRef = true
		}
	}
	if !errorPlanRef {
		t.Fatal("recover must query status=error by priority_plan_ref")
	}
}

func TestEnsureOrchestrationWorktreeReusesExisting(t *testing.T) {
	repo := t.TempDir()
	runTestGit(t, repo, "init")
	runTestGit(t, repo, "config", "user.email", "orch@example.invalid")
	runTestGit(t, repo, "config", "user.name", "Orch Test")
	if err := fileutil.WriteFile(filepath.Join(repo, "README"), []byte("base\n"), paths.FilePerm600); err != nil {
		t.Fatal(err)
	}
	runTestGit(t, repo, "add", "README")
	runTestGit(t, repo, "commit", "-m", "base")

	wtRoot := t.TempDir()
	t.Setenv(zqkenv.AgentWorktreeRoot().Name(), wtRoot)

	ctx := context.Background()
	first, err := ensureOrchestrationWorktree(ctx, repo, "ATK-reuse-1")
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	second, err := ensureOrchestrationWorktree(ctx, repo, "ATK-reuse-1")
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if first != second {
		t.Fatalf("path changed %s -> %s", first, second)
	}
	if !isGitWorktreeCheckout(first) {
		t.Fatal("expected git checkout")
	}
}

func TestEnsureOrchestrationWorktree_resetsLeftoverDirt(t *testing.T) {
	repo := t.TempDir()
	runTestGit(t, repo, "init")
	runTestGit(t, repo, "config", "user.email", "orch@example.invalid")
	runTestGit(t, repo, "config", "user.name", "Orch Test")
	if err := fileutil.WriteFile(filepath.Join(repo, "kept.go"), []byte("package kept\n"), paths.FilePerm600); err != nil {
		t.Fatal(err)
	}
	runTestGit(t, repo, "add", "kept.go")
	runTestGit(t, repo, "commit", "-m", "base")

	wtRoot := t.TempDir()
	t.Setenv(zqkenv.AgentWorktreeRoot().Name(), wtRoot)

	ctx := context.Background()
	wt, err := ensureOrchestrationWorktree(ctx, repo, "ATK-reset-1")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := fileutil.WriteFile(filepath.Join(wt, "kept.go"), []byte("package clobbered\n"), paths.FilePerm600); err != nil {
		t.Fatal(err)
	}
	junk := filepath.Join(wt, "invented.go")
	if err := fileutil.WriteFile(junk, []byte("package invented\n"), paths.FilePerm600); err != nil {
		t.Fatal(err)
	}
	casDir := filepath.Join(wt, paths.ProcessDir, "backlog")
	if err := fileutil.MkdirAll(casDir, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	casFile := filepath.Join(casDir, "deadbeef.yaml")
	if err := fileutil.WriteFile(casFile, []byte("id: BLI-KEEP\n"), paths.FilePerm600); err != nil {
		t.Fatal(err)
	}

	again, err := ensureOrchestrationWorktree(ctx, repo, "ATK-reset-1")
	if err != nil {
		t.Fatalf("reuse: %v", err)
	}
	if again != wt {
		t.Fatalf("path changed %s -> %s", wt, again)
	}
	restored, err := fileutil.ReadFile(filepath.Join(wt, "kept.go"))
	if err != nil {
		t.Fatal(err)
	}
	if string(restored) != "package kept\n" {
		t.Fatalf("tracked file not restored: %q", restored)
	}
	if _, err := fileutil.Stat(junk); !fileutil.IsNotExist(err) {
		t.Fatalf("invented file should be removed, stat err = %v", err)
	}
	if _, err := fileutil.Stat(casFile); err != nil {
		t.Fatalf("process CAS must survive reset: %v", err)
	}
}

func TestEnsureOrchestrationWorktree_retargetsStaleAgentBranch(t *testing.T) {
	repo := t.TempDir()
	runTestGit(t, repo, "init", "-b", "main")
	runTestGit(t, repo, "config", "user.email", "orch@example.invalid")
	runTestGit(t, repo, "config", "user.name", "Orch Test")
	if err := fileutil.WriteFile(filepath.Join(repo, "old.go"), []byte("package old\n"), paths.FilePerm600); err != nil {
		t.Fatal(err)
	}
	runTestGit(t, repo, "add", "old.go")
	runTestGit(t, repo, "commit", "-m", "old tip")
	runTestGit(t, repo, "branch", "agent/ATK-stale-1")

	if err := fileutil.WriteFile(filepath.Join(repo, "new.go"), []byte("package new\n"), paths.FilePerm600); err != nil {
		t.Fatal(err)
	}
	runTestGit(t, repo, "add", "new.go")
	runTestGit(t, repo, "commit", "-m", "main moved")

	wtRoot := t.TempDir()
	t.Setenv(zqkenv.AgentWorktreeRoot().Name(), wtRoot)
	wt, err := ensureOrchestrationWorktree(context.Background(), repo, "ATK-stale-1")
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if _, err := fileutil.Stat(filepath.Join(wt, "new.go")); err != nil {
		t.Fatalf("worktree must be on current main, missing new.go: %v", err)
	}
}

func TestMergeObjectsByID(t *testing.T) {
	t.Parallel()
	got := mergeObjectsByID(
		[]map[string]any{{objects.FieldKeyID: "A"}, {objects.FieldKeyID: "B"}},
		[]map[string]any{{objects.FieldKeyID: "A"}, {objects.FieldKeyID: "C"}},
	)
	if len(got) != 3 {
		t.Fatalf("len=%d want 3", len(got))
	}
}

func TestRecoverFiltersAreStorageListFilters(t *testing.T) {
	t.Parallel()
	var _ []storage.ListFilter = recoverAgentTaskListFilters("PRI-X")
}
