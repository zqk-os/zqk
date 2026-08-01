package system

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/lanceman/zqk/internal/cli"
	"github.com/lanceman/zqk/pkg/agentdelivery"
	pkgcli "github.com/lanceman/zqk/pkg/cli"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/utils/fileutil"
	"github.com/lanceman/zqk/pkg/when"
	"github.com/spf13/cobra"
)

func outputResults(cmd *cobra.Command, ctx *cli.Context, results []CheckResult, buffer *AuditEventBuffer, staleCASResult *StaleCASCleanupResult, runIssues *ValidationRunIssues) error {
	autoFix, _ := cmd.Flags().GetBool("auto-fix")
	if autoFix && cmd != nil {
		// Check if scheduler batching is enabled (default: true for batches >= 10 issues)
		useScheduler, _ := cmd.Flags().GetBool("auto-fix-scheduler")
		if useScheduler {
			autoFixableCount := 0
			for _, r := range results {
				for _, issue := range r.Issues {
					if IsIssueFixableForBatch(issue) {
						autoFixableCount++
					}
				}
			}
			if autoFixableCount >= 10 { // validationAutoFixBatchThreshold is 10
				// Disable file deletion in detectOrphanedFiles because auto-fixes are processed asynchronously in the background.
				// Deleting them now would cause a race condition with background scheduler jobs.
				autoFix = false
			}
		}
	}
	results = detectOrphanedFiles(cmd, ctx.ProjectRoot, results, autoFix)

	if layer, err := cmd.Flags().GetInt("layer"); err == nil && layer >= 0 && layer <= 3 {
		results = filterResultsByLayer(results, layer)
	}

	format := cli.GetFormat(cmd)
	if format == "ids" {
		var ids []string
		for _, r := range results {
			ids = append(ids, r.ObjectID)
		}
		delim, _ := cmd.Flags().GetString("delimiter")
		if delim == "" {
			delim = "\n"
		} else {
			delim = strings.ReplaceAll(delim, "\\n", "\n")
			delim = strings.ReplaceAll(delim, "\\t", "\t")
			delim = strings.ReplaceAll(delim, "\\r", "\r")
		}
		output := strings.Join(ids, delim)
		if len(ids) > 0 {
			if strings.HasSuffix(delim, "\n") {
				output += delim
			} else {
				output += "\n"
			}
		}
		return cli.WriteOutput(cmd, []byte(output))
	}

	unresolvableOnly, _ := cmd.Flags().GetBool("unresolvable-only")
	if unresolvableOnly {
		specLoader := objects.NewSpecLoader(ctx.ProjectRoot)
		logger := logging.GetLoggerFromProfile(ctx.Profile)
		resolver := NewViolationResolver(ctx.ProjectRoot, specLoader, logger)

		snapshot := &CheckSnapshot{
			Results: results,
		}
		filteredSnapshot, err := resolver.FilterResolvableViolations(snapshot)
		if err != nil {
			return errfmt.Newf("failed to filter resolvable violations").Wrap(err)
		}
		results = filteredSnapshot.Results
	}
	dispatchTo, _ := cmd.Flags().GetString("dispatch-to")
	if dispatchTo != "" {
		if err := dispatchViolationsToInbox(ctx.ProjectRoot, results, dispatchTo); err != nil {
			logger := logging.GetLoggerFromProfile(ctx.Profile)
			logging.Fluent(logger).Warn("Failed to dispatch violations to inbox").
				WithError(err).
				Log()
		}
	}

	outputCtx, err := initializeOutputResultsContext(cmd, ctx, buffer)
	if err != nil {
		return err
	}
	if staleCASResult != nil {
		outputCtx.StaleCASCleanupResult = staleCASResult
	}
	if runIssues != nil {
		outputCtx.RunIssues = runIssues
	}

	// Write Tier 1 failing IDs for fast re-check (--write-failing-ids)
	if err := writeFailingIDsToFile(cmd, results); err != nil {
		logger := logging.GetLoggerFromProfile(ctx.Profile)
		logging.Fluent(logger).Warn("Failed to write failing IDs file").
			WithError(err).
			Log()
	}

	if err := handleSnapshot(cmd, ctx, results, outputCtx.SnapshotPath); err != nil {
		return err
	}

	// Handle table format (returns early if handled; includes "Validation run issues" when runIssues set)
	if err := handleTableFormat(cmd, ctx, results, outputCtx); err != nil {
		return err
	}
	// If table format was handled, return early to avoid calling handleLegacyFormat
	// which would also output table format
	if outputCtx.Format == cli.FormatTable || outputCtx.FormatStr == "table" {
		return nil
	}

	// Handle streaming formats
	if err := handleStreamingFormat(cmd, ctx, results, outputCtx); err != nil {
		return err
	}
	// If streaming format was handled, return early
	if outputCtx.Handler != nil && outputCtx.Handler.IsStreaming() {
		return nil
	}

	// Handle non-streaming formats with handlers
	if err := handleNonStreamingFormat(cmd, results, outputCtx); err != nil {
		return err
	}
	// If non-streaming format handler was used, return early
	if outputCtx.Handler != nil && !outputCtx.Handler.IsStreaming() {
		return nil
	}

	// Fallback to legacy format handlers (only if no handler was used)
	return handleLegacyFormat(cmd, ctx, results, outputCtx)
}

// countUnprocessedAutofixBatches returns the number of AUTOFIX-*.json files under .zqk/autofix/
// that have not yet been processed (i.e. not renamed to FIXED_/PROCESSED_). Used so system check
// does not report "healthy" when fixes were batched but never applied.
func countUnprocessedAutofixBatches(projectRoot string) int {
	if projectRoot == emptyValue {
		return 0
	}
	autofixDir := filepath.Join(projectRoot, paths.ProjectDataDir, paths.AutofixDir)
	entries, err := os.ReadDir(autofixDir)
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(name, "AUTOFIX-") && strings.HasSuffix(name, ".json") {
			n++
		}
	}
	return n
}

// clearUnprocessedAutofixBatchesWhenGreen removes any AUTOFIX-*.json under .zqk/autofix/ so that
// "system check green" implies "0 unprocessed autofix files". Call only when the check has
// reported all violations resolved (no tier-1/2/3 issues and no pending batches at report time).
// Keeps FIXED_/PROCESSED_ files untouched.
func clearUnprocessedAutofixBatchesWhenGreen(projectRoot string) {
	if projectRoot == emptyValue {
		return
	}
	autofixDir := filepath.Join(projectRoot, paths.ProjectDataDir, paths.AutofixDir)
	entries, err := os.ReadDir(autofixDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(name, "AUTOFIX-") && strings.HasSuffix(name, ".json") {
			_ = os.Remove(filepath.Join(autofixDir, name))
		}
	}
}

