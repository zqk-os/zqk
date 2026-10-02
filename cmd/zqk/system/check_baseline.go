package system

import (
	"runtime"

	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/datacell"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"

	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/pkg/cliapp"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/validation"
)

// BaselineMetrics contains baseline performance and result metrics
type BaselineMetrics struct {
	Timestamp       time.Time      `json:"timestamp"`
	TotalObjects    int            `json:"total_objects"`
	TotalResults    int            `json:"total_results"`
	Duration        time.Duration  `json:"duration_ms"`
	ObjectsPerSec   float64        `json:"objects_per_second"`
	IssuesByTier    map[int]int    `json:"issues_by_tier"`
	TotalIssues     int            `json:"total_issues"`
	ObjectKinds     map[string]int `json:"object_kinds"`
	IssueCategories map[string]int `json:"issue_categories"`
	MemoryUsageMB   float64        `json:"memory_usage_mb"`
	Results         []CheckResult  `json:"results,omitempty"` // Optional: include full results
}

// runCheckBaseline runs the synchronous check command and outputs baseline metrics
func runCheckBaseline(cmd *cobra.Command, ctx *cli.Context, _ []string) error {
	projectRoot := ProjectRootOrResolve(ctx.ProjectRoot)
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))

	// Check if we should include full results
	includeResults, _ := cmd.Flags().GetBool("include-results") //nolint:errcheck // Flag parsing errors are non-critical

	// Check output file
	outputFile, _ := cmd.Flags().GetString("baseline-output") //nolint:errcheck // Flag parsing errors are non-critical

	logging.Fluent(logger).Info("Starting synchronous check baseline collection").
		ProjectRoot(projectRoot).
		Log()

	// Run synchronous check
	startTime := time.Now()

	// Use existing checkAll function to get all results
	allResults, err := runCheckAllSynchronous(projectRoot, cmd, ctx)
	if err != nil {
		return errfmt.Newf("failed to run synchronous check").Wrap(err)
	}

	duration := time.Since(startTime)

	// Collect metrics
	metrics := collectBaselineMetrics(allResults, duration, includeResults)

	// Output metrics (POL-CODE-007: cli.CommandOutputWriter aligns with WriteOutput / MCP)
	out := cli.CommandOutputWriter(cmd, nil)
	fmt.Fprintf(out, "=== Synchronous Check Baseline ===\n")
	fmt.Fprintf(out, "Timestamp: %s\n", metrics.Timestamp.Format(time.RFC3339))
	fmt.Fprintf(out, "Total Objects: %d\n", metrics.TotalObjects)
	fmt.Fprintf(out, "Total Results: %d\n", metrics.TotalResults)
	fmt.Fprintf(out, "Duration: %v\n", duration)
	fmt.Fprintf(out, "Objects/Second: %.2f\n", metrics.ObjectsPerSec)
	fmt.Fprintf(out, "Total Issues: %d\n", metrics.TotalIssues)

	fmt.Fprintf(out, "\nIssues by Tier:\n")
	for tier := 1; tier <= 4; tier++ {
		count := metrics.IssuesByTier[tier]
		if count > 0 {
			fmt.Fprintf(out, "  Tier %d: %d\n", tier, count)
		}
	}

	if len(metrics.ObjectKinds) > 0 {
		fmt.Fprintf(out, "\nObjects by Kind:\n")
		for kind, count := range metrics.ObjectKinds {
			fmt.Fprintf(out, "  %s: %d\n", kind, count)
		}
	}

	if len(metrics.IssueCategories) > 0 {
		fmt.Fprintf(out, "\nIssues by Category:\n")
		for category, count := range metrics.IssueCategories {
			fmt.Fprintf(out, "  %s: %d\n", category, count)
		}
	}

	// Save to file if requested
	if outputFile != emptyValue {
		if err := saveBaselineMetrics(outputFile, &metrics); err != nil {
			return errfmt.Newf("failed to save baseline metrics").Wrap(err)
		}
		fmt.Fprintf(out, "\nBaseline metrics saved to: %s\n", outputFile)
	}

	return nil
}

