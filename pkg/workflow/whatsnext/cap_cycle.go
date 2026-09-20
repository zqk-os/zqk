package whatsnext

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// CapStages is the ordered CAP loop. Selection peeks; advance is explicit after
// stage delivery evidence (see CapOrchestratorHandler).
var CapStages = []string{
	"cap_stage_planning",
	"cap_stage_design",
	"cap_stage_grooming",
	"cap_stage_orchestrating",
	"cap_stage_review",
	"cap_stage_metrics",
	"cap_stage_self_improvement",
	"cap_stage_sentinel",
}

const (
	capCycleFileName          = "cap_cycle"
	capAdvanceJournalFileName = "cap_advance_journal.jsonl"
	capStageGrooming          = "cap_stage_grooming"
	actionablePlanned         = "planned"
	actionableInProgress      = "in_progress"
	actionableVerifying       = "verifying"
	actionableBlocked         = "blocked"
	actionableValidated       = "validated"
)

// CapAdvanceJournalEntry records a legitimate CAP stage advance (CRIT-CAPH-002).
type CapAdvanceJournalEntry struct {
	Timestamp       string   `json:"timestamp"`
	CompletedStage  string   `json:"completed_stage"`
	NextStage       string   `json:"next_stage"`
	CvsID           string   `json:"cvs_id,omitempty"`
	FocusChildCvsID string   `json:"focus_child_cvs_id,omitempty"`
	ArtifactIDs     []string `json:"artifact_ids,omitempty"`
	Source          string   `json:"source"`
}

func capCyclePath(projectRoot string) string {
	return filepath.Join(projectRoot, paths.ProjectDataDir, paths.StateDir, capCycleFileName)
}

func capAdvanceJournalPath(projectRoot string) string {
	return filepath.Join(projectRoot, paths.ProjectDataDir, paths.StateDir, capAdvanceJournalFileName)
}

// PeekCAPStage returns the current CAP stage without advancing the cycle.
func PeekCAPStage(projectRoot string) string {
	stateFile := capCyclePath(projectRoot)
	fileutil.EnsureDir(filepath.Dir(stateFile)) //nolint:gosec
	b, err := fileutil.ReadFile(stateFile)
	if err != nil {
		return CapStages[0]
	}
	current := strings.TrimSpace(string(b))
	if current == "" {
		return CapStages[0]
	}
	for _, stage := range CapStages {
		if stage == current {
			return current
		}
	}
	return CapStages[0]
}

// EnsureCAPStage writes stage as the current pointer without advancing past it.
// Prefer AdvanceCAPStageFromOrchestrator for production advances (journaled).
func EnsureCAPStage(projectRoot, stage string) {
	if stage == "" {
		return
	}
	stateFile := capCyclePath(projectRoot)
	fileutil.EnsureDir(filepath.Dir(stateFile)) //nolint:gosec
	_ = fileutil.WriteStandardFile(stateFile, []byte(stage))
}

// LastCapAdvanceJournalEntry returns the last journal line, if any.
func LastCapAdvanceJournalEntry(projectRoot string) (CapAdvanceJournalEntry, bool) {
	b, err := fileutil.ReadFile(capAdvanceJournalPath(projectRoot))
	if err != nil || len(b) == 0 {
		return CapAdvanceJournalEntry{}, false
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		var e CapAdvanceJournalEntry
		if json.Unmarshal([]byte(line), &e) == nil && e.NextStage != "" {
			return e, true
		}
	}
	return CapAdvanceJournalEntry{}, false
}

// CapCycleTamperDetected reports when cap_cycle disagrees with the last journaled next stage.
func CapCycleTamperDetected(projectRoot string) bool {
	last, ok := LastCapAdvanceJournalEntry(projectRoot)
	if !ok {
		return false
	}
	return PeekCAPStage(projectRoot) != last.NextStage
}

// RestoreCAPStageFromJournal resets cap_cycle to the last journaled next stage when present.
func RestoreCAPStageFromJournal(projectRoot string) (string, bool) {
	last, ok := LastCapAdvanceJournalEntry(projectRoot)
	if !ok {
		return "", false
	}
	EnsureCAPStage(projectRoot, last.NextStage)
	return last.NextStage, true
}

// AppendCapAdvanceJournal writes one JSONL advance record.
func AppendCapAdvanceJournal(projectRoot string, e CapAdvanceJournalEntry) error {
	if e.Timestamp == "" {
		e.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}
	if e.Source == "" {
		e.Source = "cap_orchestrator"
	}
	path := capAdvanceJournalPath(projectRoot)
	fileutil.EnsureDir(filepath.Dir(path)) //nolint:gosec
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	f, err := fileutil.OpenFile(path, fileutil.O_APPEND|fileutil.O_CREATE|fileutil.O_WRONLY, paths.FilePerm644)
	if err != nil {
		return err
	}
	defer f.Close() //nolint:gosec
	_, err = f.Write(append(b, '\n'))
	return err
}

// AdvanceCAPStage moves cap_cycle from completedStage to the next stage.
func AdvanceCAPStage(projectRoot, completedStage string) string {
	current := PeekCAPStage(projectRoot)
	if completedStage != "" && current != completedStage {
		return current
	}

	next := CapStages[0]
	for i, stage := range CapStages {
		if stage == current {
			if i+1 < len(CapStages) {
				next = CapStages[i+1]
			}
			break
		}
	}
	EnsureCAPStage(projectRoot, next)
	return next
}

// AdvanceCAPStageFromOrchestrator advances and appends a journal entry (sole legitimate production path).
func AdvanceCAPStageFromOrchestrator(projectRoot, completedStage, cvsID, focusChild string, artifactIDs []string) string {
	next := AdvanceCAPStage(projectRoot, completedStage)
	_ = AppendCapAdvanceJournal(projectRoot, CapAdvanceJournalEntry{
		CompletedStage:  completedStage,
		NextStage:       next,
		CvsID:           cvsID,
		FocusChildCvsID: focusChild,
		ArtifactIDs:     artifactIDs,
		Source:          "cap_orchestrator",
	})
	return next
}

// ActionableBacklogCount sums statuses that mean the swarm/strategy still has
// work inventory (including validated BLIs awaiting planned dispatch).
func ActionableBacklogCount(counts map[string]int) int {
	if counts == nil {
		return 0
	}
	return counts[actionablePlanned] + counts[actionableInProgress] + counts[actionableVerifying] + counts[actionableBlocked] + counts[actionableValidated]
}

// SwarmStarved reports whether CAP should prioritize TPM grooming so strategy
// stays ahead of execution (empty actionable backlog).
func SwarmStarved(counts map[string]int) bool {
	return ActionableBacklogCount(counts) == 0
}

// SelectCAPInstruction peeks the cycle, but pins to grooming when the swarm
// has no actionable backlog so CAP cannot skip past an empty grooming pass.
func SelectCAPInstruction(projectRoot string, backlogCounts map[string]int) string {
	if SwarmStarved(backlogCounts) {
		EnsureCAPStage(projectRoot, capStageGrooming)
		return capStageGrooming
	}
	return PeekCAPStage(projectRoot)
}

// getSystemCAPInstruction is the legacy name used by Execute; it peeks (and
// may pin grooming when starved) and must not advance the cycle.
func getSystemCAPInstruction(projectRoot string) string {
	return PeekCAPStage(projectRoot)
}
