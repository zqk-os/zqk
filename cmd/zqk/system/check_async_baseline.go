package system

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/pkg/cliapp"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/migration/scanner"
	"github.com/zqk-os/zqk/pkg/nildecode"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/validation"
)

// AsyncBaselineMetrics contains async validator performance and result metrics
type AsyncBaselineMetrics struct {
	Timestamp       time.Time      `json:"timestamp"`
	TotalObjects    int            `json:"total_objects"`
	TotalResults    int            `json:"total_results"`
	Duration        time.Duration  `json:"duration_ms"`
	ObjectsPerSec   float64        `json:"objects_per_second"`
	IssuesByTier    map[int]int    `json:"issues_by_tier"`
	TotalIssues     int            `json:"total_issues"`
	ObjectKinds     map[string]int `json:"object_kinds"`
	IssueCategories map[string]int `json:"issue_categories"`
	Workers         int            `json:"workers"`
	CacheHits       int            `json:"cache_hits"`
	CacheMisses     int            `json:"cache_misses"`
	Results         []CheckResult  `json:"results,omitempty"`
}

// ComparisonResult compares async vs sync baseline metrics
type ComparisonResult struct {
	Baseline    BaselineMetrics       `json:"baseline"`
	Async       AsyncBaselineMetrics  `json:"async"`
	Improvement ComparisonImprovement `json:"improvement"`
	Correctness CorrectnessCheck      `json:"correctness"`
}

// ComparisonImprovement shows performance improvements
type ComparisonImprovement struct {
	DurationImprovement   float64 `json:"duration_improvement_percent"`   // Negative = faster
	ThroughputImprovement float64 `json:"throughput_improvement_percent"` // Positive = better
	Speedup               float64 `json:"speedup"`                        // Baseline duration / Async duration
}

// CorrectnessCheck verifies async validator correctness
type CorrectnessCheck struct {
	ObjectsMatch       bool    `json:"objects_match"`
	IssuesMatch        bool    `json:"issues_match"`
	ObjectCountDiff    int     `json:"object_count_diff"`
	IssueCountDiff     int     `json:"issue_count_diff"`
	IssueTierMatch     bool    `json:"issue_tier_match"`
	IssueCategoryMatch bool    `json:"issue_category_match"`
	CorrectnessScore   float64 `json:"correctness_score"` // 0.0 to 1.0
}

// runCheckAsyncBaseline runs the async validator and compares with baseline
func runCheckAsyncBaseline(cmd *cobra.Command, ctx *cli.Context, _ []string) error {
	projectRoot := ctx.ProjectRoot
	projectRoot = ProjectRootOrResolve(projectRoot)

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))

	// Load baseline metrics
	baselineFile, _ := cmd.Flags().GetString("baseline-file") //nolint:errcheck // Flag parsing errors are non-critical
	if baselineFile == emptyValue {
		baselineFile = filepath.Join(paths.ProjectDataDir, "baseline.json")
	}

	baseline, err := loadBaselineMetrics(baselineFile)
	if err != nil {
		return errfmt.Newf("failed to load baseline metrics").Wrap(err)
	}

	logging.Fluent(logger).Info("Starting async validator baseline comparison").
		ProjectRoot(projectRoot).
		String("baseline_file", baselineFile).
		Log()

	// Get workers count
	workers, _ := cmd.Flags().GetInt("workers") //nolint:errcheck // Flag parsing errors are non-critical
	if workers <= 0 {
		workers = 4 // Default
	}

	// Check if we should include full results
	includeResults, _ := cmd.Flags().GetBool("include-results") //nolint:errcheck // Flag parsing errors are non-critical

	// Check output file
	outputFile, _ := cmd.Flags().GetString("comparison-output") //nolint:errcheck // Flag parsing errors are non-critical

	// Run async validator
	startTime := time.Now()
	asyncResults, err := runAsyncValidator(projectRoot, workers, cmd, ctx)
	if err != nil {
		return errfmt.Newf("failed to run async validator").Wrap(err)
	}
	duration := time.Since(startTime)

	// Collect async metrics
	asyncMetrics := collectAsyncBaselineMetrics(asyncResults, duration, workers, includeResults)

	// Compare with baseline
	comparison := compareBaselines(&baseline, &asyncMetrics)

	// Output comparison
	if err := outputComparison(cmd, &baseline, &asyncMetrics, &comparison); err != nil {
		return errfmt.Newf("failed to output comparison").Wrap(err)
	}

	// Save comparison if requested
	if outputFile != emptyValue {
		if err := saveComparisonResult(outputFile, &comparison); err != nil {
			return errfmt.Newf("failed to save comparison").Wrap(err)
		}
		msg := fmt.Sprintf("\nComparison results saved to: %s\n", outputFile)
		if err := cli.WriteOutput(cmd, []byte(msg)); err != nil {
			return errfmt.Newf("failed to write output").Wrap(err)
		}
	}

	return nil
}