// runCheckAllSynchronous runs the synchronous check on all objects
// It uses CheckKindObjectsWithCache for each kind and collects all results
func runCheckAllSynchronous(projectRoot string, cmd *cobra.Command, ctx *cli.Context) ([]CheckResult, error) {
	// Create internal context for checkAll
	internalCtx := cli.ContextForProjectAndProfile(projectRoot, ctx.Profile)

	processDir := datacell.ProcessPrimaryDir(projectRoot)
	if _, err := fileutil.Stat(processDir); fileutil.IsNotExist(err) {
		return nil, errfmt.Errorf("process directory not found: %s", processDir)
	}

	// Discover all object kinds
	kinds := discoverObjectKinds(processDir)

	// Create shared loaders
	// Use global loaders to share caches across sync and async validation
	specLoader := objects.GetGlobalSpecLoader()
	lifecycleLoader := objects.GetGlobalLifecycleLoader()
	validatorRegistry := validation.GetGlobalRegistry()
	validator := validatorRegistry.Get("")

	// Cache hash registries per kind
	hashRegistryCache := &HashRegistryCacheType{
		cache: make(map[string]storage.HashRegistryProvider),
	}

	// Build object ID cache
	objectIDCache := GetGlobalObjectIDCache()
	checkRefs, _ := cmd.Flags().GetBool("check-refs") //nolint:errcheck // Flag parsing errors are non-critical
	fastMode, _ := cmd.Flags().GetBool("fast")        //nolint:errcheck // Flag parsing errors are non-critical
	if checkRefs && !fastMode {
		loadCtx := pkgctx.NewSystemContext()
		if cmd != nil && cmd.Context() != nil {
			loadCtx = cmd.Context()
		}
		if err := EnsureObjectIDCacheReady(loadCtx, projectRoot, false, nil, nil); err != nil {
			logger := logging.GetLoggerFromProfile(ctx.Profile)
			logging.Fluent(logger).Warn("Failed to build object ID cache").
				WithError(err).
				Log()
		}
	}

	// Collect all results
	allResults := make([]CheckResult, 0)

	// Check each kind
	for _, kind := range kinds {
		results, _, err := CheckKindObjectsWithCache(internalCtx, pkgctx.NewSystemContext(), cmd, kind, nil, specLoader, lifecycleLoader, validator, hashRegistryCache, objectIDCache)
		if err != nil {
			logger := logging.GetLoggerFromProfile(ctx.Profile)
			logging.Fluent(logger).Warn("Failed to check kind").
				Kind(kind).
				WithError(err).
				Log()
			continue
		}
		allResults = append(allResults, results...)
	}

	return allResults, nil
}

type checkResultsSummary struct {
	TotalObjects    int
	TotalIssues     int
	ObjectsPerSec   float64
	IssuesByTier    map[int]int
	ObjectKinds     map[string]int
	IssueCategories map[string]int
}

func summarizeCheckResults(results []CheckResult, duration time.Duration) checkResultsSummary {
	summary := checkResultsSummary{
		IssuesByTier:    make(map[int]int),
		ObjectKinds:     make(map[string]int),
		IssueCategories: make(map[string]int),
	}

	objectSet := make(map[string]bool)
	for _, result := range results {
		if result.ObjectID != emptyValue {
			objectSet[result.ObjectID] = true
		}
		summary.ObjectKinds[result.ObjectKind]++

		for _, issue := range result.Issues {
			summary.IssuesByTier[issue.Tier]++
			summary.TotalIssues++
			if issue.Category != emptyValue {
				summary.IssueCategories[issue.Category]++
			}
		}
	}

	summary.TotalObjects = len(objectSet)
	if duration > 0 {
		summary.ObjectsPerSec = float64(summary.TotalObjects) / duration.Seconds()
	}
	return summary
}

