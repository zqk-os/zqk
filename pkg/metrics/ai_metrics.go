package metrics

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage" // Storage provider interface for metrics calculation
)

const (
	backlogStatusComplete = "complete"

	scoreWeightComplete   = 1.0
	scoreWeightInProgress = 0.5
	scoreWeightPlanned    = 0.25
	scoreMax              = 100.0
	scoreMin              = 0.0

	freqHealthyMin = 0.5
	freqHealthyMax = 2.0
	freqLow        = 0.1
	freqHigh       = 5.0

	pcsBoostHealthy    = 5.0
	pcsDropLowFreq     = 10.0
	pcsDropHighFreq    = 5.0
	pcsBoostIncreasing = 3.0
	pcsDropDecreasing  = 5.0
	pcsDropIrregular   = 3.0

	ratioLow        = 1.0
	ratioHigh       = 10.0
	ratioHealthyMin = 2.0
	ratioHealthyMax = 5.0

	eddAdjustUnderestimated = 10.0
	eddAdjustOverestimated  = 15.0
	eddAdjustHealthy        = 2.0
)

// ProjectMetrics contains calculated AI metrics for a project
type ProjectMetrics struct {
	PCS float64 // Project Confidence Score (0-100)
	EDD float64 // Effort Distribution Discrepancy (percentage variance)
	DB  *DependenciesBlockers
}

// DependenciesBlockers contains dependency and blocker information
type DependenciesBlockers struct {
	Dependencies []DependencyItem
	Blockers     []BlockerItem
	ActionItems  []ActionItem
	Summary      string
}

// DependencyItem represents a dependency
type DependencyItem struct {
	ID          string
	Type        string // "code", "work_item", "external"
	Description string
	Severity    string // "low", "medium", "high", "critical"
}

// BlockerItem represents a blocker
type BlockerItem struct {
	ID          string
	Type        string // "code", "work_item", "external"
	Description string
	Severity    string // "low", "medium", "high", "critical"
}

// ActionItem represents an action item
type ActionItem struct {
	ID          string
	Description string
	Priority    string // "low", "medium", "high", "critical"
}

// CommitMetrics contains aggregated commit statistics
type CommitMetrics struct {
	TotalCommits      int
	CommitsLast30Days int
	CommitsLast7Days  int
	AvgCommitsPerDay  float64
	CommitFrequency   float64 // Commits per day over last 30 days
	CommitPattern     string  // "increasing", "decreasing", "stable", "irregular"
	WorkItemRatio     float64 // Commits per work item
	LinesChanged      int
	FilesChanged      int
	Authors           []string
}

// CalculateProjectMetrics calculates PCS, EDD, and D&B metrics for a project
// with optional Git commit data integration. If includeCommitData is false,
// it will automatically include commit data if available.
func CalculateProjectMetrics(
	ctx context.Context,
	storageProvider storage.ObjectStorageProvider,
	secCtx *pkgctx.SecurityContext,
	projectID string,
	includeCommitData bool,
) (*ProjectMetrics, error) {
	metrics := &ProjectMetrics{
		DB: &DependenciesBlockers{},
	}

	// Calculate base metrics (without commit data)
	basePCS, err := calculateBasePCS(ctx, storageProvider, secCtx, projectID)
	if err != nil {
		return nil, errfmt.Newf("failed to calculate base PCS").Wrap(err)
	}

	baseEDD, err := calculateBaseEDD(ctx, storageProvider, secCtx, projectID)
	if err != nil {
		return nil, errfmt.Newf("failed to calculate base EDD").Wrap(err)
	}

	baseDB, err := calculateBaseDB(ctx, storageProvider, secCtx, projectID)
	if err != nil {
		return nil, errfmt.Newf("failed to calculate base D&B").Wrap(err)
	}

	metrics.DB = baseDB

	// Auto-detect commit data availability if not explicitly requested
	shouldIncludeCommitData := includeCommitData
	if !includeCommitData {
		// Check if commit data is available
		//nolint:errcheck // Error indicates commit data unavailable, default to false
		hasCommitData, _ := hasCommitDataAvailable(ctx, storageProvider, secCtx)
		shouldIncludeCommitData = hasCommitData
	}

	// Enhance with commit data if available and requested
	if shouldIncludeCommitData {
		commitMetrics, err := calculateCommitMetrics(ctx, storageProvider, secCtx, projectID)
		if err != nil {
			// Log error but continue with base metrics
			// This allows metrics to be calculated even if commit data is unavailable
			// Log warning but don't fail - commit metrics are optional
			// Note: This is in pkg/metrics, so we can't use cli context
			// Use system logger which writes to stderr
			logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
			logging.Fluent(logger).Warn("Failed to calculate commit metrics").
				WithError(err).
				Log()
			metrics.PCS = basePCS
			metrics.EDD = baseEDD
		} else {
			// Enhance PCS with commit frequency and patterns
			metrics.PCS = enhancePCSWithCommits(basePCS, commitMetrics)

			// Enhance EDD with commit-to-work-item ratios
			metrics.EDD = enhanceEDDWithCommits(baseEDD, commitMetrics)

			// Enhance D&B with code-level dependencies from commits
			metrics.DB = enhanceDBWithCommits(baseDB, commitMetrics, ctx, storageProvider, secCtx, projectID)
		}
	} else {
		metrics.PCS = basePCS
		metrics.EDD = baseEDD
	}

	return metrics, nil
}

