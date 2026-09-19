package system

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/diskusage"
	"github.com/zqk-os/zqk/pkg/resourcehygiene"
	caspkg "github.com/zqk-os/zqk/pkg/storage/cas"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/agentdelivery"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/when"
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

	// Full-kernel live green ⇒ pending AUTOFIX-*.json are stale snapshots; clear before
	// summaries so JSON/YAML/table do not keep reporting pending_autofix_batches blockers.
	maybeClearStaleAutofixBatchesAfterLiveGreen(cmd, ctx.ProjectRoot, results)

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

	// Opt-in --notify: wake seat when thresholds trip (all output formats).
	maybeNotifySystemCheckWake(cmd, ctx.ProjectRoot, results)

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
		return evaluateSystemCheckPristine(results, outputCtx.RunIssues, ctx.ProjectRoot)
	}

	// Handle streaming formats
	if err := handleStreamingFormat(cmd, ctx, results, outputCtx); err != nil {
		return err
	}
	// If streaming format was handled, return early
	if outputCtx.Handler != nil && outputCtx.Handler.IsStreaming() {
		return evaluateSystemCheckPristine(results, outputCtx.RunIssues, ctx.ProjectRoot)
	}

	// Handle non-streaming formats with handlers
	if err := handleNonStreamingFormat(cmd, results, outputCtx); err != nil {
		return err
	}
	// If non-streaming format handler was used, return early
	if outputCtx.Handler != nil && !outputCtx.Handler.IsStreaming() {
		return evaluateSystemCheckPristine(results, outputCtx.RunIssues, ctx.ProjectRoot)
	}

	// Fallback to legacy format handlers (only if no handler was used)
	if err := handleLegacyFormat(cmd, ctx, results, outputCtx); err != nil {
		return err
	}
	return evaluateSystemCheckPristine(results, outputCtx.RunIssues, ctx.ProjectRoot)
}

