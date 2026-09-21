package workflow

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/accumulator"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/workflow/whatsnext"
)

func TestWhatsNextCommand_SyncSweepFlagRegistered(t *testing.T) {
	cmd := NewWhatsNextCmd()
	flag := cmd.Flags().Lookup("sync-sweep")
	if flag == nil {
		t.Fatal("expected --sync-sweep flag to be registered on workflow whats-next command")
	}
	if flag.DefValue != "false" {
		t.Fatalf("expected --sync-sweep default value to be 'false', got %q", flag.DefValue)
	}
}

func TestRunWhatsNextLite_ZeroCostHotPathLatency(t *testing.T) {
	tmpDir := t.TempDir()
	stateDir := filepath.Join(tmpDir, paths.ProjectDataDir, paths.StateDir)
	if err := os.MkdirAll(stateDir, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}

	litePath := filepath.Join(stateDir, "whats_next_lite.json")
	payload := &whatsnext.WhatsNextLitePayload{
		SchemaVersion:  whatsnext.MaterializedViewSchemaVersion,
		MaterializedAt: time.Now().UTC(),
		LeadPlan: &whatsnext.WhatsNextPriorityPlan{
			ID:     "PRI-TEST-LEAD",
			Title:  "Lead Plan Title",
			Status: objects.ObjectStatusInProgress,
		},
		ActivePlans: []whatsnext.WhatsNextPriorityPlan{
			{
				ID:     "PRI-TEST-LEAD",
				Title:  "Lead Plan Title",
				Status: objects.ObjectStatusInProgress,
			},
		},
		BacklogCountsByPlan: map[string]map[string]int{
			"PRI-TEST-LEAD": {
				objects.ObjectStatusPlanned:    5,
				objects.ObjectStatusInProgress: 2,
			},
		},
		TotalBacklogCounts: map[string]int{
			objects.ObjectStatusPlanned:    5,
			objects.ObjectStatusInProgress: 2,
		},
		RunwayDepth: 3,
		ActiveTasksByAssignee: map[string]*whatsnext.ActiveTaskNode{
			"PER-AGENT-1": {
				ID:     "ATK-TASK-1",
				Status: objects.ObjectStatusInProgress,
				Title:  "Test Active Task",
			},
		},
		AgentInstructionsByPlan: map[string]string{
			"PRI-TEST-LEAD": "continue",
		},
		PackagingCuesByPlan: map[string]string{
			"PRI-TEST-LEAD": "wrap-it-up",
		},
	}

	env := accumulator.Envelope[*whatsnext.WhatsNextLitePayload]{
		SchemaVersion:  whatsnext.MaterializedViewSchemaVersion,
		MaterializedAt: time.Now().UTC(),
		Payload:        payload,
	}
	data, err := json.MarshalIndent(env, "", "  ")
	if err != nil {
		t.Fatalf("marshal envelope failed: %v", err)
	}
	if err := fileutil.WriteFile(litePath, data, paths.FilePerm644); err != nil {
		t.Fatalf("write lite file failed: %v", err)
	}

	start := time.Now()
	loaded, err := whatsnext.GetOrRecoverPayload(context.Background(), nil, tmpDir, 2*time.Minute)
	duration := time.Since(start)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if loaded == nil {
		t.Fatal("expected non-nil loaded payload")
	}

	// Performance verification: Hot read must complete fast (<= 100ms in unit test environment)
	if duration > 100*time.Millisecond {
		t.Errorf("hot path read exceeded latency target: took %v (expected <= 100ms in unit test)", duration)
	}

	if loaded.Stale {
		t.Errorf("expected fresh view, got Stale=true")
	}
	if loaded.LeadPlan == nil || loaded.LeadPlan.ID != "PRI-TEST-LEAD" {
		t.Errorf("expected lead plan PRI-TEST-LEAD, got %v", loaded.LeadPlan)
	}
	if loaded.RunwayDepth != 3 {
		t.Errorf("expected runway depth 3, got %d", loaded.RunwayDepth)
	}
	if counts, ok := loaded.BacklogCountsByPlan["PRI-TEST-LEAD"]; !ok || counts[objects.ObjectStatusPlanned] != 5 {
		t.Errorf("expected 5 planned backlog items, got %v", counts)
	}
}

func TestRunWhatsNextLite_ColdBootNonBlockingRecovery(t *testing.T) {
	tmpDir := t.TempDir()

	start := time.Now()
	loaded, err := whatsnext.GetOrRecoverPayload(context.Background(), nil, tmpDir, 2*time.Minute)
	duration := time.Since(start)

	if err != nil {
		t.Fatalf("unexpected error on cold boot: %v", err)
	}
	if loaded == nil {
		t.Fatal("expected bootstrap payload on cold boot")
	}

	// Cold boot read must also complete quickly without blocking caller
	if duration > 10*time.Millisecond {
		t.Errorf("cold boot check took %v (expected <= 10ms)", duration)
	}

	if !loaded.Stale {
		t.Error("expected Stale=true on cold boot")
	}
	if !loaded.Recovering {
		t.Error("expected Recovering=true on cold boot")
	}
}