// hasCommitDataAvailable checks if commit data (code_reference objects) are available
func hasCommitDataAvailable(ctx context.Context, storageProvider storage.ObjectStorageProvider, secCtx *pkgctx.SecurityContext) (bool, error) {
	filter := storage.ListFilter{
		Kind:  objects.KindCodeReference,
		Limit: 1, // Just check if any exist
	}

	result, err := storageProvider.List(ctx, secCtx, pkgctx.NewStorageContext(), filter)
	if err != nil {
		return false, err
	}

	return len(result.Objects) > 0, nil
}

// calculateBasePCS calculates the base Project Confidence Score without commit data
func calculateBasePCS(ctx context.Context, storageProvider storage.ObjectStorageProvider, secCtx *pkgctx.SecurityContext, _ string) (float64, error) {
	// Base PCS calculation based on work item status distribution
	// This is a simplified version - can be enhanced with more factors
	filter := storage.ListFilter{
		Kind: objects.KindBacklogItem,
		Filters: map[string]any{
			objects.FieldKeyStatus: map[string]any{
				"$in": []string{
					backlogStatusComplete,
					objects.ObjectStatusInProgress,
					objects.ObjectStatusValidated,
					objects.ObjectStatusPlanned,
				},
			},
		},
	}

	result, err := storageProvider.List(ctx, secCtx, pkgctx.NewStorageContext(), filter)
	if err != nil {
		return 0, err
	}

	totalItems := len(result.Objects)
	if totalItems == 0 {
		return 50.0, nil // Default to neutral score
	}

	completeCount := 0
	inProgressCount := 0
	plannedCount := 0

	for _, obj := range result.Objects {
		status, _ := obj[objects.FieldKeyStatus].(string)
		switch status {
		case backlogStatusComplete:
			completeCount++
		case objects.ObjectStatusInProgress, objects.ObjectStatusValidated:
			inProgressCount++
		case objects.ObjectStatusPlanned:
			plannedCount++
		}
	}

	// Calculate score: weight completed items higher, in-progress medium, planned lower
	score := (float64(completeCount)*scoreWeightComplete + float64(inProgressCount)*scoreWeightInProgress + float64(plannedCount)*scoreWeightPlanned) / float64(totalItems) * scoreMax

	return math.Min(scoreMax, math.Max(scoreMin, score)), nil
}

// calculateBaseEDD calculates the base Effort Distribution Discrepancy without commit data
//
//nolint:unparam // Always returns 0 - function is a placeholder for future implementation
func calculateBaseEDD(ctx context.Context, storageProvider storage.ObjectStorageProvider, secCtx *pkgctx.SecurityContext, _ string) (float64, error) {
	// Base EDD calculation based on estimated vs actual effort
	// This is a simplified version - can be enhanced with actual effort tracking
	filter := storage.ListFilter{
		Kind: objects.KindBacklogItem,
		Filters: map[string]any{
			objects.FieldKeyStatus: backlogStatusComplete,
		},
	}

	result, err := storageProvider.List(ctx, secCtx, pkgctx.NewStorageContext(), filter)
	if err != nil {
		return 0, err
	}

	if len(result.Objects) == 0 {
		return 0.0, nil // No variance if no completed items
	}

	// For now, return 0 (no discrepancy) - this should be enhanced with actual effort tracking
	// TODO: Calculate based on estimated_effort vs actual effort
	return 0.0, nil
}

