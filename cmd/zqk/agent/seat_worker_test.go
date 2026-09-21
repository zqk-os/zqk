package agent

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/agentprompt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestParseOrchestratePlanDirective(t *testing.T) {
	t.Parallel()

	planID, ok, err := parseOrchestratePlanDirective("ORCHESTRATE_PLAN PRI-123\ncontext follows")
	if err != nil {
		t.Fatalf("parseOrchestratePlanDirective returned error: %v", err)
	}
	if !ok || planID != "PRI-123" {
		t.Fatalf("planID=%q ok=%v, want PRI-123 true", planID, ok)
	}
}

func TestExtractATKIDFromSteer(t *testing.T) {
	t.Parallel()

	body := "ATTN peer-1 NIGHT-DUTY MINT: claim ATK-1-abc for BLI-TEST-ITEM."
	got := extractATKIDFromSteer(body)
	if got != "ATK-1-abc" {
		t.Fatalf("extractATKIDFromSteer = %q", got)
	}
	if extractATKIDFromSteer("no task here") != "" {
		t.Fatal("expected empty ATK id")
	}
}

func TestSeatWorkerWorkClass(t *testing.T) {
	t.Parallel()

	cef := seatWorkerWorkClass("ATTN qwen-2 CLAIMED ATK-1. Evidence under docs/quality/cef-runs/2026-09-03-AGENT_AD/. FORBID: source edits.")
	if cef != agentprompt.WorkClassDocsEval {
		t.Fatalf("CEF remesure steer must be docs_eval, got %q", cef)
	}
	if seatWorkerWorkClass("pipeline_ref=PIP-CEF-DIAMOND-REMEASURE-001 stage=W1") != agentprompt.WorkClassDocsEval {
		t.Fatal("PIP-CEF pipeline_ref must be docs_eval")
	}
	if seatWorkerWorkClass("ATTN claim ATK-1 and fix pkg/scheduler/swarm_worker.go") != agentprompt.WorkClassCoding {
		t.Fatal("coding ATK must stay coding")
	}
}

func TestBuildSeatWorkerAgentXPromptsRefusesWithoutStorage(t *testing.T) {
	t.Parallel()

	_, err := buildSeatWorkerAgentXPrompts(
		context.Background(),
		logging.GetLoggerFromProfile("test"),
		nil,
		nil,
		t.TempDir(),
		"peer-agent-1",
		"PER-ORCH-ALPHA",
		"AFE-1",
		"ATTN claim ATK-1-abc — swarm must not idle.",
	)
	if !isPreparedContextRefusal(err) {
		t.Fatalf("want prepared-context refusal, got %v", err)
	}
}

func TestBuildSeatWorkerAgentXPromptsRefusesSteerWithoutATK(t *testing.T) {
	t.Parallel()

	_, err := buildSeatWorkerAgentXPrompts(
		context.Background(),
		logging.GetLoggerFromProfile("test"),
		nil,
		nil,
		t.TempDir(),
		"peer-agent-1",
		"PER-ORCH-ALPHA",
		"AFE-1",
		"ATTN do something useful in the repo",
	)
	if !isPreparedContextRefusal(err) {
		t.Fatalf("want prepared-context refusal, got %v", err)
	}
}

func TestParseOrchestratePlanDirectiveRejectsUnsafeID(t *testing.T) {
	t.Parallel()

	if _, ok, err := parseOrchestratePlanDirective("ORCHESTRATE_PLAN PRI-123;rm"); !ok || err == nil {
		t.Fatalf("unsafe directive ok=%v err=%v, want recognized error", ok, err)
	}
}

func TestTriggerPlanOrchestrationSubmitsAgentOrchestrate(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	binDir := filepath.Join(root, "bin")
	if err := fileutil.MkdirAll(binDir, paths.DirPerm750); err != nil {
		t.Fatal(err)
	}
	zqkPath := filepath.Join(binDir, "zqk")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$0.args\"\n"
	if err := fileutil.WriteFile(zqkPath, []byte(script), paths.DirPerm700); err != nil {
		t.Fatal(err)
	}

	out, err := triggerPlanOrchestration(context.Background(), nil, nil, root, "PRI-123")
	if err != nil {
		t.Fatalf("trigger plan orchestration: %v", err)
	}
	got, err := fileutil.ReadFile(zqkPath + ".args")
	if err != nil {
		t.Fatal(err)
	}
	args := string(got)
	for _, want := range []string{"scheduler", "submit", "agent orchestrate PRI-123"} {
		if !strings.Contains(args, want) {
			t.Fatalf("zqk args %q missing %q", args, want)
		}
	}
	if strings.Contains(args, "--persona-id") {
		t.Fatalf("persona filter must not be applied: %q", args)
	}
	if skip, msg := recentPlanOrchSubmit(root, "PRI-123"); !skip {
		t.Fatalf("expected cooldown after submit, out=%q", out)
	} else if !strings.Contains(msg, "PRI-123") {
		t.Fatalf("cooldown msg = %q", msg)
	}
	if _, err := triggerPlanOrchestration(context.Background(), nil, nil, root, "PRI-123"); err != nil {
		t.Fatalf("second trigger: %v", err)
	}
}