// compactCheckEntry is one object in the compact JSON cache: id, path (normalized), issues. Kind is the bucket key.
type compactCheckEntry struct {
	ID        string   `json:"id"`
	Path      string   `json:"path"` // relative to project root to minimize size
	Issues    []Issue  `json:"issues,omitempty"`
	AutoFixed []string `json:"auto_fixed,omitempty"`
}

// PrepareCompactCheckOutputData returns output data for JSON format: summary + results_by_kind (bucketed, paths normalized).
// Used by the format handler so the written cache (e.g. .zqk/pre-commit/system-check.json) is compact.
func PrepareCompactCheckOutputData(results []CheckResult, bufferCount int, bufferSummary map[string]int, projectRoot string) any {
	summary := struct {
		TotalObjects          int            `json:"total_objects"`
		PublicObjects         int            `json:"public_objects"`
		InternalObjects       int            `json:"internal_objects"`
		TotalIssues           int            `json:"total_issues"`
		BlockingIssues        int            `json:"blocking_issues"`
		PublicBlocking        int            `json:"public_blocking"`
		InternalBlocking      int            `json:"internal_blocking"`
		Warnings              int            `json:"warnings"`
		PublicWarnings        int            `json:"public_warnings"`
		InternalWarnings      int            `json:"internal_warnings"`
		Informational         int            `json:"informational"`
		PublicInformational   int            `json:"public_informational"`
		InternalInformational int            `json:"internal_informational"`
		Recommendations       int            `json:"recommendations"`
		AutoFixed             int            `json:"auto_fixed"`
		PendingAutofixBatches int            `json:"pending_autofix_batches,omitempty"`
		AuditEventsBuffered   int            `json:"audit_events_buffered,omitempty"`
		AuditEventsSummary    map[string]int `json:"audit_events_summary,omitempty"`
	}{}
	summary.TotalObjects = len(results)
	for _, result := range results {
		isInternal := isInternalKind(result.ObjectKind)
		when.When(func() bool { return isInternal }).Then(func() { summary.InternalObjects++ }).OrElse(func() { summary.PublicObjects++ }).Run()
		summary.AutoFixed += len(result.AutoFixed)
		for _, issue := range result.Issues {
			summary.TotalIssues++
			switch issue.Tier {
			case 1:
				summary.BlockingIssues++
				when.When(func() bool { return isInternal }).Then(func() { summary.InternalBlocking++ }).OrElse(func() { summary.PublicBlocking++ }).Run()
			case 2:
				summary.Warnings++
				when.When(func() bool { return isInternal }).Then(func() { summary.InternalWarnings++ }).OrElse(func() { summary.PublicWarnings++ }).Run()
			case 3:
				summary.Informational++
				when.When(func() bool { return isInternal }).Then(func() { summary.InternalInformational++ }).OrElse(func() { summary.PublicInformational++ }).Run()
			case 4:
				summary.Recommendations++
			}
		}
	}
	if bufferCount > 0 {
		summary.AuditEventsBuffered = bufferCount
		summary.AuditEventsSummary = bufferSummary
	}
	// Unprocessed autofix batches mean fixes were never applied; do not report healthy (POLICY-CODE-005)
	if pending := countUnprocessedAutofixBatches(projectRoot); pending > 0 {
		summary.BlockingIssues++
		summary.InternalBlocking++
		summary.PendingAutofixBatches = pending
	}
	msg := ""
	if bufferCount > 0 {
		msg = fmt.Sprintf("Note: %d audit event(s) are being buffered and will be written/flushed after this operation completes. Run the command again to see final audit event counts.", bufferCount)
	}
	return struct {
		Summary       interface{}                    `json:"summary"`
		ResultsByKind map[string][]compactCheckEntry `json:"results_by_kind"`
		Message       string                         `json:"message,omitempty"`
	}{
		Summary:       summary,
		ResultsByKind: buildResultsByKind(results, projectRoot),
		Message:       msg,
	}
}

// buildResultsByKind buckets results by kind and normalizes file paths to reduce character count.
// Paths are stored relative to projectRoot (e.g. "docs/process/audit/..." instead of full absolute path).
func buildResultsByKind(results []CheckResult, projectRoot string) map[string][]compactCheckEntry {
	byKind := make(map[string][]compactCheckEntry)
	prefix := filepath.Clean(projectRoot)
	if prefix != emptyValue && !strings.HasSuffix(prefix, string(os.PathSeparator)) {
		prefix += string(os.PathSeparator)
	}
	for _, r := range results {
		path := r.FilePath
		if prefix != emptyValue && (path == prefix || strings.HasPrefix(path, prefix)) {
			path = strings.TrimPrefix(path, prefix)
			path = filepath.ToSlash(path)
		}
		entry := compactCheckEntry{ID: r.ObjectID, Path: path, Issues: r.Issues}
		if len(r.AutoFixed) > 0 {
			entry.AutoFixed = r.AutoFixed
		}
		byKind[r.ObjectKind] = append(byKind[r.ObjectKind], entry)
	}
	return byKind
}