// ComputeEffortVariance calculates the percentage variance between estimated and actual effort for a single object.
func ComputeEffortVariance(estimated, actual string) float64 {
	var est, act float64
	_ , _ = fmt.Sscanf(estimated, "%f", &est)
	_ , _ = fmt.Sscanf(actual, "%f", &act)
	if est == 0 {
		return 0.0
	}
	return ((act - est) / est) * 100.0
}

// calculateBaseDB calculates the base Dependencies & Blockers without commit data
func calculateBaseDB(ctx context.Context, storageProvider storage.ObjectStorageProvider, secCtx *pkgctx.SecurityContext, _ string) (*DependenciesBlockers, error) {
	db := &DependenciesBlockers{
		Dependencies: []DependencyItem{},
		Blockers:     []BlockerItem{},
		ActionItems:  []ActionItem{},
	}

	// Find risk_blocker objects
	filter := storage.ListFilter{
		Kind: objects.KindRiskBlocker,
		Filters: map[string]any{
			objects.FieldKeyStatus: map[string]any{
				"$in": []string{"active", "open"},
			},
		},
	}

	result, err := storageProvider.List(ctx, secCtx, pkgctx.NewStorageContext(), filter)
	if err != nil {
		return db, err
	}

	for _, obj := range result.Objects {
		id, _ := obj[objects.FieldKeyID].(string)
		title, _ := obj[objects.FieldKeyTitle].(string)
		severity, _ := obj[objects.FieldKeySeverity].(string)
		blockerType, _ := obj[objects.FieldKeyType].(string)

		switch blockerType {
		case "blocker":
			db.Blockers = append(db.Blockers, BlockerItem{
				ID:          id,
				Type:        "work_item",
				Description: title,
				Severity:    severity,
			})
		case "dependency":
			db.Dependencies = append(db.Dependencies, DependencyItem{
				ID:          id,
				Type:        "work_item",
				Description: title,
				Severity:    severity,
			})
		}
	}

	db.Summary = generateDBSummary(db)

	return db, nil
}

// calculateCommitMetrics calculates commit-related metrics for a project
func calculateCommitMetrics(ctx context.Context, storageProvider storage.ObjectStorageProvider, secCtx *pkgctx.SecurityContext, _ string) (*CommitMetrics, error) {
	metrics := &CommitMetrics{
		Authors: []string{},
	}

	// Get all code_reference objects (these contain commit data)
	filter := storage.ListFilter{
		Kind: objects.KindCodeReference,
	}

	result, err := storageProvider.List(ctx, secCtx, pkgctx.NewStorageContext(), filter)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	thirtyDaysAgo := now.AddDate(0, 0, -30)
	sevenDaysAgo := now.AddDate(0, 0, -7)

	commitHashes := make(map[string]bool)
	authorSet := make(map[string]bool)

	for _, obj := range result.Objects {
		commitHash, _ := obj[objects.FieldKeyCommitHash].(string)
		if commitHash == emptyValue {
			continue
		}

		// Count unique commits
		if !commitHashes[commitHash] {
			commitHashes[commitHash] = true
			metrics.TotalCommits++

			// Check if commit is within last 30 days
			commitDateStr, _ := obj[objects.FieldKeyCommitDate].(string)
			if commitDateStr != emptyValue {
				commitDate, err := time.Parse(time.RFC3339, commitDateStr)
				if err == nil {
					if commitDate.After(thirtyDaysAgo) {
						metrics.CommitsLast30Days++
					}
					if commitDate.After(sevenDaysAgo) {
						metrics.CommitsLast7Days++
					}
				}
			}

			// Track authors
			author, _ := obj[objects.FieldKeyAuthor].(string)
			if author != emptyValue && !authorSet[author] {
				authorSet[author] = true
				metrics.Authors = append(metrics.Authors, author)
			}
		}

		// Aggregate lines changed (count for all code references, not just unique commits)
		// Handle both float64 and int types
		switch v := obj[objects.FieldKeyLinesAdded].(type) {
		case float64:
			metrics.LinesChanged += int(v)
		case int:
			metrics.LinesChanged += v
		case int64:
			metrics.LinesChanged += int(v)
		}
		switch v := obj[objects.FieldKeyLinesRemoved].(type) {
		case float64:
			metrics.LinesChanged += int(v)
		case int:
			metrics.LinesChanged += v
		case int64:
			metrics.LinesChanged += int(v)
		}
		// Count files changed (each code reference is a file change)
		metrics.FilesChanged++
	}

	// Calculate commit frequency
	if metrics.CommitsLast30Days > 0 {
		metrics.CommitFrequency = float64(metrics.CommitsLast30Days) / 30.0
		metrics.AvgCommitsPerDay = metrics.CommitFrequency
	}

	// Determine commit pattern (simplified - would need historical data for accuracy)
	if metrics.CommitsLast7Days > metrics.CommitsLast30Days/4 {
		metrics.CommitPattern = "increasing"
	} else if metrics.CommitsLast7Days < metrics.CommitsLast30Days/6 {
		metrics.CommitPattern = "decreasing"
	} else {
		metrics.CommitPattern = "stable"
	}

	// Calculate commit-to-work-item ratio
	workItemFilter := storage.ListFilter{
		Kind: objects.KindBacklogItem,
	}
	workItemResult, err := storageProvider.List(ctx, secCtx, pkgctx.NewStorageContext(), workItemFilter)
	if err == nil && len(workItemResult.Objects) > 0 {
		metrics.WorkItemRatio = float64(metrics.TotalCommits) / float64(len(workItemResult.Objects))
	}

	return metrics, nil
}