func TestResolveSeatWorkerPrioritySkipsProbeForEmptyInbox(t *testing.T) {
	t.Parallel()

	calls := 0
	got, err := resolveSeatWorkerPriority(context.Background(), 0, func(context.Context) (string, error) {
		calls++
		return "PRI-unexpected", nil
	})
	if err != nil {
		t.Fatalf("resolveSeatWorkerPriority returned error: %v", err)
	}
	if got != "" {
		t.Fatalf("priority = %q, want empty", got)
	}
	if calls != 0 {
		t.Fatalf("priority probe calls = %d, want 0", calls)
	}
}

func TestResolveSeatWorkerPriorityProbesWhenInboxHasWork(t *testing.T) {
	t.Parallel()

	calls := 0
	got, err := resolveSeatWorkerPriority(context.Background(), 1, func(context.Context) (string, error) {
		calls++
		return "PRI-123", nil
	})
	if err != nil {
		t.Fatalf("resolveSeatWorkerPriority returned error: %v", err)
	}
	if got != "PRI-123" {
		t.Fatalf("priority = %q, want PRI-123", got)
	}
	if calls != 1 {
		t.Fatalf("priority probe calls = %d, want 1", calls)
	}
}

func TestSeatWorkerLane_usesOrchLaneNotLeftoverSeatID(t *testing.T) {
	t.Parallel()

	cases := []struct {
		persona, agentID, want string
	}{
		{objects.ConstPersonaOrchestratorAlpha, "antigravity-1", "alpha"},
		{objects.ConstPersonaOrchestratorBeta, "antigravity-2", "beta"},
		{objects.ConstPersonaOrchestratorGamma, "leftover-vendor-3", "gamma"},
	}
	for _, tc := range cases {
		if got := seatWorkerLane(tc.persona, tc.agentID); got != tc.want {
			t.Fatalf("seatWorkerLane(%q, %q) = %q, want %q", tc.persona, tc.agentID, got, tc.want)
		}
		id := seatWorkerEngineID(tc.persona, tc.agentID)
		if strings.Contains(strings.ToLower(id), "antigravity") {
			t.Fatalf("engineID %q embeds leftover vendor-shaped seat id", id)
		}
		if !strings.HasPrefix(id, "seat-worker-"+tc.want+"-") {
			t.Fatalf("engineID %q, want prefix seat-worker-%s-", id, tc.want)
		}
	}
}

func TestSeatWorkerLane_hashesUnknownPersonaSeat(t *testing.T) {
	t.Parallel()

	got := seatWorkerLane(objects.ConstPersonaDefaultOperator, "antigravity-1")
	if got == "" || strings.Contains(got, "antigravity") || got == "antigravity-1" {
		t.Fatalf("unknown-persona lane = %q, must not echo leftover seat id", got)
	}
	if len(got) != 8 {
		t.Fatalf("hashed lane length = %d, want 8 hex chars", len(got))
	}
	if seatWorkerLane("", "") != seatWorkerLaneUnknown {
		t.Fatalf("empty lane = %q, want %q", seatWorkerLane("", ""), seatWorkerLaneUnknown)
	}
}

func TestSeatWorkerLane_configuredViaDirectEnv(t *testing.T) {
	t.Setenv("ZQK_WORKER_LANE", "fast-pipeline")
	got := seatWorkerLane("PER-ORCH-ALPHA", "peer-1")
	if got != "fast-pipeline" {
		t.Fatalf("ZQK_WORKER_LANE override = %q, want fast-pipeline", got)
	}
}

func TestSeatWorkerLane_configuredViaMappedEnv(t *testing.T) {
	t.Setenv("ZQK_WORKER_LANES", "PER-CUSTOM=custom-lane,peer-99=seat-99-lane")
	if got := seatWorkerLane("PER-CUSTOM", ""); got != "custom-lane" {
		t.Fatalf("ZQK_WORKER_LANES persona mapping = %q, want custom-lane", got)
	}
	if got := seatWorkerLane("", "peer-99"); got != "seat-99-lane" {
		t.Fatalf("ZQK_WORKER_LANES seat mapping = %q, want seat-99-lane", got)
	}

	// JSON format
	t.Setenv("ZQK_WORKER_LANES", `{"PER-SPECIAL":"special-lane"}`)
	if got := seatWorkerLane("PER-SPECIAL", ""); got != "special-lane" {
		t.Fatalf("ZQK_WORKER_LANES json mapping = %q, want special-lane", got)
	}
}