func outputJSON(cmd *cobra.Command, results []CheckResult, bufferCount int, bufferSummary map[string]int) error {
	projectRoot := ""
	if ctx := cli.GetContext(cmd); ctx != nil && ctx.ProjectRoot != emptyValue {
		projectRoot = ctx.ProjectRoot
	}
	projectRoot = ProjectRootOrResolve(projectRoot)

	// Summary (unchanged shape)
	summary := struct {
		TotalObjects          int            `json:"total_objects"`
		PublicObjects         int            `json:"public_objects"`
		InternalObjects       int            `json:"internal_objects"`
		TotalIssues           int            `json:"total_issues"`
		BlockingIssues        int            `json:"blocking_issues"`
		PublicBlocking        int            `json:"public_blocking"`
		InternalBlocking      int            `json:"internal_blocking"`
		Warnings              int            `json:"warnings"`
		PublicWarnings        int            `json:"public_warnings"`
		InternalWarnings      int            `json:"internal_warnings"`
		Informational         int            `json:"informational"`
		PublicInformational   int            `json:"public_informational"`
		InternalInformational int            `json:"internal_informational"`
		Recommendations       int            `json:"recommendations"`
		AutoFixed             int            `json:"auto_fixed"`
		PendingAutofixBatches int            `json:"pending_autofix_batches,omitempty"`
		AuditEventsBuffered   int            `json:"audit_events_buffered,omitempty"`
		AuditEventsSummary    map[string]int `json:"audit_events_summary,omitempty"`
	}{}

	summary.TotalObjects = len(results)
	for _, result := range results {
		isInternal := isInternalKind(result.ObjectKind)
		when.When(func() bool { return isInternal }).Then(func() { summary.InternalObjects++ }).OrElse(func() { summary.PublicObjects++ }).Run()
		summary.AutoFixed += len(result.AutoFixed)
		for _, issue := range result.Issues {
			summary.TotalIssues++
			switch issue.Tier {
			case 1:
				summary.BlockingIssues++
				when.When(func() bool { return isInternal }).Then(func() { summary.InternalBlocking++ }).OrElse(func() { summary.PublicBlocking++ }).Run()
			case 2:
				summary.Warnings++
				when.When(func() bool { return isInternal }).Then(func() { summary.InternalWarnings++ }).OrElse(func() { summary.PublicWarnings++ }).Run()
			case 3:
				summary.Informational++
				when.When(func() bool { return isInternal }).Then(func() { summary.InternalInformational++ }).OrElse(func() { summary.PublicInformational++ }).Run()
			case 4:
				summary.Recommendations++
			}
		}
	}
	if bufferCount > 0 {
		summary.AuditEventsBuffered = bufferCount
		summary.AuditEventsSummary = bufferSummary
	}
	if pending := countUnprocessedAutofixBatches(projectRoot); pending > 0 {
		summary.BlockingIssues++
		summary.InternalBlocking++
		summary.PendingAutofixBatches = pending
	}

	output := struct {
		Summary       interface{}                    `json:"summary"`
		ResultsByKind map[string][]compactCheckEntry `json:"results_by_kind"`
		Message       string                         `json:"message,omitempty"`
	}{
		Summary:       summary,
		ResultsByKind: buildResultsByKind(results, projectRoot),
	}
	if bufferCount > 0 {
		output.Message = fmt.Sprintf("Note: %d audit event(s) are being buffered and will be written/flushed after this operation completes. Run the command again to see final audit event counts.", bufferCount)
	}

	return cli.FormatOutputAs(cmd, cli.FormatJSON, output)
}
func outputJSONL(cmd *cobra.Command, results []CheckResult, bufferCount int, bufferSummary map[string]int) error {
	// Don't create file if there are no results and no buffer info
	// This prevents creating empty files when there's nothing to write
	if len(results) == 0 && bufferCount == 0 {
		outputPath := cli.GetOutputPath(cmd)
		if outputPath != emptyValue {
			// No results and no buffer info - don't create empty file
			return nil
		}
		// If outputting to stdout, still return (nothing to write)
		return nil
	}

	outputPath := cli.GetOutputPath(cmd)
	var writer io.Writer
	if outputPath != emptyValue {
		file, err := os.Create(outputPath)
		if err != nil {
			return errfmt.Newf("failed to create output file").Wrap(err)
		}
		defer file.Close()
		writer = file
	} else {
		ctx := cmd.Context()
		if ctx == nil {
			ctx = pkgctx.NewSystemContext()
		}
		writer = logging.GetCommandOutputWriter(ctx)
	}

	encoder := json.NewEncoder(writer)
	encoder.SetEscapeHTML(false) // Don't escape HTML characters for cleaner output

	// Output each result as a separate JSON object on its own line
	for _, result := range results {
		if err := encoder.Encode(result); err != nil {
			ctx := cli.GetContext(cmd)
			logger := logging.GetLoggerFromProfile(ctx.Profile)
			logging.Fluent(logger).Error("Error encoding JSONL", err).Log()
			return errfmt.Newf("failed to encode JSONL").Wrap(err)
		}
	}

	// If there's buffer information, output it as a final line
	if bufferCount > 0 {
		bufferInfo := map[string]any{
			objects.FieldKeyType:    "buffer_info",
			"audit_events_buffered": bufferCount,
			"audit_events_summary":  bufferSummary,
			"message":               fmt.Sprintf("Note: %d audit event(s) are being buffered and will be written/flushed after this operation completes. Run the command again to see final audit event counts.", bufferCount),
		}
		if err := encoder.Encode(bufferInfo); err != nil {
			ctx := cli.GetContext(cmd)
			logger := logging.GetLoggerFromProfile(ctx.Profile)
			logging.Fluent(logger).Error("Error encoding buffer info", err).Log()
			return errfmt.Newf("failed to encode buffer info").Wrap(err)
		}
	}

	return nil
}
func outputYAML(cmd *cobra.Command, results []CheckResult, bufferCount int, bufferSummary map[string]int) error {
	projectRoot := ""
	if ctx := cli.GetContext(cmd); ctx != nil && ctx.ProjectRoot != emptyValue {
		projectRoot = ctx.ProjectRoot
	}
	projectRoot = ProjectRootOrResolve(projectRoot)

	// Create a structured output object
	output := struct {
		Summary struct {
			TotalObjects          int            `yaml:"total_objects"`
			TotalIssues           int            `yaml:"total_issues"`
			BlockingIssues        int            `yaml:"blocking_issues"`
			Warnings              int            `yaml:"warnings"`
			Informational         int            `yaml:"informational"`
			Recommendations       int            `yaml:"recommendations"`
			AutoFixed             int            `yaml:"auto_fixed"`
			PendingAutofixBatches int            `yaml:"pending_autofix_batches,omitempty"`
			AuditEventsBuffered   int            `yaml:"audit_events_buffered,omitempty"`
			AuditEventsSummary    map[string]int `yaml:"audit_events_summary,omitempty"`
		} `yaml:"summary"`
		Results []CheckResult `yaml:"results"`
		Message string        `yaml:"message,omitempty"`
	}{}

	// Calculate summary statistics
	output.Summary.TotalObjects = len(results)
	for _, result := range results {
		output.Summary.AutoFixed += len(result.AutoFixed)
		for _, issue := range result.Issues {
			output.Summary.TotalIssues++
			switch issue.Tier {
			case 1:
				output.Summary.BlockingIssues++
			case 2:
				output.Summary.Warnings++
			case 3:
				output.Summary.Informational++
			case 4:
				output.Summary.Recommendations++
			}
		}
	}
	if pending := countUnprocessedAutofixBatches(projectRoot); pending > 0 {
		output.Summary.BlockingIssues++
		output.Summary.PendingAutofixBatches = pending
	}

	output.Results = results

	// Add audit event buffer information
	if bufferCount > 0 {
		output.Summary.AuditEventsBuffered = bufferCount
		output.Summary.AuditEventsSummary = bufferSummary
		output.Message = fmt.Sprintf("Note: %d audit event(s) are being buffered and will be written/flushed after this operation completes. Run the command again to see final audit event counts.", bufferCount)
	}

	return cli.FormatOutputAs(cmd, cli.FormatYAML, output)
}
func outputTable(cmd *cobra.Command, ctx *cli.Context, results []CheckResult, bufferCount int, bufferSummary map[string]int, staleCASResult *StaleCASCleanupResult, runIssues *ValidationRunIssues) error {
	var buf strings.Builder

	projectRoot := ""
	if ctx != nil && ctx.ProjectRoot != emptyValue {
		projectRoot = ctx.ProjectRoot
	}
	projectRoot = ProjectRootOrResolve(projectRoot)
	pendingAutofixBatches := countUnprocessedAutofixBatches(projectRoot)

	tierFilter, _ := cmd.Flags().GetInt("tier")
	tierFilterActive := tierFilter > 0
	terminalWidth := getTerminalWidth()

	// Group results by tier
	tier1, tier2, tier3, tier4, clean := groupResultsByTier(results)

	// Separate public and internal
	grouped := &TierGroupedResults{
		Tier1: tier1,
		Tier2: tier2,
		Tier3: tier3,
		Tier4: tier4,
		Clean: clean,
	}
	separatePublicAndInternal(grouped)

	// Output layered high-level summary
	writeLayeredSummary(&buf, results, cmd, ctx)

	allViolationsResolved := len(grouped.PublicTier1)+len(grouped.InternalTier1)+len(grouped.PublicTier2)+len(grouped.InternalTier2)+len(grouped.PublicTier3)+len(grouped.InternalTier3) == 0 && pendingAutofixBatches == 0
	if pendingAutofixBatches > 0 {
		writeUnprocessedAutofixBatches(&buf, pendingAutofixBatches, terminalWidth)
	}
	if !tierFilterActive && allViolationsResolved {
		// So that "check green" implies "0 autofix files": remove any stray AUTOFIX-*.json (e.g. from a prior run).
		clearUnprocessedAutofixBatchesWhenGreen(projectRoot)

		autoFixedCount := 0
		for _, result := range results {
			autoFixedCount += len(result.AutoFixed)
		}

		if autoFixedCount > 0 {
			buf.WriteString(fmt.Sprintf("All violations resolved (public and internal). %d issue(s) were auto-fixed.\n\n", autoFixedCount))
		} else {
			buf.WriteString("System check passed. No violations found (public and internal).\n\n")
		}
	}

	// Output details
	if ctx.Verbose {
		buf.WriteString("\n=== Detailed Check Results ===\n\n")
		writeTier1Details(&buf, grouped, terminalWidth, ctx)
		writeTier2Details(&buf, grouped, terminalWidth)
		writeTier3Details(&buf, grouped, terminalWidth, ctx)
		writeDetailedStatistics(&buf, results, grouped)
	} else if tierFilterActive {
		if tierFilter == 1 {
			writeTier1Details(&buf, grouped, terminalWidth, ctx)
		} else if tierFilter == 2 {
			writeTier2Details(&buf, grouped, terminalWidth)
		} else if tierFilter == 3 {
			writeTier3Details(&buf, grouped, terminalWidth, ctx)
		}
	}

	writeAutoFixedItems(&buf, results, terminalWidth)
	writeResolutionSummary(&buf, results, staleCASResult, terminalWidth)
	writeAuditEventBufferStatus(&buf, bufferCount, bufferSummary)

	// Validation run issues: visible summary so users don't have to dig through verbose logs
	if runIssues != nil && (runIssues.ForceComplete || runIssues.WorkerStopTimedOut || runIssues.CacheSaveTimedOut) {
		writeValidationRunIssues(&buf, runIssues, terminalWidth)
	}

	// Stream buffered string directly to output writer without duplicate string-to-bytes buffer copies
	outWriter := logging.GetCommandOutputWriter(cli.CommandContextOr(cmd, nil))
	_, err := outWriter.Write([]byte(buf.String()))
	return err
}