// countUnprocessedAutofixBatches returns the number of AUTOFIX-*.json files under .zqk/autofix/
// that have not yet been processed (i.e. not renamed to FIXED_/PROCESSED_). Used so system check
// does not report "healthy" when fixes were batched but never applied.
func countUnprocessedAutofixBatches(projectRoot string) int {
	if projectRoot == emptyValue {
		return 0
	}
	autofixDir := filepath.Join(projectRoot, paths.ProjectDataDir, paths.AutofixDir)
	entries, err := fileutil.ReadDir(autofixDir)
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
	entries, err := fileutil.ReadDir(autofixDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(name, "AUTOFIX-") && strings.HasSuffix(name, ".json") {
			_ = fileutil.Remove(filepath.Join(autofixDir, name))
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
func PrepareCompactCheckOutputData(cmd *cobra.Command, results []CheckResult, bufferCount int, bufferSummary map[string]int, projectRoot string) any {
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
		ErrorStatusObjects    int            `json:"error_status_objects"`
		AutoFixed             int            `json:"auto_fixed"`
		GhostRefCount         int                                  `json:"ghost_ref_count,omitempty"`
		PendingAutofixBatches int                                  `json:"pending_autofix_batches,omitempty"`
		AuditEventsBuffered   int                                  `json:"audit_events_buffered,omitempty"`
		AuditEventsSummary    map[string]int                       `json:"audit_events_summary,omitempty"`
		IOResourceTelemetry   *resourcehygiene.IOResourceTelemetry `json:"io_resource_telemetry,omitempty"`
	}{}
	summary.TotalObjects = len(results)
	if projectRoot != emptyValue {
		if ioTel, err := resourcehygiene.InspectIOResources(context.Background(), projectRoot); err == nil {
			summary.IOResourceTelemetry = ioTel
		}
	}
	casDupInv := inventoryCASDuplicateIDsForOutput(cmd, projectRoot)
	results = appendCASDuplicateIDCheckResults(results, casDupInv)
	for _, result := range results {
		isInternal := isInternalKind(result.ObjectKind)
		when.When(func() bool { return isInternal }).Then(func() { summary.InternalObjects++ }).OrElse(func() { summary.PublicObjects++ }).Run()
		summary.AutoFixed += len(result.AutoFixed)
		if result.Status == objects.ObjectStatusError {
			summary.ErrorStatusObjects++
		}
		for _, issue := range result.Issues {
			summary.TotalIssues++
			if issue.Category == "GhostRef" {
				summary.GhostRefCount++
			}
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
	// Unprocessed autofix batches mean fixes were never applied; do not report healthy (POL-CODE-005)
	if pending := countUnprocessedAutofixBatches(projectRoot); pending > 0 {
		summary.BlockingIssues++
		summary.InternalBlocking++
		summary.PendingAutofixBatches = pending
	}
	draftInv := storage.InventoryObjectDraftPlane(projectRoot)
	// Dual-plane split-brain: draft + CAS for same id → Get sees draft, check sees CAS.
	if n := len(draftInv.DualPlaneIDs); n > 0 {
		summary.BlockingIssues += n
		summary.PublicBlocking += n
		summary.TotalIssues += n
	}
	partial := checkFastModeEnabled(cmd)
	msg := ""
	switch {
	case partial && bufferCount > 0:
		msg = checkFastPartialVerdict + ". " + fmt.Sprintf("Note: %d audit event(s) are being buffered and will be written/flushed after this operation completes.", bufferCount)
	case partial:
		msg = checkFastPartialVerdict
	case bufferCount > 0:
		msg = fmt.Sprintf("Note: %d audit event(s) are being buffered and will be written/flushed after this operation completes. Run the command again to see final audit event counts.", bufferCount)
	}
	return struct {
		Summary          interface{}                        `json:"summary"`
		ResultsByKind    map[string][]compactCheckEntry     `json:"results_by_kind"`
		ObjectDraftPlane *storage.ObjectDraftPlaneInventory `json:"object_draft_plane,omitempty"`
		CASDuplicateIDs  *caspkg.CASDuplicateIDInventory    `json:"cas_duplicate_ids,omitempty"`
		PartialCheck     bool                               `json:"partial_check,omitempty"`
		Message          string                             `json:"message,omitempty"`
	}{
		Summary:          summary,
		ResultsByKind:    buildResultsByKind(results, projectRoot),
		ObjectDraftPlane: objectDraftPlaneInventoryPtr(draftInv),
		CASDuplicateIDs:  caspkg.CASDuplicateIDInventoryPtr(casDupInv),
		PartialCheck:     partial,
		Message:          msg,
	}
}

func objectDraftPlaneInventoryPtr(inv storage.ObjectDraftPlaneInventory) *storage.ObjectDraftPlaneInventory {
	if inv.Total == 0 {
		return nil
	}
	out := inv
	return &out
}

// buildResultsByKind buckets results by kind and normalizes file paths to reduce character count.
// Paths are stored relative to projectRoot (e.g. ".zqk/process/audit/..." instead of full absolute path).
func buildResultsByKind(results []CheckResult, projectRoot string) map[string][]compactCheckEntry {
	byKind := make(map[string][]compactCheckEntry)
	prefix := filepath.Clean(projectRoot)
	if prefix != emptyValue && !strings.HasSuffix(prefix, string(fileutil.PathSeparator)) {
		prefix += string(fileutil.PathSeparator)
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
	casDupInv := inventoryCASDuplicateIDsForOutput(cmd, projectRoot)
	results = appendCASDuplicateIDCheckResults(results, casDupInv)
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
	draftInv := storage.InventoryObjectDraftPlane(projectRoot)
	if n := len(draftInv.DualPlaneIDs); n > 0 {
		summary.BlockingIssues += n
		summary.PublicBlocking += n
		summary.TotalIssues += n
	}

	output := struct {
		Summary          interface{}                        `json:"summary"`
		ResultsByKind    map[string][]compactCheckEntry     `json:"results_by_kind"`
		ObjectDraftPlane *storage.ObjectDraftPlaneInventory `json:"object_draft_plane,omitempty"`
		CASDuplicateIDs  *caspkg.CASDuplicateIDInventory    `json:"cas_duplicate_ids,omitempty"`
		PartialCheck     bool                               `json:"partial_check,omitempty"`
		Message          string                             `json:"message,omitempty"`
	}{
		Summary:          summary,
		ResultsByKind:    buildResultsByKind(results, projectRoot),
		ObjectDraftPlane: objectDraftPlaneInventoryPtr(draftInv),
		CASDuplicateIDs:  caspkg.CASDuplicateIDInventoryPtr(casDupInv),
		PartialCheck:     checkFastModeEnabled(cmd),
	}
	switch {
	case checkFastModeEnabled(cmd) && bufferCount > 0:
		output.Message = checkFastPartialVerdict + ". " + fmt.Sprintf("Note: %d audit event(s) are being buffered and will be written/flushed after this operation completes.", bufferCount)
	case checkFastModeEnabled(cmd):
		output.Message = checkFastPartialVerdict
	case bufferCount > 0:
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
		file, err := fileutil.Create(outputPath)
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
		Results          []CheckResult                      `yaml:"results"`
		ObjectDraftPlane *storage.ObjectDraftPlaneInventory `yaml:"object_draft_plane,omitempty"`
		CASDuplicateIDs  *caspkg.CASDuplicateIDInventory    `yaml:"cas_duplicate_ids,omitempty"`
		Message          string                             `yaml:"message,omitempty"`
	}{}

	// Calculate summary statistics
	output.Summary.TotalObjects = len(results)
	casDupInv := inventoryCASDuplicateIDsForOutput(cmd, projectRoot)
	results = appendCASDuplicateIDCheckResults(results, casDupInv)
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
	draftInv := storage.InventoryObjectDraftPlane(projectRoot)
	if n := len(draftInv.DualPlaneIDs); n > 0 {
		output.Summary.BlockingIssues += n
		output.Summary.TotalIssues += n
	}
	output.ObjectDraftPlane = objectDraftPlaneInventoryPtr(draftInv)
	output.CASDuplicateIDs = caspkg.CASDuplicateIDInventoryPtr(casDupInv)

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

	tierFilter, _ := cmd.Flags().GetInt("tier")
	tierFilterActive := tierFilter > 0
	terminalWidth := getTerminalWidth()

	if !checkFastModeEnabled(cmd) && cmd != nil {
		fmt.Fprintln(cmd.ErrOrStderr(), "Verifying CAS duplicate IDs across index...")
	}
	casDupInv := inventoryCASDuplicateIDsForOutput(cmd, projectRoot)
	results = appendCASDuplicateIDCheckResults(results, casDupInv)

	if cmd != nil {
		fmt.Fprintln(cmd.ErrOrStderr(), "Formatting compliance summary...")
	}

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

	// Draft-plane objects have not met the obligations to cross the CAS membrane, so they
	// precede the layered summary: Layers 0-3 only describe objects inside the membrane.
	writeObjectDraftPlaneSummary(&buf, projectRoot)
	writeLayeredSummary(&buf, results, cmd)
	writeCASDuplicateIDSummary(&buf, casDupInv)
	writeIOResourceHygieneSummary(&buf, projectRoot)

	// Pending count may already have been cleared by maybeClearStaleAutofixBatchesAfterLiveGreen.
	pendingAutofixBatches := countUnprocessedAutofixBatches(projectRoot)
	liveClean := len(grouped.PublicTier1)+len(grouped.InternalTier1)+len(grouped.PublicTier2)+len(grouped.InternalTier2)+len(grouped.PublicTier3)+len(grouped.InternalTier3) == 0
	allViolationsResolved := liveClean && pendingAutofixBatches == 0
	if pendingAutofixBatches > 0 {
		writeUnprocessedAutofixBatches(&buf, pendingAutofixBatches, terminalWidth)
	}
	if !tierFilterActive && allViolationsResolved {
		autoFixedCount := 0
		for _, result := range results {
			autoFixedCount += len(result.AutoFixed)
		}

		if checkFastModeEnabled(cmd) {
			buf.WriteString(checkFastPartialPassLine())
		} else if autoFixedCount > 0 {
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
	categoryCacheLag            = "CacheLag"
	categoryCacheCoherence      = "cache_coherence"
	categoryGhostRef            = "GhostRef"
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

const preliminaryCASMessage = "preliminary status %q on CAS membrane (listable) — preliminary objects belong on the object draft plane; promote or park before CAS"

func preliminaryCASIssue(r CheckResult) (Issue, bool) {
	if r.Status == "" || r.ObjectKind == "" || isStaticConfigKind(r.ObjectKind) {
		return Issue{}, false
	}
	isPreliminary, _ := objects.GetGlobalLifecycleLoader().IsPreliminaryStatusForKind(r.ObjectKind, r.Status)
	if !isPreliminary {
		return Issue{}, false
	}
	return Issue{
		Category: categoryIntegrity,
		Message:  fmt.Sprintf(preliminaryCASMessage, r.Status),
		Tier:     1,
	}, true
}

// SystemCheckError is returned when system check completes but the system is not pristine.
// It implements interface{ ExitCode() int } to map violations to specific exit codes:
//
//	Exit Code 2: Layer 0 CAS & Integrity Blockers (registry, parse, membrane violations)
//	Exit Code 1: Layer 1 / Tier 1 Blocking Violations (preconditions, broken references, error status)
//	Exit Code 3: Non-blocking Warnings (Tier 2/3/4 issues)
//	Exit Code 4: System Run Issues (timeouts, force completion, unapplied autofix batches)
type SystemCheckError struct {
	ExitCodeVal   int
	Message       string
	Layer0Count   int
	Tier1Count    int
	WarningsCount int
}

func (e *SystemCheckError) Error() string {
	return e.Message
}

func (e *SystemCheckError) ExitCode() int {
	return e.ExitCodeVal
}

// isCompletedSystemCheckError reports whether err is a finished-check verdict
// (Layer 0 / Tier 1 / warnings / run issues). Those must return to Cobra so
// ExitCode() is honored. They are not a stuck validator; logging them as
// "Validation stuck" and os.Exit(1) flattened overnight autofix into exit 1.
func isCompletedSystemCheckError(err error) bool {
	var sc *SystemCheckError
	return errors.As(err, &sc)
}

func evaluateSystemCheckPristine(results []CheckResult, runIssues *ValidationRunIssues, projectRoot string) error {
	layer0Count := 0
	tier1Count := 0
	warningsCount := 0

	for _, r := range results {
		status := strings.ToLower(r.Status)
		status = strings.ReplaceAll(status, " ", "_")
		if status == "error" || status == "failed" {
			tier1Count++
		}

		if _, isPreliminary := preliminaryCASIssue(r); isPreliminary {
			layer0Count++
		}

		autoFixedRemaining := len(r.AutoFixed)
		for _, issue := range r.Issues {
			if autoFixedRemaining > 0 && (issue.AutoFixable || autoFixedRemaining >= len(r.Issues)) {
				autoFixedRemaining--
				continue
			}
			if issue.Category == categoryIntegrity || issue.Category == categoryRegistration {
				layer0Count++
			} else if issue.Tier == 1 {
				tier1Count++
			} else if issue.Tier > 1 {
				warningsCount++
			}
		}
	}

	hasRunIssues := false
	runIssueMsg := ""
	if runIssues != nil {
		if runIssues.ForceComplete {
			hasRunIssues = true
			runIssueMsg = "validation run was force-completed"
		} else if runIssues.WorkerStopTimedOut {
			hasRunIssues = true
			runIssueMsg = "validation worker stop timed out"
		} else if runIssues.CacheSaveTimedOut {
			hasRunIssues = true
			runIssueMsg = "validation cache save timed out"
		}
	}

	unprocessedBatches := countUnprocessedAutofixBatches(projectRoot)
	if unprocessedBatches > 0 {
		hasRunIssues = true
		runIssueMsg = fmt.Sprintf("%d unprocessed autofix batch(es) pending", unprocessedBatches)
	}

	if layer0Count > 0 {
		return &SystemCheckError{
			ExitCodeVal:   2,
			Message:       fmt.Sprintf("system check failed with exit code 2: %d Layer 0 CAS & integrity blocker(s) found", layer0Count),
			Layer0Count:   layer0Count,
			Tier1Count:    tier1Count,
			WarningsCount: warningsCount,
		}
	}

	if tier1Count > 0 {
		return &SystemCheckError{
			ExitCodeVal:   1,
			Message:       fmt.Sprintf("system check failed with exit code 1: %d Tier 1 blocking violation(s) found", tier1Count),
			Layer0Count:   0,
			Tier1Count:    tier1Count,
			WarningsCount: warningsCount,
		}
	}

	if hasRunIssues {
		return &SystemCheckError{
			ExitCodeVal: 4,
			Message:     fmt.Sprintf("system check failed with exit code 4: %s", runIssueMsg),
		}
	}

	if warningsCount > 0 {
		return &SystemCheckError{
			ExitCodeVal:   3,
			Message:       fmt.Sprintf("system check failed with exit code 3: %d non-blocking warning(s) found", warningsCount),
			WarningsCount: warningsCount,
		}
	}

	return nil
}

// writeLayeredSummary summarizes objects that are inside the CAS membrane:
// Layer 0: CAS & Integrity Blockers
// Layer 1: CAS objects needing fixes
// Layer 2: Active Priority Plans ready for execution
// Layer 3: Completed and successfully validated items

func writeIOResourceHygieneSummary(buf *strings.Builder, projectRoot string) {
	if projectRoot == emptyValue {
		return
	}
	ioTel, err := resourcehygiene.InspectIOResources(context.Background(), projectRoot)
	if err != nil || ioTel == nil {
		return
	}
	buf.WriteString("┌──────────────────────────────────────────────────────────────────────────────────────────────────┐\n")
	buf.WriteString("│ Layer 4: I/O Resource Hygiene & Storage Telemetry                                                │\n")
	buf.WriteString("├────────────────────────────────┬─────────────────┬───────────────────────────────────────────────┤\n")
	buf.WriteString("│ METRIC                         │ VALUE           │ STATUS                                        │\n")
	buf.WriteString("├────────────────────────────────┼─────────────────┼───────────────────────────────────────────────┤\n")
	fdStatus := "healthy"
	if ioTel.MaxFileDescriptors > 0 && ioTel.OpenFileDescriptors > ioTel.MaxFileDescriptors*8/10 {
		fdStatus = "warning (>80% FD limit)"
	}
	fdVal := fmt.Sprintf("%d / %d", ioTel.OpenFileDescriptors, ioTel.MaxFileDescriptors)
	if ioTel.OpenFileDescriptors < 0 {
		fdVal = "unavailable"
	}
	fmt.Fprintf(buf, "│ %-30s │ %-15s │ %-45s │\n", "Open File Descriptors", fdVal, fdStatus)

	storageVal := fmt.Sprintf("%d files (%s)", ioTel.TotalZqkFiles, diskusage.FormatBytes(ioTel.TotalZqkBytes))
	fmt.Fprintf(buf, "│ %-30s │ %-15s │ %-45s │\n", ".zqk Storage Volume", storageVal, "tracked")

	lockStatus := "clean (0 stale)"
	if ioTel.StaleLocksCount > 0 {
		lockStatus = fmt.Sprintf("attention (%d stale .lock files)", ioTel.StaleLocksCount)
	}
	fmt.Fprintf(buf, "│ %-30s │ %-15d │ %-45s │\n", "Stale Lock Files", ioTel.StaleLocksCount, lockStatus)

	tempStatus := "clean (0 orphaned)"
	if ioTel.OrphanedTempCount > 0 {
		tempStatus = fmt.Sprintf("attention (%d orphaned .tmp files)", ioTel.OrphanedTempCount)
	}
	fmt.Fprintf(buf, "│ %-30s │ %-15d │ %-45s │\n", "Orphaned Temp Files", ioTel.OrphanedTempCount, tempStatus)
	buf.WriteString("└────────────────────────────────┴─────────────────┴───────────────────────────────────────────────┘\n\n")
}