// runAsyncValidator runs the async validator on all objects
func runAsyncValidator(projectRoot string, workers int, cmd *cobra.Command, ctx *cli.Context) ([]CheckResult, error) {
	startTime := time.Now()
	// Get async validator
	validator := GetAsyncValidator(cmd.Context(), projectRoot, 0) // Use default worker count

	// Configure workers
	if workers > 0 {
		// Recreate validator with specified workers
		validator = validation.NewAsyncValidator(cmd.Context(), projectRoot, workers, validation.DefaultValidationStateCacheMaxAge)
	}

	// Set up validation function (same as regular async check)
	// Create a dummy command context for setupAsyncValidationFunction
	dummyCmd := &cobra.Command{}
	setupAsyncValidationFunction(validator, dummyCmd, ctx, projectRoot, nil, "", nil, nil)

	// Start validator
	if err := validator.Start(); err != nil {
		return nil, errfmt.Newf("failed to start async validator").Wrap(err)
	}
	defer func() { _ = validator.Stop() }() //nolint:errcheck // Cleanup errors are non-critical

	// Discover all objects using scanner (handles CAS hash-based files correctly)
	processDir, err := resolveProcessDirForProject(projectRoot)
	if err != nil {
		return nil, errfmt.Newf("resolve process directory").Wrap(err)
	}

	// Use YAMLScanner to discover objects (handles both ID-based and hash-based files)
	scnr := scanner.NewYAMLScanner(processDir)
	scannedFiles, err := scnr.Scan()
	if err != nil {
		return nil, errfmt.Newf("failed to scan for objects").Wrap(err)
	}

	// Enqueue all objects
	totalEnqueued := 0
	for _, file := range scannedFiles {
		// Skip files without object IDs (shouldn't happen, but be safe)
		if file.ObjectID == emptyValue {
			continue
		}

		// Determine priority (Tier 1 = highest priority)
		// Use default priority based on kind (simplified)
		priority := 2 // Default priority (Tier 2)
		if file.ObjectType == objects.KindBacklogItem || file.ObjectType == objects.KindRequirement || file.ObjectType == objects.KindTestCase {
			priority = 1 // Higher priority for core objects
		}

		// Enqueue for validation
		validator.Enqueue(file.ObjectID, file.ObjectType, file.Path, priority)
		totalEnqueued++
	}

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	logging.Fluent(logger).Info("Enqueued objects for async validation").Total(totalEnqueued).Log()

	// Wait for all validations to complete
	timeout := time.After(10 * time.Minute) // Increased timeout for large datasets
	tick := time.Tick(1 * time.Second)      // Check every second

	lastQueueSize := -1
	stableCount := 0

	for {
		select {
		case <-timeout:
			// Get final stats before timeout
			total, stale, _, queueSize := validator.GetValidationStats()
			logging.Fluent(logger).Warn("Timeout waiting for async validation").
				Total(total).
				Enqueued(totalEnqueued).
				QueueSize(queueSize).
				Log()

			// Emit timeout event via coordinator if project root available
			if projectRoot != emptyValue {
				factory, storageErr := storage.NewStorageFactory(pkgctx.NewSystemContext(), projectRoot)
				if storageErr == nil {
					fileStorage := factory.GetStorage()
					profile := systemProfileSystem // Default for baseline runs
					if ctx != nil && ctx.Profile != emptyValue {
						profile = ctx.Profile
					}
					operationID := fmt.Sprintf("baseline_check_%d", time.Now().Unix())
					timeoutDuration := 30 * time.Minute // Default timeout
					completedCount := total - stale
					emitCheckTimeoutEvent(
						pkgctx.NewSystemContext(),
						projectRoot,
						fileStorage,
						operationID,
						completedCount,
						totalEnqueued,
						completedCount,
						0, // Failed count
						queueSize,
						timeoutDuration,
						profile,
					)
					_ = fileStorage.Shutdown(context.Background()) // Background: request-or-shutdown derived
				}
			}

			// Continue to collect results anyway
			results := collectAsyncResults(validator, processDir)
			return results, nil
		case <-tick:
			total, stale, withIssues, queueSize := validator.GetValidationStats()

			// Log progress periodically (also emit via coordinator if available)
			if queueSize != lastQueueSize {
				logging.Fluent(logger).Info("Async validation progress").
					Total(total).
					Stale(stale).
					WithIssues(withIssues).
					QueueSize(queueSize).
					Enqueued(totalEnqueued).
					Log()
				lastQueueSize = queueSize

				// Emit progress event via coordinator if project root available
				// This provides unified observability for baseline comparison runs
				if projectRoot != emptyValue {
					factory, storageErr := storage.NewStorageFactory(pkgctx.NewSystemContext(), projectRoot)
					if storageErr == nil {
						fileStorage := factory.GetStorage()
						profile := systemProfileSystem // Default for baseline runs
						if ctx != nil && ctx.Profile != emptyValue {
							profile = ctx.Profile
						}
						operationID := fmt.Sprintf("baseline_check_%d", time.Now().Unix())
						completedCount := total - stale
						emitCheckProgressEventViaCoordinator(
							pkgctx.NewSystemContext(),
							projectRoot,
							fileStorage,
							operationID,
							"progress",
							completedCount,
							totalEnqueued,
							completedCount,
							0, // Failed count (not tracked in baseline)
							queueSize,
							fmt.Sprintf("Baseline validation: %d/%d processed (queue: %d)", completedCount, totalEnqueued, queueSize),
							profile,
						)
						_ = fileStorage.Shutdown(context.Background()) // Background: request-or-shutdown derived
					}
				}
			}

			// Check if queue is empty and we've processed enough
			if queueSize == 0 {
				stableCount++
				// Wait for 3 consecutive checks with empty queue to ensure completion
				if stableCount >= 3 {
					logging.Fluent(logger).Info("Async validation completed").
						Total(total).
						Stale(stale).
						WithIssues(withIssues).
						Log()

					// Emit completion event via coordinator if project root available
					if projectRoot != emptyValue {
						factory, storageErr := storage.NewStorageFactory(pkgctx.NewSystemContext(), projectRoot)
						if storageErr == nil {
							fileStorage := factory.GetStorage()
							profile := systemProfileSystem // Default for baseline runs
							if ctx != nil && ctx.Profile != emptyValue {
								profile = ctx.Profile
							}
							operationID := fmt.Sprintf("baseline_check_%d", time.Now().Unix())
							duration := time.Since(startTime)
							completedCount := total - stale
							emitCheckCompletionEvent(
								pkgctx.NewSystemContext(),
								projectRoot,
								fileStorage,
								operationID,
								totalEnqueued,
								completedCount,
								0, // Failed count (not tracked in baseline)
								duration,
								profile,
							)
							_ = fileStorage.Shutdown(context.Background()) // Background: request-or-shutdown derived
						}
					}

					// Collect results from cache
					results := collectAsyncResults(validator, processDir)
					return results, nil
				}
			} else {
				stableCount = 0 // Reset stable count if queue has items
			}
		}
	}
}