// writeValidationRunIssues writes a prominent "Validation run issues" section so these are visible without digging through logs.
func writeValidationRunIssues(buf *strings.Builder, runIssues *ValidationRunIssues, terminalWidth int) {
	buf.WriteString("=== Validation run issues (check logs for details) ===\n")
	buf.WriteString("⚠️  This run had issues that may affect results. Review logs if counts or cache seem wrong:\n\n")
	if runIssues.ForceComplete {
		buf.WriteString("  • Force-complete: queue was empty for 15s+ but some tasks were not accounted for. Results may be partial.\n")
	}
	if runIssues.WorkerStopTimedOut {
		buf.WriteString("  • Worker stop timeout: one or more workers did not stop in time; they may have been stuck.\n")
	}
	if runIssues.CacheSaveTimedOut {
		buf.WriteString("  • Cache save timeout: validation state cache may not have been saved. Next run may re-validate more objects.\n")
	}
	buf.WriteString("\n")
}

// writeUnprocessedAutofixBatches writes a Tier-1-style message when unprocessed AUTOFIX-*.json files exist.
// So "healthy" is not reported until batches are processed or removed.
func writeUnprocessedAutofixBatches(buf *strings.Builder, count int, terminalWidth int) {
	buf.WriteString("=== Unprocessed autofix batches (blocking) ===\n")
	fmt.Fprintf(buf, "⚠️  %d unprocessed batch file(s) under .zqk/autofix/ — fixes have not been applied.\n", count)
	buf.WriteString("   Run: zqk system auto-fix-batch --batch-file .zqk/autofix/<file> --project-root <root>\n")
	buf.WriteString("   Or wait for the scheduler to process the batch job.\n\n")
}

const (
	roleSecurityEngineer        = "Security-Engineer"
	roleQAEngineer              = "QA-Engineer"
	roleTechnicalProgramManager = "technical-program-manager"
	dispatchAuto                = "auto"
	promptFormatAgentPrompt     = "agent-prompt"
	categoryPolicy              = "policy"
	categoryReference           = "reference"
	categoryIntegrity           = "integrity"
	categoryRegistration        = "registration"
)