func TestSeatWorkerLane_configuredViaPeerSeats(t *testing.T) {
	tmpDir := t.TempDir()
	peerSeatsDir := filepath.Join(tmpDir, paths.ProjectDataDir, paths.StateDir, paths.MeshStateSubdir)
	if err := fileutil.MkdirAll(peerSeatsDir, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	peerSeatsContent := `{
		"schema_version": "1",
		"seats": {
			"peer-custom-seat": {
				"lane": "configured-seat-lane",
				"persona_ref": "PER-DOCS"
			}
		}
	}`
	if err := fileutil.WriteFile(filepath.Join(peerSeatsDir, paths.PeerSeatsFile), []byte(peerSeatsContent), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}

	got := seatWorkerLaneWithRoot(tmpDir, "PER-DOCS", "peer-custom-seat")
	if got != "configured-seat-lane" {
		t.Fatalf("peer_seats.json configured lane = %q, want configured-seat-lane", got)
	}
}

func TestSeatWorkerLane_configuredViaWorkerLanesFile(t *testing.T) {
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, paths.ProjectDataDir, paths.ConfigDir)
	if err := fileutil.MkdirAll(configDir, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	configContent := `{
		"worker_lanes": {
			"PER-CUSTOM-LEAD": "lead-lane",
			"worker-agent-77": "high-priority"
		}
	}`
	if err := fileutil.WriteFile(filepath.Join(configDir, "worker_lanes.json"), []byte(configContent), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}

	if got := seatWorkerLaneWithRoot(tmpDir, "PER-CUSTOM-LEAD", ""); got != "lead-lane" {
		t.Fatalf("worker_lanes.json persona lane = %q, want lead-lane", got)
	}
	if got := seatWorkerLaneWithRoot(tmpDir, "", "worker-agent-77"); got != "high-priority" {
		t.Fatalf("worker_lanes.json seat lane = %q, want high-priority", got)
	}
}

func TestSeatWorkerLane_dynamicArbitraryOrchestrator(t *testing.T) {
	t.Parallel()

	cases := []struct {
		persona string
		want    string
	}{
		{"PER-ORCH-DELTA", "delta"},
		{"PER-ORCH-OMEGA", "omega"},
		{"PER-ORCH-DOCS-EVAL", "docs-eval"},
		{"ORCH-FAST-TRACK", "fast-track"},
	}
	for _, tc := range cases {
		if got := seatWorkerLane(tc.persona, "agent-1"); got != tc.want {
			t.Fatalf("dynamic lane for %q = %q, want %q", tc.persona, got, tc.want)
		}
	}
}

func TestSeatWorker_WorktreeRootBinding(t *testing.T) {
	// 1. Unbonded worktree under zqk-worktrees must fail closed
	unbondedWorktree := filepath.Join(fileutil.TempDir(), "zqk-worktrees", "test-repo", "ATK-worker-unbonded")
	if err := fileutil.EnsureDir(filepath.Join(unbondedWorktree, paths.ProjectDataDir)); err != nil {
		t.Fatal(err)
	}
	defer fileutil.RemoveAll(unbondedWorktree)

	t.Setenv(zqkenv.ProjectRoot().Name(), unbondedWorktree)
	cmd := NewSeatWorkerCmd()
	cmd.SetArgs([]string{"--agent-id", "test-agent", "--once"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected seat-worker to fail closed with unbonded agent worktree ZQK_PROJECT_ROOT, got nil")
	}

	// 2. Acceptance test case: /tmp/ATK-* unbonded must fail closed
	atkWorktree := filepath.Join(fileutil.TempDir(), "ATK-acceptance-unbonded")
	if err := fileutil.EnsureDir(filepath.Join(atkWorktree, paths.ProjectDataDir)); err != nil {
		t.Fatal(err)
	}
	defer fileutil.RemoveAll(atkWorktree)

	t.Setenv(zqkenv.ProjectRoot().Name(), atkWorktree)
	cmd2 := NewSeatWorkerCmd()
	cmd2.SetArgs([]string{"--agent-id", "test-agent", "--once"})
	err = cmd2.Execute()
	if err == nil {
		t.Fatal("expected seat-worker to fail closed with /tmp/ATK-* unbonded worktree, got nil")
	}
}