// enhancePCSWithCommits enhances PCS with commit frequency and patterns
func enhancePCSWithCommits(basePCS float64, commitMetrics *CommitMetrics) float64 {
	// Start with base PCS
	enhancedPCS := basePCS

	// Adjust based on commit frequency (more commits = higher confidence, up to a point)
	// Optimal range: 0.5-2 commits per day
	if commitMetrics.CommitFrequency > 0 {
		if commitMetrics.CommitFrequency >= freqHealthyMin && commitMetrics.CommitFrequency <= freqHealthyMax {
			// Healthy commit frequency: +5 points
			enhancedPCS += pcsBoostHealthy
		} else if commitMetrics.CommitFrequency < freqLow {
			// Very low commit frequency: -10 points
			enhancedPCS -= pcsDropLowFreq
		} else if commitMetrics.CommitFrequency > freqHigh {
			// Very high commit frequency (might indicate churn): -5 points
			enhancedPCS -= pcsDropHighFreq
		}
	}

	// Adjust based on commit pattern
	switch commitMetrics.CommitPattern {
	case "increasing":
		// Increasing commits: +3 points (positive momentum)
		enhancedPCS += pcsBoostIncreasing
	case "decreasing":
		// Decreasing commits: -5 points (losing momentum)
		enhancedPCS -= pcsDropDecreasing
	case "irregular":
		// Irregular pattern: -3 points (unpredictable)
		enhancedPCS -= pcsDropIrregular
	}

	// Clamp to 0-100 range
	return math.Min(scoreMax, math.Max(scoreMin, enhancedPCS))
}

// enhanceEDDWithCommits enhances EDD with commit-to-work-item ratios
func enhanceEDDWithCommits(baseEDD float64, commitMetrics *CommitMetrics) float64 {
	// Start with base EDD
	enhancedEDD := baseEDD

	// Adjust based on commit-to-work-item ratio
	// Optimal ratio: 2-5 commits per work item
	if commitMetrics.WorkItemRatio > 0 {
		if commitMetrics.WorkItemRatio < ratioLow {
			// Low ratio: work items may be under-scoped or not being worked on
			enhancedEDD += eddAdjustUnderestimated // Indicates underestimation
		} else if commitMetrics.WorkItemRatio > ratioHigh {
			// Very high ratio: work items may be over-scoped
			enhancedEDD -= eddAdjustOverestimated // Indicates overestimation
		} else if commitMetrics.WorkItemRatio >= ratioHealthyMin && commitMetrics.WorkItemRatio <= ratioHealthyMax {
			// Healthy ratio: slight positive adjustment
			enhancedEDD -= eddAdjustHealthy
		}
	}

	return enhancedEDD
}