func dispatchViolationsToInbox(projectRoot string, results []CheckResult, dispatchTo string) error {
	for _, res := range results {
		if len(res.Issues) == 0 {
			continue
		}
		// Determine recipient role
		role := dispatchTo
		if role == dispatchAuto || role == "" {
			role = determineRoleForViolation(res)
		}

		// Build Markdown body
		var sb strings.Builder
		sb.WriteString("# ZQK System Health Violation Alert\n\n")
		sb.WriteString("## Object Details\n")
		fmt.Fprintf(&sb, "- **Object ID**: `%s`\n", res.ObjectID)
		fmt.Fprintf(&sb, "- **Object Kind**: `%s`\n", res.ObjectKind)
		fmt.Fprintf(&sb, "- **File Path**: `%s`\n\n", res.FilePath)

		sb.WriteString("## Detected Violations\n")
		for idx, issue := range res.Issues {
			fmt.Fprintf(&sb, "### Violation %d [Tier %d] - Category: `%s`\n", idx+1, issue.Tier, issue.Category)
			fmt.Fprintf(&sb, "- **Message**: %s\n", issue.Message)
			fmt.Fprintf(&sb, "- **Auto-Fixable**: %t\n", issue.AutoFixable)
			if issue.FixCommand != "" {
				fmt.Fprintf(&sb, "- **Fix Command**: `%s`\n", issue.FixCommand)
			}
			sb.WriteString("\n")
		}
		sb.WriteString("---\n")
		sb.WriteString("Please remedy these compliance/health violations as soon as possible.\n")

		// Create delivery Prompt
		filename := fmt.Sprintf("violation_%s_%d.md", res.ObjectID, time.Now().UnixNano())
		destPath := filepath.Join(projectRoot, paths.ProjectDataDir, "inbox", role, filename)

		prompt := agentdelivery.Prompt{
			Markdown: []byte(sb.String()),
			Format:   promptFormatAgentPrompt,
			DestPath: destPath,
		}

		if err := fileutil.EnsureDir(filepath.Dir(destPath)); err != nil {
			return errfmt.Newf("failed to create directory for violation prompt").Wrap(err)
		}
		if err := fileutil.WriteSecureFile(destPath, prompt.Markdown); err != nil {
			return errfmt.Newf("failed to deliver violation prompt to %s", role).Wrap(err)
		}
	}
	return nil
}

func determineRoleForViolation(res CheckResult) string {
	// Rule-based routing to appropriate agent roles
	for _, issue := range res.Issues {
		if issue.Category == categoryPolicy || res.ObjectKind == objects.KindPolicy || res.ObjectKind == objects.KindRule {
			return roleSecurityEngineer
		}
		if issue.Category == categoryReference || res.ObjectKind == objects.KindTestCase {
			return roleQAEngineer
		}
	}
	return roleTechnicalProgramManager
}

func isStaticConfigKind(kind string) bool {
	switch kind {
	case "prompt_template", "sampler_profile", "test_command_rule", "kind_synonym",
		"auth_strategy", "bucketing_strategy", "namespace", "namespace_registry",
		"domain_registry", "vocabulary_scheme", "command_spec", "api_spec", "role",
		"policy", "rule", "metadata_package", "context_refresh_schedule":
		return true
	default:
		return false
	}
}

