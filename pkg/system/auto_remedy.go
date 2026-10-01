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
	var plans []RemedyPlan

	if e.ProjectRoot == "" {
		return plans, nil
	}

	// 1. Diagnose stale locks and orphaned temp files from I/O resource hygiene
	ioTel, err := resourcehygiene.InspectIOResources(ctx, e.ProjectRoot)
	if err == nil && ioTel != nil {
		for _, lockPath := range ioTel.StaleLockPaths {
			fullPath := lockPath
			if !filepath.IsAbs(fullPath) {
				fullPath = filepath.Join(e.ProjectRoot, lockPath)
			}
			plans = append(plans, RemedyPlan{
				ID:          fmt.Sprintf("REMEDY-STALE-LOCK-%s", filepath.Base(lockPath)),
				Title:       fmt.Sprintf("Clean Stale Lock File (%s)", filepath.Base(lockPath)),
				Description: fmt.Sprintf("Remove abandoned lock file: %s", lockPath),
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
			plans = append(plans, RemedyPlan{
				ID:          fmt.Sprintf("REMEDY-ORPHAN-TMP-%s", filepath.Base(tmpPath)),
				Title:       fmt.Sprintf("Clean Orphaned Temp File (%s)", filepath.Base(tmpPath)),
				Description: fmt.Sprintf("Remove abandoned temp file: %s", tmpPath),
				ActionType:  ActionRemoveFile,
				Target:      fullPath,
				Confidence:  1.0,
				AutoApply:   true,
			})
		}

		for _, orphanProc := range ioTel.OrphanedProcesses {
			plans = append(plans, RemedyPlan{
				ID:          fmt.Sprintf("REMEDY-ORPHAN-PROC-%s", strings.ReplaceAll(orphanProc, " ", "-")),
				Title:       fmt.Sprintf("Terminate Orphaned Process (%s)", orphanProc),
				Description: fmt.Sprintf("Terminate orphaned process detached under PID 1: %s", orphanProc),
				ActionType:  ActionKillProcess,
				Target:      orphanProc,
				Confidence:  1.0,
				AutoApply:   true,
			})
		}
	}

	// 2. Diagnose Seeding Preconditions
	processDir := filepath.Join(e.ProjectRoot, paths.ProcessDir)
	policyDir := filepath.Join(processDir, "policies")
	if !dirHasYAML(policyDir) && !dirHasYAML(filepath.Join(processDir, "policy")) {
		plans = append(plans, RemedyPlan{
			ID:          "REMEDY-UNSEEDED-POLICIES",
			Title:       "Seed Default Policy Pack",
			Description: "No policy objects found in .zqk/process/policies",
			ActionType:  ActionSeed,
			Target:      "policies",
			Command:     paths.CLICommandName + " system init --force",
			Confidence:  1.0,
			AutoApply:   e.PolicySeeder != nil,
		})
	}

	personaDir := filepath.Join(processDir, "personas")
	if !dirHasYAML(personaDir) && !dirHasYAML(filepath.Join(processDir, "persona")) {
		plans = append(plans, RemedyPlan{
			ID:          "REMEDY-UNSEEDED-PERSONAS",
			Title:       "Seed Default Agent Seating Pack",
			Description: "No persona objects found in .zqk/process/personas",
			ActionType:  ActionSeed,
			Target:      "personas",
			Command:     paths.CLICommandName + " system agent-onboard",
			Confidence:  1.0,
			AutoApply:   e.PersonaSeeder != nil,
		})
	}

	// 3. Diagnose fixable check issues
	for _, cr := range checkResults {
		for _, issue := range cr.Issues {
			if issue.FixCommand != "" {
				plans = append(plans, RemedyPlan{
					ID:          fmt.Sprintf("REMEDY-ISSUE-%s", cr.ObjectID),
					Title:       fmt.Sprintf("Fix validation issue for %s", cr.ObjectID),
					Description: issue.Message,
					ActionType:  ActionRunCommand,
					Target:      cr.ObjectID,
					Command:     issue.FixCommand,
					Confidence:  0.95,
					AutoApply:   false, // External commands require explicit confirmation or runner
				})
			} else if issue.AutoFixable {
				plans = append(plans, RemedyPlan{
					ID:          fmt.Sprintf("REMEDY-AUTOFIX-%s", cr.ObjectID),
					Title:       fmt.Sprintf("Apply autofix for %s", cr.ObjectID),
					Description: issue.Message,
					ActionType:  ActionRunCommand,
					Target:      cr.ObjectID,
					Command:     fmt.Sprintf("%s object fix %s", paths.CLICommandName, cr.ObjectID),
					Confidence:  0.90,
					AutoApply:   false,
				})
			}
		}
	}

	return plans, nil
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
				Detail:  "requires manual execution or external confirmation",
			})
			continue
		}

		outcome := RemedyOutcome{
			Plan: plan,
		}

		switch plan.ActionType {
		case ActionRemoveFile:
			cleanTarget := filepath.Clean(plan.Target)
			zqkPrefix := filepath.Clean(filepath.Join(e.ProjectRoot, paths.ProjectDataDir))
			if strings.HasPrefix(cleanTarget, zqkPrefix) {
				if strings.Contains(filepath.ToSlash(cleanTarget), "/state/daemon_locks/") {
					outcome.Applied = false
					outcome.Detail = "refusing to remove daemon singleton lock file"
					continue
				}
				if err := fileutil.Remove(cleanTarget); err != nil && !fileutil.IsNotExist(err) {
					outcome.Error = err.Error()
				} else {
					outcome.Applied = true
					outcome.Detail = fmt.Sprintf("removed %s", filepath.Base(cleanTarget))
					report.TotalApplied++
				}
			} else {
				outcome.Error = "target file outside .zqk directory"
			}

		case ActionTouchFile:
			cleanTarget := filepath.Clean(plan.Target)
			zqkPrefix := filepath.Clean(filepath.Join(e.ProjectRoot, paths.ProjectDataDir))
			if strings.HasPrefix(cleanTarget, zqkPrefix) {
				f, err := fileutil.OpenFile(cleanTarget, fileutil.O_RDWR|fileutil.O_CREATE, paths.FilePerm644)
				if err != nil {
					outcome.Error = err.Error()
				} else {
					_ = f.Close()
					currentTime := time.Now().Local()
					_ = fileutil.Chtimes(cleanTarget, currentTime, currentTime)
					outcome.Applied = true
					outcome.Detail = fmt.Sprintf("touched %s", filepath.Base(cleanTarget))
					report.TotalApplied++
				}
			} else {
				outcome.Error = "target file outside .zqk directory"
			}

		case ActionSeed:
			if plan.Target == "policies" && e.PolicySeeder != nil {
				n, err := e.PolicySeeder(e.ProjectRoot)
				if err != nil {
					outcome.Error = err.Error()
				} else {
					outcome.Applied = true
					outcome.Detail = fmt.Sprintf("seeded %d policies", n)
					report.TotalApplied++
				}
			} else if plan.Target == "personas" && e.PersonaSeeder != nil {
				n, err := e.PersonaSeeder(e.ProjectRoot)
				if err != nil {
					outcome.Error = err.Error()
				} else {
					outcome.Applied = true
					outcome.Detail = fmt.Sprintf("seeded %d personas", n)
					report.TotalApplied++
				}
			} else {
				outcome.Applied = false
				outcome.Detail = "no seeder registered"
			}

		case ActionKillProcess:
			cnt, reaped, err := resourcehygiene.ReapOrphanedProcesses(e.ProjectRoot, false)
			if err != nil {
				outcome.Error = err.Error()
			} else {
				outcome.Applied = true
				outcome.Detail = fmt.Sprintf("terminated %d orphaned processes: %s", cnt, strings.Join(reaped, ", "))
				report.TotalApplied++
			}

		case ActionRunCommand:
			outcome.Applied = false
			outcome.Error = "command action requires runner execution"
		}

		report.Outcomes = append(report.Outcomes, outcome)
	}

	return report, nil
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