// enhanceDBWithCommits enhances D&B with code-level dependencies from commits
func enhanceDBWithCommits(
	baseDB *DependenciesBlockers,
	commitMetrics *CommitMetrics,
	ctx context.Context,
	storageProvider storage.ObjectStorageProvider,
	secCtx *pkgctx.SecurityContext,
	_ string,
) *DependenciesBlockers {
	enhancedDB := &DependenciesBlockers{
		Dependencies: make([]DependencyItem, len(baseDB.Dependencies)),
		Blockers:     make([]BlockerItem, len(baseDB.Blockers)),
		ActionItems:  make([]ActionItem, len(baseDB.ActionItems)),
		Summary:      baseDB.Summary,
	}

	copy(enhancedDB.Dependencies, baseDB.Dependencies)
	copy(enhancedDB.Blockers, baseDB.Blockers)
	copy(enhancedDB.ActionItems, baseDB.ActionItems)

	// Detect code-level dependencies from commit patterns and file co-changes
	deps, blockers := detectCommitBasedDependencies(ctx, storageProvider, secCtx, commitMetrics)
	enhancedDB.Dependencies = append(enhancedDB.Dependencies, deps...)
	enhancedDB.Blockers = append(enhancedDB.Blockers, blockers...)

	// Detect dependencies from commit patterns
	if commitMetrics.CommitFrequency > 0 && commitMetrics.CommitPattern == "irregular" {
		// Irregular commit patterns might indicate dependency issues
		enhancedDB.Dependencies = append(enhancedDB.Dependencies, DependencyItem{
			ID:          "code-dependency-pattern",
			Type:        "code",
			Description: fmt.Sprintf("Irregular commit patterns detected (%.2f commits/day). May indicate code-level dependencies or coordination issues.", commitMetrics.CommitFrequency),
			Severity:    "medium",
		})
	}

	// If commit frequency is very low, might indicate blockers
	if commitMetrics.CommitFrequency < 0.1 && commitMetrics.TotalCommits > 0 {
		enhancedDB.Blockers = append(enhancedDB.Blockers, BlockerItem{
			ID:          "code-blocker-frequency",
			Type:        "code",
			Description: fmt.Sprintf("Very low commit frequency (%.2f commits/day). May indicate development blockers.", commitMetrics.CommitFrequency),
			Severity:    "high",
		})
	}

	// Detect potential blockers from work item ratio
	if commitMetrics.WorkItemRatio > 0 && commitMetrics.WorkItemRatio < 0.5 {
		enhancedDB.Blockers = append(enhancedDB.Blockers, BlockerItem{
			ID:          "code-blocker-workitem-ratio",
			Type:        "code",
			Description: fmt.Sprintf("Low commit-to-work-item ratio (%.2f). Work items may be blocked or under-scoped.", commitMetrics.WorkItemRatio),
			Severity:    "medium",
		})
	}

	enhancedDB.Summary = generateDBSummary(enhancedDB)

	return enhancedDB
}