// writeLayeredSummary constructs a high-level summary of the check results structured in 3 layers:
// Layer 0: CAS & Integrity Blockers
// Layer 1: Objects Needing Fixes (separated into Draft/Exploring vs Active/In-Flight views)
// Layer 2: Active Priority Plans ready for execution
// Layer 3: Completed and successfully validated items
func writeLayeredSummary(buf *strings.Builder, results []CheckResult, cmd *cobra.Command, ctx *cli.Context) {
	var layer0Issues []CheckResult
	var activeIssues []CheckResult
	var draftIssues []CheckResult
	var readyObjects []CheckResult
	var completeObjects []CheckResult
	var staticObjects []CheckResult
	includeIds, _ := cmd.Flags().GetBool("include-ids")
	detailsFlag, _ := cmd.Flags().GetBool("details")
	details := includeIds || detailsFlag

	for _, r := range results {
		status := strings.ToLower(r.Status)
		status = strings.ReplaceAll(status, " ", "_")
		isTerminal := false
		if r.Status != "" && r.ObjectKind != "" {
			isTerminal, _ = objects.GetGlobalLifecycleLoader().IsTerminalStatusForKind(r.ObjectKind, r.Status)
		}

		// Treat "error" or "failed" status as a lifecycle blocker issue
		if status == "error" || status == "failed" {
			hasErrorStatusIssue := false
			for _, issue := range r.Issues {
				if strings.Contains(issue.Message, "status is") {
					hasErrorStatusIssue = true
					break
				}
			}
			if !hasErrorStatusIssue {
				r.Issues = append(r.Issues, Issue{
					Category: "lifecycle",
					Message:  fmt.Sprintf("Object is in %q status", r.Status),
					Tier:     1,
				})
			}
		}

		if len(r.Issues) == 0 {
			if r.Status == "" || isStaticConfigKind(r.ObjectKind) {
				staticObjects = append(staticObjects, r)
			} else if isTerminal {
				completeObjects = append(completeObjects, r)
			} else {
				isPreliminary := false
				if r.Status != "" && r.ObjectKind != "" {
					isPreliminary, _ = objects.GetGlobalLifecycleLoader().IsPreliminaryStatusForKind(r.ObjectKind, r.Status)
				}
				if !isPreliminary {
					readyObjects = append(readyObjects, r)
				} else {
					r.Issues = append(r.Issues, Issue{
						Category: "lifecycle",
						Message:  fmt.Sprintf("Object is in preliminary status %q", r.Status),
						Tier:     4,
					})
					draftIssues = append(draftIssues, r)
				}
			}
		} else {
			// Check if there are any Layer 0 (CAS/Integrity) issues
			hasCASIssue := false
			for _, issue := range r.Issues {
				if issue.Category == categoryIntegrity || issue.Category == categoryRegistration {
					hasCASIssue = true
					break
				}
			}

			if hasCASIssue {
				layer0Issues = append(layer0Issues, r)
			} else {
				isActive := status == objects.ObjectStatusActive || status == objects.ObjectStatusInProgress || status == objects.ObjectStatusPlanned || status == "error" || status == "failed"
				if isActive {
					activeIssues = append(activeIssues, r)
				} else {
					draftIssues = append(draftIssues, r)
				}
			}
		}
	}

	buf.WriteString("\n=== System Check: High-Level Layered Summary ===\n\n")

	// Layer 0: CAS & Integrity Blockers
	{
		headers := []string{"Issue", "Severity", "Kind"}
		var rows [][]string
		if len(layer0Issues) > 0 {
			rows = getGroupedLayer0Rows(layer0Issues, details)
		} else {
			rows = append(rows, []string{"No CAS or integrity blockers found.", "-", "-"})
		}
		buf.WriteString(renderTableWithTitle("Layer 0: CAS & Integrity Blockers", headers, rows))
		buf.WriteString("\n")
	}

	// Layer 1: Objects to Fix (Draft View)
	{
		headers := []string{"Issue", "Severity", "Kind"}
		var rows [][]string
		if len(draftIssues) > 0 {
			rows = getGroupedIssueRows(draftIssues, details)
		} else {
			rows = append(rows, []string{"No draft issues found.", "-", "-"})
		}
		buf.WriteString(renderTableWithTitle("Layer 1: Preliminary & Draft Items (Draft/Exploring View)", headers, rows))
		buf.WriteString("\n")
	}

	// Layer 1: Objects to Fix (Active View)
	{
		headers := []string{"Issue", "Severity", "Kind"}
		var rows [][]string
		if len(activeIssues) > 0 {
			rows = getGroupedIssueRows(activeIssues, details)
		} else {
			rows = append(rows, []string{"No active issues found.", "-", "-"})
		}
		buf.WriteString(renderTableWithTitle("Layer 1: Objects Needing Fixes (Active/In-Flight View)", headers, rows))
		buf.WriteString("\n")
	}

	// Layer 2: Ready for Execution/Promotion
	{
		headers := []string{"Kind", "Ready Count", "Status Breakdown"}
		var rows [][]string
		if len(readyObjects) > 0 {
			byKind := make(map[string][]CheckResult)
			for _, r := range readyObjects {
				byKind[r.ObjectKind] = append(byKind[r.ObjectKind], r)
			}
			var kinds []string
			for k := range byKind {
				kinds = append(kinds, k)
			}
			sort.Strings(kinds)

			for _, kind := range kinds {
				objs := byKind[kind]
				statusCounts := make(map[string]int)
				for _, r := range objs {
					status := strings.ToLower(r.Status)
					if status == "" {
						status = "unknown"
					}
					statusCounts[status]++
				}
				var parts []string
				var statuses []string
				for s := range statusCounts {
					statuses = append(statuses, s)
				}
				sort.Strings(statuses)
				for _, s := range statuses {
					parts = append(parts, fmt.Sprintf("%d %s", statusCounts[s], s))
				}
				statusStr := strings.Join(parts, ", ")
				rows = append(rows, []string{formatColumnHeader(kind), fmt.Sprintf("%d", len(objs)), statusStr})
			}
		} else {
			rows = append(rows, []string{"-", "-", "No objects ready for execution or promotion."})
		}
		buf.WriteString(renderTableWithTitle("Layer 2: Objects Ready for Execution/Promotion", headers, rows))
		buf.WriteString("\n")
	}

	// Layer 3: Completed Items
	{
		headers := []string{"Kind", "Completed Count", "Status Breakdown"}
		var rows [][]string
		if len(completeObjects) > 0 {
			byKind := make(map[string][]CheckResult)
			for _, r := range completeObjects {
				byKind[r.ObjectKind] = append(byKind[r.ObjectKind], r)
			}
			var kinds []string
			for k := range byKind {
				kinds = append(kinds, k)
			}
			sort.Strings(kinds)

			for _, kind := range kinds {
				objs := byKind[kind]
				statusCounts := make(map[string]int)
				for _, r := range objs {
					status := strings.ToLower(r.Status)
					if status == "" {
						status = "unknown"
					}
					statusCounts[status]++
				}
				var parts []string
				var statuses []string
				for s := range statusCounts {
					statuses = append(statuses, s)
				}
				sort.Strings(statuses)
				for _, s := range statuses {
					parts = append(parts, fmt.Sprintf("%d %s", statusCounts[s], s))
				}
				statusStr := strings.Join(parts, ", ")
				rows = append(rows, []string{formatColumnHeader(kind), fmt.Sprintf("%d", len(objs)), statusStr})
			}
		} else {
			rows = append(rows, []string{"-", "-", "No completed objects found in this check run."})
		}
		buf.WriteString(renderTableWithTitle("Layer 3: Completed and Validated Items", headers, rows))
		buf.WriteString("\n")
	}

	// Static Configuration Health
	if len(staticObjects) > 0 {
		byKind := make(map[string]int)
		for _, r := range staticObjects {
			byKind[r.ObjectKind]++
		}
		var kinds []string
		for k := range byKind {
			kinds = append(kinds, k)
		}
		sort.Strings(kinds)

		var headers = []string{"Kind", "Compliant Count"}
		var rows [][]string
		for _, kind := range kinds {
			rows = append(rows, []string{formatColumnHeader(kind), fmt.Sprintf("%d", byKind[kind])})
		}
		buf.WriteString(renderTableWithTitle("Static configurations and specifications", headers, rows))
		buf.WriteString("\n")
	}
}

func getGroupedLayer0Rows(results []CheckResult, details bool) [][]string {
	type groupKey struct {
		Kind    string
		Message string
	}
	type groupInfo struct {
		Tier        int
		AutoFixable bool
		IDs         []string
	}

	groups := make(map[groupKey]*groupInfo)
	var keys []groupKey

	for _, r := range results {
		for _, issue := range r.Issues {
			if issue.Category == categoryIntegrity || issue.Category == categoryRegistration {
				normMsg := normalizeErrorMessage(issue.Message)
				key := groupKey{
					Kind:    r.ObjectKind,
					Message: normMsg,
				}
				info, exists := groups[key]
				if !exists {
					info = &groupInfo{
						Tier:        issue.Tier,
						AutoFixable: issue.AutoFixable,
					}
					groups[key] = info
					keys = append(keys, key)
				}
				info.IDs = append(info.IDs, r.ObjectID)
				if issue.AutoFixable {
					info.AutoFixable = true
				}
			}
		}
	}

	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Message != keys[j].Message {
			return keys[i].Message < keys[j].Message
		}
		return keys[i].Kind < keys[j].Kind
	})

	var rows [][]string
	var lastMessage string
	for _, key := range keys {
		info := groups[key]
		severitySymbol := "🟡"
		if info.Tier == 1 {
			severitySymbol = "❌"
		} else if info.Tier == 3 {
			severitySymbol = "🔹"
		} else if info.Tier == 4 {
			severitySymbol = "💡"
		}

		fixableStr := ""
		if info.AutoFixable {
			fixableStr = "  🔧 [Auto-fix]"
		}

		var suffix string
		if details {
			suffix = fixableStr + fmt.Sprintf(" (IDs: %s)", strings.Join(info.IDs, ", "))
		} else {
			suffix = fixableStr
		}

		msgText := key.Message + suffix
		displayMsg := msgText
		displaySeverity := severitySymbol
		if displayMsg == lastMessage {
			displayMsg = ""
			displaySeverity = ""
		} else {
			lastMessage = displayMsg
		}

		kindVal := fmt.Sprintf("%s (%d)", formatColumnHeader(key.Kind), len(info.IDs))
		rows = append(rows, []string{displayMsg, displaySeverity, kindVal})
	}
	return rows
}

