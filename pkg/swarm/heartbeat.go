package swarm

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"path/filepath"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/agentfeed"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/resourcehygiene"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/systemcheck/integrity"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/workflow/whatsnext"
)

// DefaultHeartbeatCadence is the default interval between heartbeat sweeps.
const DefaultHeartbeatCadence = 30 * time.Minute

// DefaultLockStaleHeartbeatAge is the default cutoff for abandoned lock files.
const DefaultLockStaleHeartbeatAge = 2 * time.Minute

// HeartbeatOptions holds parameters for running the swarm heartbeat cadence.
type HeartbeatOptions struct {
	ProjectRoot     string
	Cadence         time.Duration
	Once            bool
	DryRun          bool
	Storage         storage.ObjectStorageProvider
	SecurityContext *pkgctx.SecurityContext
}

// HeartbeatReport aggregates the outcome of a single heartbeat pulse.
type HeartbeatReport struct {
	Timestamp time.Time            `json:"timestamp"`
	Cadence   time.Duration        `json:"cadence"`
	Hygiene   HygieneSweepReport   `json:"hygiene"`
	Alignment AlignmentPulseReport `json:"alignment"`
	Wakeup    WakeupBusReport      `json:"wakeup"`
}

// HygieneSweepReport summarizes the automated hygiene sweep results.
type HygieneSweepReport struct {
	StaleLocksCleared int      `json:"stale_locks_cleared"`
	ClearedLockPaths  []string `json:"cleared_lock_paths,omitempty"`
	TempFilesCleaned  int      `json:"temp_files_cleaned"`
	CleanedTempPaths  []string `json:"cleaned_temp_paths,omitempty"`
	MembraneHealthy   bool     `json:"membrane_healthy"`
	OrphanObjectCount int      `json:"orphan_object_count"`
	OrphanAlerts      []string `json:"orphan_alerts,omitempty"`
}

// AlignmentPulseReport captures the refreshed ambient strategic alignment state.
type AlignmentPulseReport struct {
	AlignmentScore    float64 `json:"alignment_score"`
	GoalsCount        int     `json:"goals_count"`
	ItemsWithGoals    int     `json:"items_with_goals"`
	ItemsWithoutGoals int     `json:"items_without_goals"`
	CachePath         string  `json:"cache_path"`
}

// WakeupBusReport records deterministic wakeup event dispatch and work availability.
type WakeupBusReport struct {
	WakeupDispatched     bool     `json:"wakeup_dispatched"`
	EventID              string   `json:"event_id,omitempty"`
	UnblockedWorkCount   int      `json:"unblocked_work_count"`
	UnblockedWorkIDs     []string `json:"unblocked_work_ids,omitempty"`
	PendingApprovalCount int      `json:"pending_approval_count"`
	PendingApprovalIDs   []string `json:"pending_approval_ids,omitempty"`
	Message              string   `json:"message,omitempty"`
}