// collectAsyncResults collects validation results from async validator cache
func collectAsyncResults(validator *validation.AsyncValidator, processDir string) []CheckResult {
	results := make([]CheckResult, 0)

	// Use scanner to discover all objects (handles CAS hash-based files correctly)
	scnr := scanner.NewYAMLScanner(processDir)
	scannedFiles, err := scnr.Scan()
	if err != nil {
		// Log error but continue with empty results
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		logging.Fluent(logger).Warn("Failed to scan for objects when collecting async results").WithError(err).Log()
		return results
	}

	for _, file := range scannedFiles {
		// Skip files without object IDs
		if file.ObjectID == emptyValue {
			continue
		}

		// Get cached validation state
		state, ok := validator.GetCachedState(file.ObjectID)
		if !ok {
			continue
		}
		state, ok = nildecode.DecodeNonNilPayload[*validation.ValidationState](state)
		if !ok {
			continue
		}

		// Convert ValidationState to CheckResult
		result := CheckResult{
			ObjectID:   state.ObjectID,
			ObjectKind: state.ObjectKind,
			FilePath:   state.FilePath,
			Issues:     convertValidationIssues(state.Issues),
		}

		results = append(results, result)
	}

	return results
}

// convertValidationIssues converts validation.ValidationIssue to Issue
func convertValidationIssues(validationIssues []validation.ValidationIssue) []Issue {
	issues := make([]Issue, 0, len(validationIssues))
	for _, vi := range validationIssues {
		// ValidationIssue already has Tier, Category, Message, AutoFixable
		issues = append(issues, Issue{
			Tier:        vi.Tier,
			Category:    vi.Category,
			Message:     vi.Message,
			AutoFixable: vi.AutoFixable,
		})
	}
	return issues
}