// collectBaselineMetrics collects metrics from check results
func collectBaselineMetrics(results []CheckResult, duration time.Duration, includeResults bool) BaselineMetrics {
	summary := summarizeCheckResults(results, duration)
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	metrics := BaselineMetrics{
		Timestamp:       time.Now(),
		TotalResults:    len(results),
		Duration:        duration,
		TotalObjects:    summary.TotalObjects,
		TotalIssues:     summary.TotalIssues,
		ObjectsPerSec:   summary.ObjectsPerSec,
		MemoryUsageMB:   float64(m.Alloc) / 1024 / 1024,
		IssuesByTier:    summary.IssuesByTier,
		ObjectKinds:     summary.ObjectKinds,
		IssueCategories: summary.IssueCategories,
	}
	if includeResults {
		metrics.Results = results
	}
	return metrics
}

// saveBaselineMetrics saves baseline metrics to a JSON file
func saveBaselineMetrics(outputFile string, metrics *BaselineMetrics) error {
	// Convert duration to milliseconds for JSON
	metricsJSON := struct {
		Timestamp       string         `json:"timestamp"`
		TotalObjects    int            `json:"total_objects"`
		TotalResults    int            `json:"total_results"`
		DurationMS      float64        `json:"duration_ms"`
		ObjectsPerSec   float64        `json:"objects_per_second"`
		IssuesByTier    map[int]int    `json:"issues_by_tier"`
		TotalIssues     int            `json:"total_issues"`
		ObjectKinds     map[string]int `json:"object_kinds"`
		IssueCategories map[string]int `json:"issue_categories"`
		Results         []CheckResult  `json:"results,omitempty"`
	}{
		Timestamp:       metrics.Timestamp.Format(time.RFC3339),
		TotalObjects:    metrics.TotalObjects,
		TotalResults:    metrics.TotalResults,
		DurationMS:      float64(metrics.Duration.Nanoseconds()) / 1e6,
		ObjectsPerSec:   metrics.ObjectsPerSec,
		IssuesByTier:    metrics.IssuesByTier,
		TotalIssues:     metrics.TotalIssues,
		ObjectKinds:     metrics.ObjectKinds,
		IssueCategories: metrics.IssueCategories,
		Results:         metrics.Results,
	}

	return writeJSONFile(outputFile, metricsJSON)
}

// writeJSONFile writes indented JSON data to targetPath, ensuring parent directory exists.
func writeJSONFile(targetPath string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return errfmt.Newf("failed to marshal JSON").Wrap(err)
	}

	if err := fileutil.MkdirAll(filepath.Dir(targetPath), paths.DirPerm755); err != nil {
		return errfmt.Newf("failed to create output directory").Wrap(err)
	}

	if err := fileutil.WriteFile(targetPath, data, paths.FilePerm644); err != nil { //nolint:gosec // Output files - 0644 is standard
		return errfmt.Newf("failed to write output file").Wrap(err)
	}

	return nil
}

// NewCheckBaselineCmd creates a command to collect baseline metrics
func NewCheckBaselineCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Collect baseline metrics from synchronous check command",
		"Collects baseline performance and result metrics from the synchronous check command.",
		"This baseline is used to compare against async validator performance.",
	).
		AddExample("Collect baseline and save to file", "%s system check-baseline --baseline-output baseline.json").
		AddExample("Include full results in output", "%s system check-baseline --baseline-output baseline.json --include-results").
		ExcludeCommonFlags()

	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemCheckBaselineCommandBuilder(), &cobra.Command{
		Use: "check-baseline",
	})
	bindSystemCliContextRunner(cmd, runCheckBaseline)

	// Apply help builder to command
	helpBuilder.ApplyToCommand(cmd)

	cmd.Flags().String("baseline-output", "", "Output file for baseline metrics (JSON format)")
	cmd.Flags().Bool("include-results", false, "Include full check results in output (increases file size)")

	cli.AddCommonFlags(cmd)
	return cmd
}
