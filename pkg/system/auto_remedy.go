package system

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/resourcehygiene"
	"github.com/zqk-os/zqk/pkg/systemcheck"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// RemedyActionType denotes what action is required to fix an anomaly.
type RemedyActionType string

const (
	ActionRemoveFile  RemedyActionType = "remove_file"
	ActionRunCommand  RemedyActionType = "run_command"
	ActionTouchFile   RemedyActionType = "touch_file"
	ActionSeed        RemedyActionType = "seed"
	ActionKillProcess RemedyActionType = "kill_process"
)

const (
	errTargetOutsideZQK = "target file outside .zqk directory"
	msgManualRequired   = "requires manual execution or external confirmation"
	msgRefuseDaemonLock = "refusing to remove daemon singleton lock file"
	msgNoSeeder         = "no seeder registered"
	msgCommandRequires  = "command action requires runner execution"
	targetPolicies      = "policies"
	targetPersonas      = "personas"
)

// RemedyPlan defines a concrete, executable recovery step for a system check or precondition anomaly.
type RemedyPlan struct {
	ID          string           `json:"id"`
	Title       string           `json:"title"`
	Description string           `json:"description"`
	ActionType  RemedyActionType `json:"action_type"`
	Target      string           `json:"target"`
	Command     string           `json:"command,omitempty"`
	Confidence  float64          `json:"confidence"`
	AutoApply   bool             `json:"auto_apply"`
}

// RemedyOutcome summarizes the outcome of attempting an auto-remedy fix.
type RemedyOutcome struct {
	Plan    RemedyPlan `json:"plan"`
	Applied bool       `json:"applied"`
	Error   string     `json:"error,omitempty"`
	Detail  string     `json:"detail,omitempty"`
}

// RemedyReport bundles diagnosed remedy plans and auto-fix execution records.
type RemedyReport struct {
	TotalDiagnosed int             `json:"total_diagnosed"`
	TotalApplied   int             `json:"total_applied"`
	Outcomes       []RemedyOutcome `json:"outcomes"`
}

// DiagnosticsRemedyEngine analyzes system health, I/O hygiene anomalies, and check issues
// to produce deterministic remedy recipes and auto-apply safe fixes.
type DiagnosticsRemedyEngine struct {
	ProjectRoot   string
	PolicySeeder  func(projectRoot string) (int, error)
	PersonaSeeder func(projectRoot string) (int, error)
}

// NewDiagnosticsRemedyEngine constructs a new remedy engine.
func NewDiagnosticsRemedyEngine(projectRoot string) *DiagnosticsRemedyEngine {
	return &DiagnosticsRemedyEngine{
		ProjectRoot: projectRoot,
	}
}

// Diagnose evaluates I/O telemetry, check results, and seeding preconditions to generate remedy plans.
func (e *DiagnosticsRemedyEngine) Diagnose(ctx context.Context, checkResults []systemcheck.CheckResult) ([]RemedyPlan, error) {
	if e.ProjectRoot == "" {
		return nil, nil
	}

	plans := make([]RemedyPlan, 0)
	plans = append(plans, e.diagnoseIOResources(ctx)...)
	plans = append(plans, e.diagnoseSeedingPreconditions()...)
	plans = append(plans, e.diagnoseCheckIssues(checkResults)...)
	return plans, nil
}

func (e *DiagnosticsRemedyEngine) diagnoseIOResources(ctx context.Context) []RemedyPlan {
	var plans []RemedyPlan
	ioTel, err := resourcehygiene.InspectIOResources(ctx, e.ProjectRoot)
	if err != nil || ioTel == nil {
		return plans
	}

	for _, lockPath := range ioTel.StaleLockPaths {
		fullPath := lockPath
		if !filepath.IsAbs(fullPath) {
			fullPath = filepath.Join(e.ProjectRoot, lockPath)
		}
		base := filepath.Base(lockPath)
		plans = append(plans, RemedyPlan{
			ID:          "REMEDY-STALE-LOCK-" + base,
			Title:       "Clean Stale Lock File (" + base + ")",
			Description: "Remove abandoned lock file: " + lockPath,
			ActionType:  ActionRemoveFile,
			Target:      fullPath,
			Confidence:  1.0,
			AutoApply:   true,
		})
	}

	for _, tmpPath := range ioTel.OrphanedTempPaths {
		fullPath := tmpPath
		if !filepath.IsAbs(fullPath) {
			fullPath = filepath.Join(e.ProjectRoot, tmpPath)
		}
		base := filepath.Base(tmpPath)
		plans = append(plans, RemedyPlan{
			ID:          "REMEDY-ORPHAN-TMP-" + base,
			Title:       "Clean Orphaned Temp File (" + base + ")",
			Description: "Remove abandoned temp file: " + tmpPath,
			ActionType:  ActionRemoveFile,
			Target:      fullPath,
			Confidence:  1.0,
			AutoApply:   true,
		})
	}

	for _, orphanProc := range ioTel.OrphanedProcesses {
		procID := strings.ReplaceAll(orphanProc, " ", "-")
		plans = append(plans, RemedyPlan{
			ID:          "REMEDY-ORPHAN-PROC-" + procID,
			Title:       "Terminate Orphaned Process (" + orphanProc + ")",
			Description: "Terminate orphaned process detached under PID 1: " + orphanProc,
			ActionType:  ActionKillProcess,
			Target:      orphanProc,
			Confidence:  1.0,
			AutoApply:   true,
		})
	}
	return plans
}

