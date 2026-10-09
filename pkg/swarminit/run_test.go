package swarminit

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/agentfeed"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestRun_unknownExecutorFailClosed(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	store := map[string]map[string]any{
		"PIP-BAD": testPipeline("PIP-BAD", []map[string]any{
			{objects.FieldKeyID: "ghost", stageKeyExecutor: "not_registered"},
		}),
	}
	art, err := Run(context.Background(), Options{
		PipelineID: "PIP-BAD",
		Env:        testEnv(root, store),
	})
	if err == nil || !strings.Contains(err.Error(), "unknown swarm-init executor") {
		t.Fatalf("want unknown executor, got art=%+v err=%v", art, err)
	}
	if art.OK || len(art.Stages) != 1 || art.Stages[0].Status != stageStatusFail {
		t.Fatalf("artifact=%+v", art)
	}
}

func TestRun_dryRunDoesNotWriteSeatsOrInstall(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	seatsPath := filepath.Join(root, "seats-sentinel.json")
	if err := fileutil.WriteFile(seatsPath, []byte("keep\n"), paths.FilePerm600); err != nil {
		t.Fatal(err)
	}
	store := map[string]map[string]any{
		"PIP-DRY": testPipeline("PIP-DRY", []map[string]any{
			{objects.FieldKeyID: "control_plane", stageKeyExecutor: ExecutorControlPlane},
			{objects.FieldKeyID: "bind_seats", stageKeyExecutor: ExecutorBindSeats, stageKeyPass: map[string]any{"conversation_live": false, "require_pid": false}},
			{objects.FieldKeyID: "seat_workers", stageKeyExecutor: ExecutorSeatWorkers},
			{objects.FieldKeyID: "comms_check", stageKeyExecutor: ExecutorCommsCheck},
			{objects.FieldKeyID: "orchestrate_plan", stageKeyExecutor: ExecutorOrchestratePlan, stageKeyConfig: map[string]any{"plan_id": "PRI-MESH-SWARM-INIT-001", "to_agent_id": "peer-agent-1"}},
		}),
	}
	saved := false
	installed := false
	env := testEnv(root, store)
	env.DryRun = true
	env.PlanID = "PRI-MESH-SWARM-INIT-001"
	env.SaveSeats = func(string, agentfeed.PeerSeatsFile) error {
		saved = true
		return nil
	}
	env.InstallSeatWorkers = func(context.Context, SeatWorkerInstallConfig) error {
		installed = true
		return nil
	}
	env.Steer = func(context.Context, string, string, bool) (SteerOutcome, error) {
		t.Fatal("steer must not run on dry-run")
		return SteerOutcome{}, nil
	}
	art, err := Run(context.Background(), Options{PipelineID: "PIP-DRY", Env: env})
	if err != nil {
		t.Fatal(err)
	}
	if !art.OK || !art.DryRun {
		t.Fatalf("artifact=%+v", art)
	}
	if saved || installed {
		t.Fatalf("dry-run wrote seats=%v install=%v", saved, installed)
	}
	body, err := fileutil.ReadFile(seatsPath) //nolint:gosec
	if err != nil || string(body) != "keep\n" {
		t.Fatalf("sentinel mutated: %s %v", body, err)
	}
}

func TestRun_fromStageSkipsPrefix(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	store := map[string]map[string]any{
		"PIP-FROM": testPipeline("PIP-FROM", []map[string]any{
			{objects.FieldKeyID: "control_plane", stageKeyExecutor: ExecutorControlPlane},
			{objects.FieldKeyID: "comms_check", stageKeyExecutor: ExecutorCommsCheck},
		}),
	}
	env := testEnv(root, store)
	env.DryRun = true
	env.FromStage = "comms_check"
	art, err := Run(context.Background(), Options{PipelineID: "PIP-FROM", Env: env})
	if err != nil {
		t.Fatal(err)
	}
	if len(art.Stages) != 2 || art.Stages[0].Status != stageStatusSkip || art.Stages[1].Status != stageStatusPass {
		t.Fatalf("stages=%+v", art.Stages)
	}
}

func TestRun_workflowResolvesPipelineRef(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	store := map[string]map[string]any{
		"WFL-MESH": {
			objects.FieldKeyID: "WFL-MESH",
			objects.FieldKeyMetadata: map[string]any{
				SwarmInitPipelineRefKey: "PIP-W",
			},
		},
		"PIP-W": testPipeline("PIP-W", []map[string]any{
			{objects.FieldKeyID: "control_plane", stageKeyExecutor: ExecutorControlPlane},
		}),
	}
	env := testEnv(root, store)
	env.DryRun = true
	art, err := Run(context.Background(), Options{WorkflowID: "WFL-MESH", Env: env})
	if err != nil {
		t.Fatal(err)
	}
	if art.PipelineID != "PIP-W" || art.WorkflowID != "WFL-MESH" {
		t.Fatalf("art=%+v", art)
	}
}

func testEnv(root string, store map[string]map[string]any) *Env {
	return &Env{
		ProjectRoot: root,
		Now:         func() time.Time { return time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC) },
		GetObject: func(_ context.Context, id string) (map[string]any, error) {
			obj, ok := store[id]
			if !ok {
				return nil, errNotFound(id)
			}
			return obj, nil
		},
		LoadSeats: func(string) (agentfeed.PeerSeatsFile, error) {
			return agentfeed.PeerSeatsFile{Seats: map[string]agentfeed.PeerSeatRecord{}}, nil
		},
		SaveSeats: func(string, agentfeed.PeerSeatsFile) error { return nil },
	}
}

type missingObj string

func (m missingObj) Error() string { return "object not found: " + string(m) }

func errNotFound(id string) error { return missingObj(id) }

// tdd refresh