// RunHygieneSweep executes the automated hygiene sweep:
// 1. Clears stale .lock files in .zqk/lock/ whose lease heartbeat has expired.
// 2. Cleans orphaned temporary files (*.tmp) in .zqk/metrics/ and .zqk/process/.
// 3. Validates CAS membrane integrity and alerts on orphan objects.
func RunHygieneSweep(ctx context.Context, projectRoot string, store storage.ObjectStorageProvider, sec *pkgctx.SecurityContext, lockMaxAge time.Duration, dryRun bool) (HygieneSweepReport, error) {
	report := HygieneSweepReport{
		MembraneHealthy:  true,
		ClearedLockPaths: make([]string, 0),
		CleanedTempPaths: make([]string, 0),
		OrphanAlerts:     make([]string, 0),
	}

	if projectRoot == "" {
		return report, errfmt.Errorf("project root cannot be empty")
	}
	if lockMaxAge <= 0 {
		lockMaxAge = DefaultLockStaleHeartbeatAge
	}

	// 1. Clear stale .lock files in .zqk/lock/ (and scheduler locks if present)
	lockDirs := []string{
		filepath.Join(projectRoot, paths.ProjectDataDir, paths.LockDir),
		filepath.Join(projectRoot, paths.ProjectDataDir, paths.SchedulerDir, paths.SchedulerLocksDir),
	}
	for _, dir := range lockDirs {
		entries, err := fileutil.ReadDir(dir)
		if err != nil || len(entries) == 0 {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".lock") {
				continue
			}
			lockPath := filepath.Join(dir, e.Name())
			if dryRun {
				report.StaleLocksCleared++
				report.ClearedLockPaths = append(report.ClearedLockPaths, lockPath)
				continue
			}
			info, iErr := e.Info()
			if iErr == nil && time.Since(info.ModTime()) > lockMaxAge {
				if errRem := fileutil.Remove(lockPath); errRem == nil {
					report.StaleLocksCleared++
					report.ClearedLockPaths = append(report.ClearedLockPaths, lockPath)
				}
			}
		}
	}

	// 2. Clean orphaned temporary files (*.tmp) in .zqk/metrics/ and .zqk/process/
	cleanDirs := []string{
		filepath.Join(projectRoot, paths.ProjectDataDir, "metrics"),
		filepath.Join(projectRoot, paths.ProjectDataDir, paths.ProcessDir),
	}
	for _, dir := range cleanDirs {
		if !fileutil.Exists(dir) {
			continue
		}
		var cleanDir func(curr string)
		cleanDir = func(curr string) {
			entries, err := fileutil.ReadDir(curr)
			if err != nil {
				return
			}
			for _, e := range entries {
				full := filepath.Join(curr, e.Name())
				if e.IsDir() {
					cleanDir(full)
					continue
				}
				name := e.Name()
				if strings.HasSuffix(name, ".tmp") || strings.Contains(name, ".tmp.") || resourcehygiene.IsTempFileName(name) {
					report.TempFilesCleaned++
					report.CleanedTempPaths = append(report.CleanedTempPaths, full)
					if !dryRun {
						_ = fileutil.Remove(full)
					}
				}
			}
		}
		cleanDir(dir)
	}

	// 3. Validate CAS membrane integrity and alert on orphan objects
	if store != nil {
		if sec == nil {
			sec = pkgctx.NewSystemSecurityContext()
		}

		// Check kernel integrity and membrane coverage
		integrityReport, intErr := integrity.BuildKernelIntegrityReport(ctx, projectRoot, store, sec)
		if intErr != nil {
			report.MembraneHealthy = false
			report.OrphanAlerts = append(report.OrphanAlerts, fmt.Sprintf("CAS integrity inspection error: %v", intErr))
		} else if !integrityReport.MembraneHealthy {
			report.MembraneHealthy = false
			if len(integrityReport.MembraneCoverageGaps) > 0 {
				report.OrphanAlerts = append(report.OrphanAlerts, fmt.Sprintf("Membrane coverage gaps: %s", strings.Join(integrityReport.MembraneCoverageGaps, ", ")))
			}
		}

		// Scan dangling references
		danglingHits := integrity.ScanDanglingRefs(ctx, sec, store, 50)
		for _, hit := range danglingHits {
			report.OrphanAlerts = append(report.OrphanAlerts, fmt.Sprintf("Dangling reference in %s (%s): %s -> missing %s", hit.ObjectID, hit.ObjectKind, hit.Field, hit.MissingRef))
		}

		// Detect orphan backlog items without parent goal or plan reference
		resBLI, errBLI := store.List(ctx, sec, nil, storage.ListFilter{
			Kind: objects.KindBacklogItem,
			Filters: map[string]any{
				objects.FieldKeyStatus: map[string]any{
					"$ne": objects.ObjectStatusArchived,
				},
			},
			Fields: []string{objects.FieldKeyID, objects.FieldKeyTitle, objects.FieldKeyGoalRefs, "goal_ref", objects.FieldKeyPriorityPlanRef},
			Limit:  5000,
		})
		if errBLI == nil && resBLI != nil {
			for _, obj := range resBLI.Objects {
				id, _ := obj[objects.FieldKeyID].(string)
				gRefs := extractGoalRefs(obj)
				planRef, _ := obj[objects.FieldKeyPriorityPlanRef].(string)
				if len(gRefs) == 0 && strings.TrimSpace(planRef) == "" {
					report.OrphanAlerts = append(report.OrphanAlerts, fmt.Sprintf("Orphan backlog_item %s: no goal_refs or priority_plan_ref assigned", id))
				}
			}
		}

		// Detect orphan criteria without requirement reference
		resCrit, errCrit := store.List(ctx, sec, nil, storage.ListFilter{
			Kind: objects.KindCriteria,
			Filters: map[string]any{
				objects.FieldKeyStatus: map[string]any{
					"$ne": objects.ObjectStatusArchived,
				},
			},
			Fields: []string{objects.FieldKeyID, objects.FieldKeyRequirementRefs, "requirement_ref"},
			Limit:  5000,
		})
		if errCrit == nil && resCrit != nil {
			for _, obj := range resCrit.Objects {
				id, _ := obj[objects.FieldKeyID].(string)
				reqRef, _ := obj["requirement_ref"].(string)
				reqRefs := parseStringSlice(obj[objects.FieldKeyRequirementRefs])
				if strings.TrimSpace(reqRef) == "" && len(reqRefs) == 0 {
					report.OrphanAlerts = append(report.OrphanAlerts, fmt.Sprintf("Orphan criteria %s: missing requirement_refs", id))
				}
			}
		}

		report.OrphanObjectCount = len(report.OrphanAlerts)
		if report.OrphanObjectCount > 0 {
			report.MembraneHealthy = false
		}
	}

	return report, nil
}