func (e *DiagnosticsRemedyEngine) diagnoseSeedingPreconditions() []RemedyPlan {
	var plans []RemedyPlan
	processDir := filepath.Join(e.ProjectRoot, paths.ProcessDir)
	policyDir := filepath.Join(processDir, targetPolicies)
	if !dirHasYAML(policyDir) && !dirHasYAML(filepath.Join(processDir, "policy")) {
		plans = append(plans, RemedyPlan{
			ID:          "REMEDY-UNSEEDED-POLICIES",
			Title:       "Seed Default Policy Pack",
			Description: "No policy objects found in .zqk/process/policies",
			ActionType:  ActionSeed,
			Target:      targetPolicies,
			Command:     paths.CLICommandName + " system init --force",
			Confidence:  1.0,
			AutoApply:   e.PolicySeeder != nil,
		})
	}

	personaDir := filepath.Join(processDir, targetPersonas)
	if !dirHasYAML(personaDir) && !dirHasYAML(filepath.Join(processDir, "persona")) {
		plans = append(plans, RemedyPlan{
			ID:          "REMEDY-UNSEEDED-PERSONAS",
			Title:       "Seed Default Agent Seating Pack",
			Description: "No persona objects found in .zqk/process/personas",
			ActionType:  ActionSeed,
			Target:      targetPersonas,
			Command:     paths.CLICommandName + " system agent-onboard",
			Confidence:  1.0,
			AutoApply:   e.PersonaSeeder != nil,
		})
	}
	return plans
}

func (e *DiagnosticsRemedyEngine) diagnoseCheckIssues(checkResults []systemcheck.CheckResult) []RemedyPlan {
	var plans []RemedyPlan
	for _, cr := range checkResults {
		for _, issue := range cr.Issues {
			if issue.FixCommand != "" {
				plans = append(plans, RemedyPlan{
					ID:          "REMEDY-ISSUE-" + cr.ObjectID,
					Title:       "Fix validation issue for " + cr.ObjectID,
					Description: issue.Message,
					ActionType:  ActionRunCommand,
					Target:      cr.ObjectID,
					Command:     issue.FixCommand,
					Confidence:  0.95,
					AutoApply:   false,
				})
			} else if issue.AutoFixable {
				plans = append(plans, RemedyPlan{
					ID:          "REMEDY-AUTOFIX-" + cr.ObjectID,
					Title:       "Apply autofix for " + cr.ObjectID,
					Description: issue.Message,
					ActionType:  ActionRunCommand,
					Target:      cr.ObjectID,
					Command:     paths.CLICommandName + " system check --auto-fix --ids " + cr.ObjectID,
					Confidence:  0.90,
					AutoApply:   false,
				})
			}
		}
	}
	return plans
}

// Apply executes designated auto-remedy plans safely.
func (e *DiagnosticsRemedyEngine) Apply(ctx context.Context, plans []RemedyPlan) (*RemedyReport, error) {
	report := &RemedyReport{
		TotalDiagnosed: len(plans),
		Outcomes:       make([]RemedyOutcome, 0, len(plans)),
	}

	for _, plan := range plans {
		if !plan.AutoApply {
			report.Outcomes = append(report.Outcomes, RemedyOutcome{
				Plan:    plan,
				Applied: false,
				Detail:  msgManualRequired,
			})
			continue
		}

		outcome := e.applyPlan(plan)
		if outcome.Applied {
			report.TotalApplied++
		}
		report.Outcomes = append(report.Outcomes, outcome)
	}

	return report, nil
}