func getGroupedIssueRows(results []CheckResult, details bool) [][]string {
	type groupKey struct {
		Kind    string
		Status  string
		Message string
	}
	type groupInfo struct {
		Tier        int
		AutoFixable bool
		IDs         []string
	}

	groups := make(map[groupKey]*groupInfo)
	var keys []groupKey

	for _, r := range results {
		for _, issue := range r.Issues {
			normMsg := normalizeErrorMessage(issue.Message)
			key := groupKey{
				Kind:    r.ObjectKind,
				Status:  r.Status,
				Message: normMsg,
			}
			info, exists := groups[key]
			if !exists {
				info = &groupInfo{
					Tier:        issue.Tier,
					AutoFixable: issue.AutoFixable,
				}
				groups[key] = info
				keys = append(keys, key)
			}
			info.IDs = append(info.IDs, r.ObjectID)
			if issue.AutoFixable {
				info.AutoFixable = true
			}
		}
	}

	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Message != keys[j].Message {
			return keys[i].Message < keys[j].Message
		}
		return keys[i].Kind < keys[j].Kind
	})

	var rows [][]string
	var lastMessage string
	for _, key := range keys {
		info := groups[key]
		severitySymbol := "🟡"
		if info.Tier == 1 {
			severitySymbol = "❌"
		} else if info.Tier == 3 {
			severitySymbol = "🔹"
		} else if info.Tier == 4 {
			severitySymbol = "💡"
		}

		fixableStr := ""
		if info.AutoFixable {
			fixableStr = "  🔧 [Auto-fix]"
		}

		var suffix string
		if details {
			suffix = fixableStr + fmt.Sprintf(" (IDs: %s)", strings.Join(info.IDs, ", "))
		} else {
			suffix = fixableStr
		}

		var kindVal string
		if key.Status != "" && !strings.Contains(strings.ToLower(key.Message), strings.ToLower(key.Status)) {
			kindVal = fmt.Sprintf("%s (%d, status: %q)", formatColumnHeader(key.Kind), len(info.IDs), key.Status)
		} else {
			kindVal = fmt.Sprintf("%s (%d)", formatColumnHeader(key.Kind), len(info.IDs))
		}

		msgText := key.Message + suffix
		displayMsg := msgText
		displaySeverity := severitySymbol
		if displayMsg == lastMessage {
			displayMsg = ""
			displaySeverity = ""
		} else {
			lastMessage = displayMsg
		}

		rows = append(rows, []string{displayMsg, displaySeverity, kindVal})
	}
	return rows
}

func renderTableWithTitle(title string, headers []string, rows [][]string) string {
	if len(rows) == 0 {
		return ""
	}

	// Determine optimal column widths to sum up to exactly:
	// - 93 for 2 columns (overhead = 7, total = 100)
	// - 90 for 3 columns (overhead = 10, total = 100)
	widths := make([]int, len(headers))
	if len(headers) == 2 {
		// Static configurations
		widths[0] = 30
		widths[1] = 63
	} else if len(headers) == 3 {
		if headers[0] == "ISSUE" || headers[0] == "Issue" {
			// Layer 0 and Layer 1: Issue, Severity, Kind
			widths[0] = 55 // Issue message
			widths[1] = 10 // Severity icon
			widths[2] = 25 // Kind (count, status)
		} else if headers[1] == "READY COUNT" || headers[1] == "Ready Count" {
			// Layer 2
			widths[0] = 30
			widths[1] = 15
			widths[2] = 45
		} else if headers[1] == "COMPLETED COUNT" || headers[1] == "Completed Count" {
			// Layer 3
			widths[0] = 30
			widths[1] = 17
			widths[2] = 43
		} else {
			widths[0] = 30
			widths[1] = 15
			widths[2] = 45
		}
	} else {
		// Fallback dynamic width calculation
		for i, h := range headers {
			widths[i] = len(h)
		}
		for _, row := range rows {
			for i, val := range row {
				if i < len(widths) {
					if len(val) > widths[i] {
						widths[i] = len(val)
					}
				}
			}
		}
	}

	return pkgcli.RenderTableWithTitleAndWrap(title, headers, widths, rows)
}

func renderTable(headers []string, rows [][]string) string {
	return renderTableWithTitle("", headers, rows)
}

func formatColumnHeader(s string) string {
	s = strings.ReplaceAll(s, "_", " ")
	if len(s) == 0 {
		return ""
	}
	return strings.ToUpper(s[0:1]) + s[1:]
}

func filterResultsByLayer(results []CheckResult, layer int) []CheckResult {
	var filtered []CheckResult
	for _, r := range results {
		status := strings.ToLower(r.Status)
		isTerminal := false
		if r.Status != "" && r.ObjectKind != "" {
			isTerminal, _ = objects.GetGlobalLifecycleLoader().IsTerminalStatusForKind(r.ObjectKind, r.Status)
		}
		hasIssues := len(r.Issues) > 0 || status == "error" || status == "failed"

		// Identify CAS/Integrity layer 0 issue
		hasCASIssue := false
		if len(r.Issues) > 0 {
			for _, issue := range r.Issues {
				if issue.Category == categoryIntegrity || issue.Category == categoryRegistration {
					hasCASIssue = true
					break
				}
			}
		}

		isPreliminary := false
		if r.Status != "" && r.ObjectKind != "" {
			isPreliminary, _ = objects.GetGlobalLifecycleLoader().IsPreliminaryStatusForKind(r.ObjectKind, r.Status)
		}

		switch layer {
		case 0:
			if hasCASIssue {
				filtered = append(filtered, r)
			}
		case 1:
			if (hasIssues && !hasCASIssue) || (!hasIssues && r.Status != "" && !isTerminal && isPreliminary) {
				filtered = append(filtered, r)
			}
		case 2:
			if !hasIssues && r.Status != "" && !isTerminal && !isPreliminary {
				filtered = append(filtered, r)
			}
		case 3:
			if !hasIssues && r.Status != "" && isTerminal {
				filtered = append(filtered, r)
			}
		}
	}
	return filtered
}