// collectAsyncBaselineMetrics collects metrics from async validation results
func collectAsyncBaselineMetrics(results []CheckResult, duration time.Duration, workers int, includeResults bool) AsyncBaselineMetrics {
	metrics := AsyncBaselineMetrics{
		Timestamp:       time.Now(),
		TotalResults:    len(results),
		Duration:        duration,
		Workers:         workers,
		IssuesByTier:    make(map[int]int),
		ObjectKinds:     make(map[string]int),
		IssueCategories: make(map[string]int),
	}

	// Count unique objects
	objectSet := make(map[string]bool)
	for _, result := range results {
		if result.ObjectID != emptyValue {
			objectSet[result.ObjectID] = true
		}
		metrics.ObjectKinds[result.ObjectKind]++

		// Count issues by tier and category
		for _, issue := range result.Issues {
			metrics.IssuesByTier[issue.Tier]++
			metrics.TotalIssues++
			if issue.Category != emptyValue {
				metrics.IssueCategories[issue.Category]++
			}
		}
	}

	metrics.TotalObjects = len(objectSet)
	if duration > 0 {
		metrics.ObjectsPerSec = float64(metrics.TotalObjects) / duration.Seconds()
	}

	// Include full results if requested
	if includeResults {
		metrics.Results = results
	}

	return metrics
}

// compareBaselines compares async metrics with baseline
func compareBaselines(baseline *BaselineMetrics, async *AsyncBaselineMetrics) ComparisonResult {
	improvement := ComparisonImprovement{}
	correctness := CorrectnessCheck{}

	// Calculate performance improvements
	if baseline.Duration > 0 {
		improvement.DurationImprovement = ((float64(async.Duration) - float64(baseline.Duration)) / float64(baseline.Duration)) * 100
		improvement.Speedup = float64(baseline.Duration) / float64(async.Duration)
	}
	if baseline.ObjectsPerSec > 0 {
		improvement.ThroughputImprovement = ((async.ObjectsPerSec - baseline.ObjectsPerSec) / baseline.ObjectsPerSec) * 100
	}

	// Check correctness
	correctness.ObjectsMatch = baseline.TotalObjects == async.TotalObjects
	correctness.ObjectCountDiff = async.TotalObjects - baseline.TotalObjects
	correctness.IssuesMatch = baseline.TotalIssues == async.TotalIssues
	correctness.IssueCountDiff = async.TotalIssues - baseline.TotalIssues

	// Check tier and category match
	correctness.IssueTierMatch = maps.Equal(baseline.IssuesByTier, async.IssuesByTier)
	correctness.IssueCategoryMatch = maps.Equal(baseline.IssueCategories, async.IssueCategories)

	// Calculate correctness score (0.0 to 1.0)
	score := 0.0
	if correctness.ObjectsMatch {
		score += 0.4
	}
	if correctness.IssuesMatch {
		score += 0.3
	}
	if correctness.IssueTierMatch {
		score += 0.2
	}
	if correctness.IssueCategoryMatch {
		score += 0.1
	}
	correctness.CorrectnessScore = score

	return ComparisonResult{
		Baseline:    *baseline,
		Async:       *async,
		Improvement: improvement,
		Correctness: correctness,
	}
}