func (e *DiagnosticsRemedyEngine) applyPlan(plan RemedyPlan) RemedyOutcome {
	switch plan.ActionType {
	case ActionRemoveFile:
		return e.applyRemoveFile(plan)
	case ActionTouchFile:
		return e.applyTouchFile(plan)
	case ActionSeed:
		return e.applySeed(plan)
	case ActionKillProcess:
		return e.applyKillProcess(plan)
	case ActionRunCommand:
		return RemedyOutcome{Plan: plan, Applied: false, Error: msgCommandRequires}
	default:
		return RemedyOutcome{Plan: plan, Applied: false, Error: "unrecognized action type"}
	}
}

func (e *DiagnosticsRemedyEngine) isTargetInsideZQK(cleanTarget string) bool {
	zqkPrefix := filepath.Clean(filepath.Join(e.ProjectRoot, paths.ProjectDataDir))
	return strings.HasPrefix(cleanTarget, zqkPrefix)
}

func (e *DiagnosticsRemedyEngine) applyRemoveFile(plan RemedyPlan) RemedyOutcome {
	cleanTarget := filepath.Clean(plan.Target)
	if !e.isTargetInsideZQK(cleanTarget) {
		return RemedyOutcome{Plan: plan, Error: errTargetOutsideZQK}
	}
	if strings.Contains(filepath.ToSlash(cleanTarget), "/state/daemon_locks/") {
		return RemedyOutcome{Plan: plan, Applied: false, Detail: msgRefuseDaemonLock}
	}
	if err := fileutil.Remove(cleanTarget); err != nil && !fileutil.IsNotExist(err) {
		return RemedyOutcome{Plan: plan, Error: err.Error()}
	}
	return RemedyOutcome{Plan: plan, Applied: true, Detail: "removed " + filepath.Base(cleanTarget)}
}

func (e *DiagnosticsRemedyEngine) applyTouchFile(plan RemedyPlan) RemedyOutcome {
	cleanTarget := filepath.Clean(plan.Target)
	if !e.isTargetInsideZQK(cleanTarget) {
		return RemedyOutcome{Plan: plan, Error: errTargetOutsideZQK}
	}
	f, err := fileutil.OpenFile(cleanTarget, fileutil.O_RDWR|fileutil.O_CREATE, paths.FilePerm644)
	if err != nil {
		return RemedyOutcome{Plan: plan, Error: err.Error()}
	}
	closeErr := f.Close()
	currentTime := time.Now().Local()
	timeErr := fileutil.Chtimes(cleanTarget, currentTime, currentTime)
	if closeErr != nil {
		return RemedyOutcome{Plan: plan, Error: closeErr.Error()}
	}
	if timeErr != nil {
		return RemedyOutcome{Plan: plan, Error: timeErr.Error()}
	}
	return RemedyOutcome{Plan: plan, Applied: true, Detail: "touched " + filepath.Base(cleanTarget)}
}

func (e *DiagnosticsRemedyEngine) applySeed(plan RemedyPlan) RemedyOutcome {
	switch plan.Target {
	case targetPolicies:
		if e.PolicySeeder == nil {
			return RemedyOutcome{Plan: plan, Applied: false, Detail: msgNoSeeder}
		}
		n, err := e.PolicySeeder(e.ProjectRoot)
		if err != nil {
			return RemedyOutcome{Plan: plan, Error: err.Error()}
		}
		return RemedyOutcome{Plan: plan, Applied: true, Detail: fmt.Sprintf("seeded %d policies", n)}
	case targetPersonas:
		if e.PersonaSeeder == nil {
			return RemedyOutcome{Plan: plan, Applied: false, Detail: msgNoSeeder}
		}
		n, err := e.PersonaSeeder(e.ProjectRoot)
		if err != nil {
			return RemedyOutcome{Plan: plan, Error: err.Error()}
		}
		return RemedyOutcome{Plan: plan, Applied: true, Detail: fmt.Sprintf("seeded %d personas", n)}
	default:
		return RemedyOutcome{Plan: plan, Applied: false, Detail: msgNoSeeder}
	}
}

func (e *DiagnosticsRemedyEngine) applyKillProcess(plan RemedyPlan) RemedyOutcome {
	cnt, reaped, err := resourcehygiene.ReapOrphanedProcesses(e.ProjectRoot, false)
	if err != nil {
		return RemedyOutcome{Plan: plan, Error: err.Error()}
	}
	return RemedyOutcome{
		Plan:    plan,
		Applied: true,
		Detail:  fmt.Sprintf("terminated %d orphaned processes: %s", cnt, strings.Join(reaped, ", ")),
	}
}

func dirHasYAML(dir string) bool {
	entries, err := fileutil.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() && (strings.HasSuffix(e.Name(), ".yaml") || strings.HasSuffix(e.Name(), ".yml")) {
			return true
		}
	}
	return false
}