// RunAlignmentPulse recalculates the strategic alignment score and persists the snapshot to
// .zqk/state/ambient/align-latest.json.
func RunAlignmentPulse(ctx context.Context, projectRoot string, store storage.ObjectStorageProvider, sec *pkgctx.SecurityContext, dryRun bool) (AlignmentPulseReport, error) {
	report := AlignmentPulseReport{
		AlignmentScore: 100.0,
		CachePath:      filepath.Join(projectRoot, paths.ProjectDataDir, whatsnext.AlignLatestRelativePath),
	}
	if projectRoot == "" {
		return report, errfmt.Errorf("project root cannot be empty")
	}

	if store == nil {
		return report, nil
	}
	if sec == nil {
		sec = pkgctx.NewSystemSecurityContext()
	}

	// 1. List active backlog items
	listResult, err := store.List(ctx, sec, nil, storage.ListFilter{
		Kind: objects.KindBacklogItem,
		Filters: map[string]any{
			objects.FieldKeyStatus: map[string]any{
				"$ne": objects.ObjectStatusArchived,
			},
		},
		Fields: []string{objects.FieldKeyID, objects.FieldKeyStatus, objects.FieldKeyGoalRefs, "goal_ref", objects.FieldKeyRequirementRefs, "requirement_ref"},
		Limit:  10000,
	})
	if err != nil {
		return report, errfmt.Newf("list backlog_item for alignment").Wrap(err)
	}

	// 2. List active goals
	goalList, _ := store.List(ctx, sec, nil, storage.ListFilter{
		Kind: objects.KindGoal,
		Filters: map[string]any{
			objects.FieldKeyStatus: map[string]any{
				"$ne": objects.ObjectStatusArchived,
			},
		},
		Fields: []string{objects.FieldKeyID},
		Limit:  5000,
	})
	goalsCount := 0
	if goalList != nil {
		goalsCount = len(goalList.Objects)
	}

	// 2b. Pre-fetch requirements for transitive goal resolution (GH-981)
	reqList, _ := store.List(ctx, sec, nil, storage.ListFilter{
		Kind: objects.KindRequirement,
		Filters: map[string]any{
			objects.FieldKeyStatus: map[string]any{
				"$ne": objects.ObjectStatusArchived,
			},
		},
		Fields: []string{objects.FieldKeyID, objects.FieldKeyGoalRefs, "goal_ref"},
		Limit:  10000,
	})
	reqToGoalRefs := make(map[string][]string)
	if reqList != nil {
		for _, robj := range reqList.Objects {
			if rid, ok := robj[objects.FieldKeyID].(string); ok && rid != "" {
				g := objects.ExtractGoalRefs(robj)
				if len(g) > 0 {
					reqToGoalRefs[rid] = g
				}
			}
		}
	}

	total := len(listResult.Objects)
	withCount := 0
	for _, obj := range listResult.Objects {
		grefs := objects.ExtractGoalRefs(obj)
		if len(grefs) == 0 {
			reqRefs := parseStringSlice(obj[objects.FieldKeyRequirementRefs])
			if singleReq, ok := obj["requirement_ref"].(string); ok && strings.TrimSpace(singleReq) != "" {
				reqRefs = append(reqRefs, strings.TrimSpace(singleReq))
			}
			for _, rID := range reqRefs {
				if parentGoals, ok := reqToGoalRefs[rID]; ok && len(parentGoals) > 0 {
					grefs = parentGoals
					break
				}
			}
		}
		if len(grefs) > 0 {
			withCount++
		}
	}
	gapCount := total - withCount

	var coveragePct float64
	if total > 0 {
		coveragePct = math.Round((100.0*float64(withCount)/float64(total))*100) / 100
	} else {
		coveragePct = 100.0
	}

	report.AlignmentScore = coveragePct
	report.GoalsCount = goalsCount
	report.ItemsWithGoals = withCount
	report.ItemsWithoutGoals = gapCount

	// 3. Persist alignment snapshot into .zqk/state/ambient/align-latest.json
	snapshot := map[string]any{
		"alignment": map[string]any{
			"goal_work": map[string]any{
				"total_work_items":    total,
				"items_with_goals":    withCount,
				"items_without_goals": gapCount,
				"goal_coverage_pct":   coveragePct,
			},
			"alignment_score": coveragePct,
		},
		"alignment_score": coveragePct,
		"goals_count":     goalsCount,
		"measured_at":     time.Now().UTC().Format(time.RFC3339),
	}

	if !dryRun {
		cacheDir := filepath.Dir(report.CachePath)
		if mkErr := fileutil.MkdirAll(cacheDir, paths.DirPerm755); mkErr != nil {
			return report, errfmt.Newf("create ambient alignment directory").Wrap(mkErr)
		}
		data, mErr := json.MarshalIndent(snapshot, "", "  ")
		if mErr != nil {
			return report, errfmt.Newf("marshal ambient alignment snapshot").Wrap(mErr)
		}
		if wErr := fileutil.WriteFile(report.CachePath, data, paths.FilePerm644); wErr != nil {
			return report, errfmt.Newf("write ambient alignment snapshot").Wrap(wErr)
		}
	}

	return report, nil
}

