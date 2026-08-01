package system

import (
	"github.com/lanceman/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/lanceman/zqk/pkg/datacell"

	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/lanceman/zqk/internal/cli"
	clipkg "github.com/lanceman/zqk/pkg/cli"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/validation"
	"github.com/spf13/cobra"
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
	projectRoot := ctx.ProjectRoot
	projectRoot = ProjectRootOrResolve(projectRoot)

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

	// Output metrics (POLICY-CODE-007: cli.CommandOutputWriter aligns with WriteOutput / MCP)
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
	if _, err := os.Stat(processDir); os.IsNotExist(err) {
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

// collectBaselineMetrics collects metrics from check results
func collectBaselineMetrics(results []CheckResult, duration time.Duration, includeResults bool) BaselineMetrics {
	metrics := BaselineMetrics{
		Timestamp:       time.Now(),
		TotalResults:    len(results),
		Duration:        duration,
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

	data, err := json.MarshalIndent(metricsJSON, "", "  ")
	if err != nil {
		return errfmt.Newf("failed to marshal metrics").Wrap(err)
	}

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(outputFile), paths.DirPerm755); err != nil {
		return errfmt.Newf("failed to create output directory").Wrap(err)
	}

	if err := os.WriteFile(outputFile, data, paths.FilePerm644); err != nil { //nolint:gosec // Baseline files - 0600 is acceptable
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
	cli.BindAsyncProgress(cmd, func(cmd *cobra.Command, args []string) error {
		initCtx := &pkgctx.CliInitializationContext{
			ProjectRoot: ProjectRootOrResolve(""),
		}
		ctx, err := cli.GetContextFromCommand(cmd, initCtx)
		if err != nil {
			return err
		}
		return runCheckBaseline(cmd, ctx, args)
	})

	// Apply help builder to command
	helpBuilder.ApplyToCommand(cmd)

	cmd.Flags().String("baseline-output", "", "Output file for baseline metrics (JSON format)")
	cmd.Flags().Bool("include-results", false, "Include full check results in output (increases file size)")

	cli.AddCommonFlags(cmd)
	return cmd
}