// outputComparison outputs the comparison results
func outputComparison(cmd *cobra.Command, baseline *BaselineMetrics, async *AsyncBaselineMetrics, comparison *ComparisonResult) error {
	var buf strings.Builder

	buf.WriteString("\n=== Async Validator Baseline Comparison ===\n\n")

	// Performance comparison
	buf.WriteString("Performance:\n")
	fmt.Fprintf(&buf, "  Baseline Duration: %v (%.2f obj/s)\n", baseline.Duration, baseline.ObjectsPerSec)
	fmt.Fprintf(&buf, "  Async Duration:    %v (%.2f obj/s)\n", async.Duration, async.ObjectsPerSec)
	fmt.Fprintf(&buf, "  Speedup:           %.2fx\n", comparison.Improvement.Speedup)
	fmt.Fprintf(&buf, "  Duration Change:   %.2f%%\n", comparison.Improvement.DurationImprovement)
	fmt.Fprintf(&buf, "  Throughput Change: %.2f%%\n", comparison.Improvement.ThroughputImprovement)

	// Correctness comparison
	buf.WriteString("\nCorrectness:\n")
	fmt.Fprintf(&buf, "  Objects Match:     %v (diff: %d)\n", comparison.Correctness.ObjectsMatch, comparison.Correctness.ObjectCountDiff)
	fmt.Fprintf(&buf, "  Issues Match:       %v (diff: %d)\n", comparison.Correctness.IssuesMatch, comparison.Correctness.IssueCountDiff)
	fmt.Fprintf(&buf, "  Tier Match:         %v\n", comparison.Correctness.IssueTierMatch)
	fmt.Fprintf(&buf, "  Category Match:     %v\n", comparison.Correctness.IssueCategoryMatch)
	fmt.Fprintf(&buf, "  Correctness Score:  %.1f%%\n", comparison.Correctness.CorrectnessScore*100)

	// Issue breakdown comparison
	buf.WriteString("\nIssues by Tier:\n")
	fmt.Fprintf(&buf, "  Baseline: %v\n", baseline.IssuesByTier)
	fmt.Fprintf(&buf, "  Async:    %v\n", async.IssuesByTier)

	return cli.WriteOutput(cmd, []byte(buf.String()))
}

// loadBaselineMetrics loads baseline metrics from JSON file
func loadBaselineMetrics(filePath string) (BaselineMetrics, error) {
	data, err := fileutil.ReadFile(filePath)
	if err != nil {
		return BaselineMetrics{}, errfmt.Newf("failed to read baseline file").Wrap(err)
	}

	var baselineJSON struct {
		Timestamp       string         `json:"timestamp"`
		TotalObjects    int            `json:"total_objects"`
		TotalResults    int            `json:"total_results"`
		DurationMS      float64        `json:"duration_ms"`
		ObjectsPerSec   float64        `json:"objects_per_second"`
		IssuesByTier    map[string]int `json:"issues_by_tier"`
		TotalIssues     int            `json:"total_issues"`
		ObjectKinds     map[string]int `json:"object_kinds"`
		IssueCategories map[string]int `json:"issue_categories"`
	}

	if err := json.Unmarshal(data, &baselineJSON); err != nil {
		return BaselineMetrics{}, errfmt.Newf("failed to parse baseline JSON").Wrap(err)
	}

	// Convert string keys to int keys for IssuesByTier
	issuesByTier := make(map[int]int)
	for k, v := range baselineJSON.IssuesByTier {
		var tier int
		_, _ = fmt.Sscanf(k, "%d", &tier)
		issuesByTier[tier] = v
	}

	// Parse timestamp
	timestamp, _ := time.Parse(time.RFC3339, baselineJSON.Timestamp) //nolint:errcheck // Timestamp parsing errors use zero value

	return BaselineMetrics{
		Timestamp:       timestamp,
		TotalObjects:    baselineJSON.TotalObjects,
		TotalResults:    baselineJSON.TotalResults,
		Duration:        time.Duration(baselineJSON.DurationMS * 1e6), // Convert ms to nanoseconds
		ObjectsPerSec:   baselineJSON.ObjectsPerSec,
		IssuesByTier:    issuesByTier,
		TotalIssues:     baselineJSON.TotalIssues,
		ObjectKinds:     baselineJSON.ObjectKinds,
		IssueCategories: baselineJSON.IssueCategories,
	}, nil
}