// RunWakeupBus checks for unblocked work items and pending approvals, then dispatches a deterministic
// wakeup event onto the agent feed bus.
func RunWakeupBus(ctx context.Context, projectRoot string, store storage.ObjectStorageProvider, sec *pkgctx.SecurityContext, dryRun bool) (WakeupBusReport, error) {
	report := WakeupBusReport{
		UnblockedWorkIDs:   make([]string, 0),
		PendingApprovalIDs: make([]string, 0),
	}
	if projectRoot == "" {
		return report, errfmt.Errorf("project root cannot be empty")
	}

	if store != nil {
		if sec == nil {
			sec = pkgctx.NewSystemSecurityContext()
		}

		// Check for unblocked work (active/planned BLIs without unresolved blockers)
		bliList, err := store.List(ctx, sec, nil, storage.ListFilter{
			Kind: objects.KindBacklogItem,
			Filters: map[string]any{
				objects.FieldKeyStatus: map[string]any{
					"$in": []any{
						objects.ObjectStatusPlanned,
						objects.ObjectStatusInProgress,
						objects.ObjectStatusActive,
					},
				},
			},
			Fields: []string{objects.FieldKeyID, objects.FieldKeyStatus, objects.FieldKeyBlockers, objects.FieldKeyRiskBlockerRefs, "blocker_refs"},
			Limit:  1000,
		})
		if err == nil && bliList != nil {
			for _, obj := range bliList.Objects {
				id, _ := obj[objects.FieldKeyID].(string)
				blockers := parseStringSlice(obj[objects.FieldKeyBlockers])
				if len(blockers) == 0 {
					blockers = parseStringSlice(obj[objects.FieldKeyRiskBlockerRefs])
				}
				if len(blockers) == 0 {
					blockers = parseStringSlice(obj["blocker_refs"])
				}
				if len(blockers) == 0 {
					report.UnblockedWorkIDs = append(report.UnblockedWorkIDs, id)
				}
			}
		}
		report.UnblockedWorkCount = len(report.UnblockedWorkIDs)

		// Check for pending approvals (objects with pending_approval status or TDE envelopes)
		approvalList, errApp := store.List(ctx, sec, nil, storage.ListFilter{
			Filters: map[string]any{
				objects.FieldKeyStatus: map[string]any{
					"$in": []any{
						"pending_approval",
						"blocked_pending_approval",
						"pending",
					},
				},
			},
			Fields: []string{objects.FieldKeyID, objects.FieldKeyStatus},
			Limit:  500,
		})
		if errApp == nil && approvalList != nil {
			for _, obj := range approvalList.Objects {
				id, _ := obj[objects.FieldKeyID].(string)
				if id != "" {
					report.PendingApprovalIDs = append(report.PendingApprovalIDs, id)
				}
			}
		}
		report.PendingApprovalCount = len(report.PendingApprovalIDs)
	}

	report.Message = fmt.Sprintf("[SWARM HEARTBEAT] Cadence pulse: %d unblocked work item(s), %d pending approval(s)",
		report.UnblockedWorkCount, report.PendingApprovalCount)

	if !dryRun {
		// Deterministic feed append
		appendRes, appErr := agentfeed.AppendEvent(agentfeed.AppendEventInput{
			ProjectRoot:      projectRoot,
			Message:          report.Message,
			AgentID:          agentfeed.FeedKernelAgentID,
			EventType:        agentfeed.FeedEventTypeWake,
			Sender:           "swarm_heartbeat",
			SkipEnabledCheck: true,
		})
		if appErr == nil {
			report.WakeupDispatched = true
			report.EventID = appendRes.EventID
		}

		// Broadcast peer wake signal to active mesh seats
		_ = agentfeed.WakePeer(ctx, projectRoot, report.Message)
	} else {
		report.WakeupDispatched = true
		report.EventID = "dry-run-wake-event"
	}

	return report, nil
}

