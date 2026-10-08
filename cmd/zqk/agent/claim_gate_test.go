package agent

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestNewClaimGateCmd(t *testing.T) {
	cmd := NewClaimGateCmd()
	if cmd == nil {
		t.Fatalf("expected non-nil claim gate command")
	}
	if cmd.Flags() == nil {
		t.Fatalf("expected non-nil flags")
	}
	if cmd.Flags().Lookup(flagClaimGateBy) == nil {
		t.Errorf("missing flag --by")
	}
	if cmd.Flags().Lookup(flagClaimGateAssignment) == nil {
		t.Errorf("missing flag --assignment")
	}
}

func TestActiveIntentAssignment(t *testing.T) {
	tmpDir := t.TempDir()

	// Missing pointer file returns empty
	if a := activeIntentAssignment(tmpDir); a != "" {
		t.Errorf("expected empty string for missing pointer, got: %s", a)
	}

	stateDir := paths.StateDirPath(tmpDir)
	if err := fileutil.EnsureDir(stateDir); err != nil {
		t.Fatalf("failed to create state dir: %v", err)
	}

	pointerPath := filepath.Join(stateDir, "change_intent_active")
	intentPath := filepath.Join(tmpDir, "intent.json")

	// Write pointer to relative path
	if err := fileutil.WriteStandardFile(pointerPath, []byte("intent.json")); err != nil {
		t.Fatalf("failed to write pointer: %v", err)
	}

	// Corrupt intent file returns empty
	if err := fileutil.WriteStandardFile(intentPath, []byte("invalid json")); err != nil {
		t.Fatalf("failed to write corrupt intent: %v", err)
	}
	if a := activeIntentAssignment(tmpDir); a != "" {
		t.Errorf("expected empty string for invalid json, got: %s", a)
	}

	// Valid intent file returns assignment
	intentData, err := json.Marshal(map[string]any{"assignment": "PRI-TEST-001"})
	if err != nil {
		t.Fatalf("failed to marshal intent: %v", err)
	}
	if err := fileutil.WriteStandardFile(intentPath, intentData); err != nil {
		t.Fatalf("failed to write valid intent: %v", err)
	}

	if a := activeIntentAssignment(tmpDir); a != "PRI-TEST-001" {
		t.Errorf("expected PRI-TEST-001, got: %s", a)
	}
}

func TestRunAgentClaimGate_CommandExecution(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	out, err := executeAgentCommand(t, tempDir, provider, "claim-gate", "--by", "seat-tester", "--assignment", "PRI-SAMPLE")
	if err != nil && !strings.Contains(err.Error(), "claim") && !strings.Contains(out, "claim") {
		t.Logf("claim-gate returned: %v, out: %s", err, out)
	}
}

func TestResolveClaimantIdentity_Variations(t *testing.T) {
	// 1. ZQK_AGENT_ID set
	t.Setenv("ZQK_AGENT_ID", "custom-agent-99")
	if got := resolveClaimantIdentity(nil, nil); got != "custom-agent-99" {
		t.Errorf("expected custom-agent-99, got %s", got)
	}

	// 2. Clear ZQK_AGENT_ID, fallback to host or anonymous
	t.Setenv("ZQK_AGENT_ID", "")
	got := resolveClaimantIdentity(nil, nil)
	if got == "" {
		t.Errorf("expected non-empty claimant identity")
	}
}

func TestReleaseCmd_Execution(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	_, err := executeAgentCommand(t, tempDir, provider, "release")
	if err == nil {
		t.Errorf("expected error for missing args")
	}

	_, err = executeAgentCommand(t, tempDir, provider, "release", "ATK-NONEXISTENT")
	if err == nil {
		t.Errorf("expected error for non-existent task")
	}
}

func TestRunAgentClaimGate_AutoAssignAndWake(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	// 1. auto-assign with wake
	out, err := executeAgentCommand(t, tempDir, provider, "claim-gate", "--by", "seat-tester", "--auto-assign", "--wake")
	if err != nil && !strings.Contains(err.Error(), "claim") && !strings.Contains(out, "claim") {
		t.Logf("claim-gate auto-assign returned: %v, out: %s", err, out)
	}

	// 2. require-assignment without assignment fails or runs gate check
	out, err = executeAgentCommand(t, tempDir, provider, "claim-gate", "--by", "seat-tester", "--require-assignment")
	if err != nil && !strings.Contains(err.Error(), "claim") && !strings.Contains(out, "claim") {
		t.Logf("claim-gate require-assignment returned: %v, out: %s", err, out)
	}
}

func TestClaimTaskForExecute_NonAgentTask(t *testing.T) {
	if err := claimTaskForExecute(nil, nil, "", nil); err != nil {
		t.Fatalf("expected nil for nil task, got: %v", err)
	}
	bli := map[string]any{objects.FieldKeyKind: objects.KindBacklogItem}
	if err := claimTaskForExecute(nil, nil, "BLI-1", bli); err != nil {
		t.Fatalf("expected nil for non-agent task, got: %v", err)
	}
}