// detectCommitBasedDependencies analyzes code_reference objects to detect
// code-level dependencies based on file co-changes and commit relationships
func detectCommitBasedDependencies(
	ctx context.Context,
	storageProvider storage.ObjectStorageProvider,
	secCtx *pkgctx.SecurityContext,
	commitMetrics *CommitMetrics,
) ([]DependencyItem, []BlockerItem) {
	var dependencies []DependencyItem
	var blockers []BlockerItem

	// Get all code_reference objects
	filter := storage.ListFilter{
		Kind: objects.KindCodeReference,
	}

	result, err := storageProvider.List(ctx, secCtx, pkgctx.NewStorageContext(), filter)
	if err != nil || len(result.Objects) == 0 {
		return dependencies, blockers
	}

	// Analyze file co-changes: files that are frequently changed together
	// indicate potential code dependencies
	fileCommitMap := make(map[string][]string) // file -> list of commit hashes
	commitFileMap := make(map[string][]string) // commit -> list of files

	for _, obj := range result.Objects {
		filePath, _ := obj[objects.FieldKeyFilePath].(string)
		commitHash, _ := obj[objects.FieldKeyCommitHash].(string)

		if filePath != emptyValue && commitHash != emptyValue {
			fileCommitMap[filePath] = append(fileCommitMap[filePath], commitHash)
			commitFileMap[commitHash] = append(commitFileMap[commitHash], filePath)
		}
	}

	// Detect files that are frequently changed together (co-change analysis)
	// If two files appear in the same commits frequently, they may be dependent
	filePairs := make(map[string]int) // "file1|file2" -> co-occurrence count
	for _, files := range commitFileMap {
		// For each commit, track pairs of files changed together
		for i := 0; i < len(files); i++ {
			for j := i + 1; j < len(files); j++ {
				// Create canonical pair key (alphabetically sorted)
				pair1 := fmt.Sprintf("%s|%s", files[i], files[j])
				pair2 := fmt.Sprintf("%s|%s", files[j], files[i])
				if files[i] < files[j] {
					filePairs[pair1]++
				} else {
					filePairs[pair2]++
				}
			}
		}
	}

	// Identify strong co-change relationships (potential dependencies)
	// Threshold: files changed together in at least 3 commits
	coChangeThreshold := 3
	if commitMetrics.TotalCommits < 10 {
		coChangeThreshold = 2 // Lower threshold for smaller projects
	}

	for pair, count := range filePairs {
		if count >= coChangeThreshold {
			parts := splitPair(pair)
			if len(parts) == 2 {
				dependencies = append(dependencies, DependencyItem{
					ID:          fmt.Sprintf("code-dep-cochange-%d", len(dependencies)+1),
					Type:        "code",
					Description: fmt.Sprintf("Files %s and %s are frequently changed together (%d commits), indicating potential code dependency.", parts[0], parts[1], count),
					Severity:    "medium",
				})
			}
		}
	}

	// Detect potential blockers: files with many commits but no recent activity
	// might indicate blocked work
	now := time.Now()
	thirtyDaysAgo := now.AddDate(0, 0, -30)

	for filePath, commits := range fileCommitMap {
		uniqueCommits := make(map[string]bool)
		for _, commit := range commits {
			uniqueCommits[commit] = true
		}

		// Check if file has many commits but no recent activity
		if len(uniqueCommits) >= 5 {
			// Check for recent commits in code references
			hasRecentCommit := false
			for _, obj := range result.Objects {
				objFile, _ := obj[objects.FieldKeyFilePath].(string)
				objCommit, _ := obj[objects.FieldKeyCommitHash].(string)
				if objFile == filePath {
					// Check if this commit is recent
					commitDateStr, _ := obj[objects.FieldKeyCommitDate].(string)
					if commitDateStr != emptyValue {
						commitDate, err := time.Parse(time.RFC3339, commitDateStr)
						if err == nil && commitDate.After(thirtyDaysAgo) {
							// Check if this commit hash is in our list
							if uniqueCommits[objCommit] {
								hasRecentCommit = true
								break
							}
						}
					}
				}
			}

			if !hasRecentCommit {
				blockers = append(blockers, BlockerItem{
					ID:          fmt.Sprintf("code-blocker-stale-%d", len(blockers)+1),
					Type:        "code",
					Description: fmt.Sprintf("File %s has %d commits but no recent activity (last 30 days), may indicate blocked work.", filePath, len(uniqueCommits)),
					Severity:    "low",
				})
			}
		}
	}

	return dependencies, blockers
}

// splitPair splits a "file1|file2" pair string into two file paths
func splitPair(pair string) []string {
	idx := strings.Index(pair, "|")
	if idx == -1 {
		return []string{pair}
	}
	return []string{pair[:idx], pair[idx+1:]}
}

// generateDBSummary generates a natural language summary of dependencies and blockers
func generateDBSummary(db *DependenciesBlockers) string {
	if len(db.Dependencies) == 0 && len(db.Blockers) == 0 {
		return "No dependencies or blockers identified."
	}

	summary := ""
	if len(db.Blockers) > 0 {
		criticalBlockers := 0
		highBlockers := 0
		for _, blocker := range db.Blockers {
			switch blocker.Severity {
			case "critical":
				criticalBlockers++
			case "high":
				highBlockers++
			}
		}

		if criticalBlockers > 0 {
			summary += fmt.Sprintf("%d critical blocker(s) identified. ", criticalBlockers)
		}
		if highBlockers > 0 {
			summary += fmt.Sprintf("%d high-priority blocker(s) identified. ", highBlockers)
		}
		if len(db.Blockers) > criticalBlockers+highBlockers {
			summary += fmt.Sprintf("%d additional blocker(s) present. ", len(db.Blockers)-criticalBlockers-highBlockers)
		}
	}

	if len(db.Dependencies) > 0 {
		criticalDeps := 0
		highDeps := 0
		for _, dep := range db.Dependencies {
			switch dep.Severity {
			case "critical":
				criticalDeps++
			case "high":
				highDeps++
			}
		}

		if criticalDeps > 0 {
			summary += fmt.Sprintf("%d critical dependency(ies) identified. ", criticalDeps)
		}
		if highDeps > 0 {
			summary += fmt.Sprintf("%d high-priority dependency(ies) identified. ", highDeps)
		}
		if len(db.Dependencies) > criticalDeps+highDeps {
			summary += fmt.Sprintf("%d additional dependency(ies) present. ", len(db.Dependencies)-criticalDeps-highDeps)
		}
	}

	return summary
}