// RunHeartbeatPulse performs a single unified cadence execution cycle.
func RunHeartbeatPulse(ctx context.Context, opts HeartbeatOptions) (*HeartbeatReport, error) {
	if opts.ProjectRoot == "" {
		opts.ProjectRoot = paths.ResolveProjectRoot(".")
	}
	if opts.Cadence <= 0 {
		opts.Cadence = DefaultHeartbeatCadence
	}
	if opts.SecurityContext == nil {
		opts.SecurityContext = pkgctx.NewSystemSecurityContext()
	}

	report := &HeartbeatReport{
		Timestamp: time.Now().UTC(),
		Cadence:   opts.Cadence,
	}

	// 1. Automated Hygiene Sweep
	hygiene, err := RunHygieneSweep(ctx, opts.ProjectRoot, opts.Storage, opts.SecurityContext, opts.Cadence, opts.DryRun)
	if err != nil {
		return nil, errfmt.Newf("hygiene sweep failed").Wrap(err)
	}
	report.Hygiene = hygiene

	// 2. Ambient Alignment Pulse
	alignment, err := RunAlignmentPulse(ctx, opts.ProjectRoot, opts.Storage, opts.SecurityContext, opts.DryRun)
	if err != nil {
		return nil, errfmt.Newf("alignment pulse failed").Wrap(err)
	}
	report.Alignment = alignment

	// 3. Reactive Wakeup Bus
	wakeup, err := RunWakeupBus(ctx, opts.ProjectRoot, opts.Storage, opts.SecurityContext, opts.DryRun)
	if err != nil {
		return nil, errfmt.Newf("wakeup bus failed").Wrap(err)
	}
	report.Wakeup = wakeup

	return report, nil
}

// RunHeartbeatLoop executes continuous or one-shot heartbeat pulses.
func RunHeartbeatLoop(ctx context.Context, opts HeartbeatOptions, onPulse func(*HeartbeatReport)) error {
	if opts.Cadence <= 0 {
		opts.Cadence = DefaultHeartbeatCadence
	}

	pulse, err := RunHeartbeatPulse(ctx, opts)
	if err != nil {
		return err
	}
	if onPulse != nil {
		onPulse(pulse)
	}

	if opts.Once {
		return nil
	}

	ticker := time.NewTicker(opts.Cadence)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			pulse, err := RunHeartbeatPulse(ctx, opts)
			if err != nil {
				return err
			}
			if onPulse != nil {
				onPulse(pulse)
			}
		}
	}
}

func extractGoalRefs(obj map[string]any) []string {
	return objects.ExtractGoalRefs(obj)
}

func parseStringSlice(v any) []string {
	if v == nil {
		return nil
	}
	switch val := v.(type) {
	case string:
		if strings.TrimSpace(val) == "" {
			return nil
		}
		return []string{strings.TrimSpace(val)}
	case []string:
		return val
	case []any:
		var res []string
		for _, item := range val {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				res = append(res, strings.TrimSpace(s))
			}
		}
		return res
	default:
		return nil
	}
}