func normalizeErrorMessage(msg string) string {
	// 1. Normalize scheduler job IDs (SCH-run-bundle-XX)
	reSched := regexp.MustCompile(`SCH-run-bundle-\d+`)
	msg = reSched.ReplaceAllString(msg, "SCH-run-bundle-*")

	// 2. Normalize hex yaml filenames (64 hex characters + .yaml)
	reHexYaml := regexp.MustCompile(`[a-f0-9]{64}\.yaml`)
	msg = reHexYaml.ReplaceAllString(msg, "<hash>.yaml")

	// 3. Normalize general IDs in the message, e.g., "for object XXX" or "object XXX"
	// Only match strings starting with uppercase letters/digits or containing dashes/colons (avoiding lowercase words like get, exists, cache)
	reObjID := regexp.MustCompile(`object\s+([A-Z0-9][A-Za-z0-9_-]*:[A-Za-z0-9_-]+|[A-Z0-9]{2,10}-[A-Za-z0-9_-]+)`)
	msg = reObjID.ReplaceAllString(msg, "object <id>")

	// 4. Normalize file paths (anything containing docs/process/ or absolute paths)
	rePath := regexp.MustCompile(`/Users/[A-Za-z0-9_./-]+`)
	msg = rePath.ReplaceAllString(msg, "<path>")

	// 5. Normalize modification durations, e.g. "file modified 4m31s ago"
	reFileMod := regexp.MustCompile(`file modified\s+[0-9a-zA-Z.]+\s+ago`)
	msg = reFileMod.ReplaceAllString(msg, "file modified <duration> ago")

	// 6. Normalize any hex hashes with dots, e.g. "fc4c134c51989081..." or "fc4c134c51989081...."
	reHexDots := regexp.MustCompile(`(?i)[a-f0-9]{8,64}\.\.\.*`)
	msg = reHexDots.ReplaceAllString(msg, "<hash>...")

	return msg
}

func detectOrphanedFiles(cmd *cobra.Command, projectRoot string, results []CheckResult, autoFix bool) []CheckResult {
	if projectRoot == "" {
		return results
	}

	var isFullCheck bool
	if cmd != nil {
		args := cmd.Flags().Args()
		isFullCheck = len(args) == 0 || (len(args) == 1 && args[0] == "all")
	} else {
		isFullCheck = true
	}
	if !isFullCheck {
		return results
	}

	// Build map of tracked absolute file paths
	trackedPaths := make(map[string]bool)
	for _, r := range results {
		if r.FilePath != "" {
			trackedPaths[filepath.Clean(r.FilePath)] = true
		}
	}

	processDir := filepath.Join(projectRoot, "docs/process")
	var orphanedResults []CheckResult

	// Get all registered kinds
	kinds := objects.GetGlobalKindMapper().GetAllKinds()
	for _, kind := range kinds {
		// Skip high-volume, stream-backed, or automated kinds that do not participate in CAS index registry
		if kind == objects.KindAuditEvent || kind == objects.KindChangeJournalEntry || kind == objects.KindMcpSession || kind == objects.KindSchedulerJob || kind == "command_spec" || (len(kind) > 7 && kind[len(kind)-7:] == "_metric") {
			continue
		}

		dirName := objects.GetDirectoryFromKind(kind)
		if dirName == "" {
			continue
		}
		kindDir := filepath.Join(processDir, dirName)

		// Walk kind directory if it exists
		if _, err := os.Stat(kindDir); err != nil {
			continue
		}

		// Load CAS index mapping for this kind to prevent deleting valid CAS files
		var indexMappings map[string]string
		cas := storage.NewContentAddressableStorage(kindDir, kind)
		if cas != nil {
			idx := cas.GetIndex()
			if idx != nil {
				indexMappings = idx.Mappings
			}
		}

		_ = filepath.Walk(kindDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if info.IsDir() {
				name := info.Name()
				if name == "schemas" || name == "_internal" || strings.HasPrefix(name, ".") {
					return filepath.SkipDir
				}
				return nil
			}
			ext := strings.ToLower(filepath.Ext(path))
			if ext != ".yaml" && ext != ".yml" {
				return nil
			}
			filename := filepath.Base(path)
			if strings.HasPrefix(filename, ".") {
				return nil
			}

			cleanPath := filepath.Clean(path)
			if !trackedPaths[cleanPath] {
				// Check if this is a registered CAS file
				ext := filepath.Ext(filename)
				nameWithoutExt := strings.TrimSuffix(filename, ext)
				if len(nameWithoutExt) == 64 {
					isRegistered := false
					for _, h := range indexMappings {
						if h == nameWithoutExt {
							isRegistered = true
							break
						}
					}
					if isRegistered {
						return nil // Registered in CAS index, NOT orphaned!
					}
				}

				// Orphaned file found!
				id := strings.TrimSuffix(filename, filepath.Ext(filename))

				if autoFix {
					// Delete the orphaned file
					removeErr := os.Remove(cleanPath)
					if removeErr == nil {
						orphanedResults = append(orphanedResults, CheckResult{
							ObjectID:   id,
							ObjectKind: kind,
							FilePath:   cleanPath,
							Issues:     nil, // Cleaned up
							AutoFixed:  []string{fmt.Sprintf("deleted orphaned file: %s", filepath.Base(cleanPath))},
						})
					} else {
						orphanedResults = append(orphanedResults, CheckResult{
							ObjectID:   id,
							ObjectKind: kind,
							FilePath:   cleanPath,
							Issues: []Issue{
								{
									Tier:        1,
									Category:    "integrity",
									Message:     fmt.Sprintf("Orphaned file on disk could not be deleted: %v", removeErr),
									AutoFixable: true,
								},
							},
						})
					}
				} else {
					orphanedResults = append(orphanedResults, CheckResult{
						ObjectID:   id,
						ObjectKind: kind,
						FilePath:   cleanPath,
						Issues: []Issue{
							{
								Tier:        1,
								Category:    "integrity",
								Message:     "Orphaned file on disk: file exists in docs/process but is not registered in the database index (recovery or pruning required)",
								AutoFixable: true,
							},
						},
					})
				}
			}
			return nil
		})
	}

	return append(results, orphanedResults...)
}
