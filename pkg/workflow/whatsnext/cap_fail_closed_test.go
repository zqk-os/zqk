package whatsnext

import (
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// TestCAPFailClosedEvidence validates that CAP orchestration strictly fail-closes
// without silent success or unbounded hanging when work cannot be completed.
func TestCAPFailClosedEvidence(t *testing.T) {
	tmpDir, err := fileutil.MkdirTemp("", "cap_fail_closed_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer fileutil.RemoveAll(tmpDir)

	// 1. When swarm is starved (empty backlog), CAP pins to grooming (fail-closed against empty execution)
	emptyCounts := map[string]int{
		actionablePlanned:    0,
		actionableInProgress: 0,
		actionableVerifying:  0,
		actionableBlocked:    0,
		actionableValidated:  0,
	}

	if !SwarmStarved(emptyCounts) {
		t.Errorf("expected SwarmStarved to be true for empty backlog")
	}

	selected := SelectCAPInstruction(tmpDir, emptyCounts)
	if selected != capStageGrooming {
		t.Errorf("expected capStageGrooming when starved, got %s", selected)
	}

	// 2. Advance journal recording and tamper detection
	EnsureCAPStage(tmpDir, CapStages[0])
	adv := AdvanceCAPStageFromOrchestrator(tmpDir, CapStages[0], "CVS-001", "CVS-002", []string{"ART-001"})
	if adv != CapStages[1] {
		t.Errorf("expected advance to %s, got %s", CapStages[1], adv)
	}

	last, ok := LastCapAdvanceJournalEntry(tmpDir)
	if !ok || last.NextStage != CapStages[1] {
		t.Fatalf("expected last journal entry nextStage %s, got %v", CapStages[1], last)
	}

	// 3. Tampering detection
	stateFile := filepath.Join(tmpDir, paths.ProjectDataDir, paths.StateDir, capCycleFileName)
	_ = fileutil.WriteFile(stateFile, []byte("cap_stage_review"), paths.FilePerm644)
	if !CapCycleTamperDetected(tmpDir) {
		t.Errorf("expected tamper detection when stateFile does not match journal")
	}

	restored, ok := RestoreCAPStageFromJournal(tmpDir)
	if !ok || restored != CapStages[1] {
		t.Errorf("expected restored stage %s, got %s", CapStages[1], restored)
	}
}