// saveComparisonResult saves comparison results to JSON file
func saveComparisonResult(outputFile string, comparison *ComparisonResult) error {
	comparisonJSON := struct {
		Baseline struct {
			Timestamp     string      `json:"timestamp"`
			TotalObjects  int         `json:"total_objects"`
			DurationMS    float64     `json:"duration_ms"`
			ObjectsPerSec float64     `json:"objects_per_second"`
			TotalIssues   int         `json:"total_issues"`
			IssuesByTier  map[int]int `json:"issues_by_tier"`
		} `json:"baseline"`
		Async struct {
			Timestamp     string      `json:"timestamp"`
			TotalObjects  int         `json:"total_objects"`
			DurationMS    float64     `json:"duration_ms"`
			ObjectsPerSec float64     `json:"objects_per_second"`
			TotalIssues   int         `json:"total_issues"`
			IssuesByTier  map[int]int `json:"issues_by_tier"`
			Workers       int         `json:"workers"`
		} `json:"async"`
		Improvement ComparisonImprovement `json:"improvement"`
		Correctness CorrectnessCheck      `json:"correctness"`
	}{
		Improvement: comparison.Improvement,
		Correctness: comparison.Correctness,
	}

	comparisonJSON.Baseline.Timestamp = comparison.Baseline.Timestamp.Format(time.RFC3339)
	comparisonJSON.Baseline.TotalObjects = comparison.Baseline.TotalObjects
	comparisonJSON.Baseline.DurationMS = float64(comparison.Baseline.Duration.Nanoseconds()) / 1e6
	comparisonJSON.Baseline.ObjectsPerSec = comparison.Baseline.ObjectsPerSec
	comparisonJSON.Baseline.TotalIssues = comparison.Baseline.TotalIssues
	comparisonJSON.Baseline.IssuesByTier = comparison.Baseline.IssuesByTier

	comparisonJSON.Async.Timestamp = comparison.Async.Timestamp.Format(time.RFC3339)
	comparisonJSON.Async.TotalObjects = comparison.Async.TotalObjects
	comparisonJSON.Async.DurationMS = float64(comparison.Async.Duration.Nanoseconds()) / 1e6
	comparisonJSON.Async.ObjectsPerSec = comparison.Async.ObjectsPerSec
	comparisonJSON.Async.TotalIssues = comparison.Async.TotalIssues
	comparisonJSON.Async.IssuesByTier = comparison.Async.IssuesByTier
	comparisonJSON.Async.Workers = comparison.Async.Workers

	data, err := json.MarshalIndent(comparisonJSON, "", "  ")
	if err != nil {
		return errfmt.Newf("failed to marshal comparison").Wrap(err)
	}

	if err := fileutil.MkdirAll(filepath.Dir(outputFile), paths.DirPerm755); err != nil {
		return errfmt.Newf("failed to create output directory").Wrap(err)
	}

	if err := fileutil.WriteFile(outputFile, data, paths.FilePerm644); err != nil { //nolint:gosec // Baseline files - 0600 is acceptable
		return errfmt.Newf("failed to write output file").Wrap(err)
	}

	return nil
}

// NewCheckAsyncBaselineCmd creates a command to compare async validator with baseline
func NewCheckAsyncBaselineCmd() *cobra.Command {
	defaultBaselineFile := filepath.Join(paths.ProjectDataDir, "baseline.json")
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Run async validator and compare with baseline metrics",
		"Runs the async validator on all objects and compares results with baseline metrics.",
		"",
		"This command:",
		"  - Runs async validator on all objects",
		"  - Compares performance with baseline",
		"  - Verifies correctness (same objects, same issues)",
		"  - Reports improvements and differences",
	).
		AddExample("Compare with default baseline file", "%s system check-async-baseline").
		AddExample("Compare with custom baseline file", "%s system check-async-baseline --baseline-file custom-baseline.json").
		AddExample("Use more workers for faster execution", "%s system check-async-baseline --workers 8").
		AddExample("Save comparison results", "%s system check-async-baseline --comparison-output comparison.json").
		ExcludeCommonFlags()

	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemCheckAsyncBaselineCommandBuilder(), &cobra.Command{
		Use: "check-async-baseline",
	})
	cli.BindAsyncProgress(cmd, func(cmd *cobra.Command, args []string) error {
		initCtx := &pkgctx.CliInitializationContext{
			ProjectRoot: ProjectRootOrResolve(""),
		}
		ctx, err := cli.GetContextFromCommand(cmd, initCtx)
		if err != nil {
			return err
		}
		return runCheckAsyncBaseline(cmd, ctx, args)
	})

	// Apply help builder to command
	helpBuilder.ApplyToCommand(cmd)

	cmd.Flags().String("baseline-file", defaultBaselineFile, "Path to baseline metrics file")
	cmd.Flags().String("comparison-output", "", "Output file for comparison results (JSON format)")
	cmd.Flags().Int("workers", 4, "Number of validation workers")
	cmd.Flags().Bool("include-results", false, "Include full check results in output (increases file size)")

	cli.AddCommonFlags(cmd)
	return cmd
}
