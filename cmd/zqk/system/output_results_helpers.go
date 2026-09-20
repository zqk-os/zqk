package system

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/internal/cli"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// ValidationRunIssues holds flags from the validation run (force-complete, timeouts) for visible summary output.
// Surfaces issues that would otherwise only appear as warn lines in verbose logs.
type ValidationRunIssues struct {
	ForceComplete      bool
	WorkerStopTimedOut bool
	CacheSaveTimedOut  bool
}

// OutputResultsContext groups state for output results operation
type OutputResultsContext struct {
	Format                cli.OutputFormat
	FormatStr             string
	SnapshotPath          string
	BufferCount           int
	BufferSummary         map[string]int
	Handler               cli.FormatHandler
	OutputPath            string
	StaleCASCleanupResult *StaleCASCleanupResult // When set, resolution summary is shown (check --auto-fix)
	RunIssues             *ValidationRunIssues   // When set, "Validation run issues" section is shown (visibility without log diving)
}

// initializeOutputResultsContext sets up the output results context
func initializeOutputResultsContext(cmd *cobra.Command, ctx *cli.Context, buffer *AuditEventBuffer) (*OutputResultsContext, error) {
	formatStr := getFormatString(cmd)
	format := cli.OutputFormat(formatStr)

	snapshotPath, _ := cmd.Flags().GetString("snapshot")
	handler := cli.GetFormatHandler(format)
	outputPath := cli.GetOutputPath(cmd)

	var bufferCount int
	var bufferSummary map[string]int
	if buffer != nil {
		bufferCount = buffer.GetBufferCount()
		if bufferCount > 0 {
			bufferSummary = buffer.GetBufferSummary()
		}
	}

	return &OutputResultsContext{
		Format:        format,
		FormatStr:     formatStr,
		SnapshotPath:  snapshotPath,
		BufferCount:   bufferCount,
		BufferSummary: bufferSummary,
		Handler:       handler,
		OutputPath:    outputPath,
	}, nil
}

// getFormatString gets the format string from command flags or context
func getFormatString(cmd *cobra.Command) string {
	formatFlag := cmd.Flag("format")
	if formatFlag != nil && formatFlag.Changed {
		return formatFlag.Value.String()
	}
	format := cli.GetFormat(cmd)
	return string(format)
}

// handleSnapshot saves a snapshot if requested
func handleSnapshot(cmd *cobra.Command, ctx *cli.Context, results []CheckResult, snapshotPath string) error {
	if snapshotPath == emptyValue {
		return nil
	}

	command := buildCommandString(cmd)
	logger := logging.GetLoggerFromProfile(ctx.Profile)
	if err := SaveCheckSnapshot(results, snapshotPath, ctx.ProjectRoot, command, logger); err != nil {
		logging.Fluent(logger).Warn("Failed to save check snapshot").
			WithError(err).
			Log()
	}
	return nil
}

// buildCommandString builds the command string for snapshot metadata
func buildCommandString(cmd *cobra.Command) string {
	command := paths.CLICommandName + " system check"
	if verbose, _ := cmd.Flags().GetBool("verbose"); verbose {
		command += " --verbose"
	}
	formatStr := getFormatString(cmd)
	if formatStr != emptyValue {
		command += " --format " + formatStr
	}
	if tier, _ := cmd.Flags().GetInt("tier"); tier > 0 {
		command += fmt.Sprintf(" --tier %d", tier)
	}
	return command
}

// handleTableFormat handles table format output
func handleTableFormat(cmd *cobra.Command, ctx *cli.Context, results []CheckResult, outputCtx *OutputResultsContext) error {
	if outputCtx.Format == cli.FormatTable || outputCtx.FormatStr == "table" {
		return outputTable(cmd, ctx, results, outputCtx.BufferCount, outputCtx.BufferSummary, outputCtx.StaleCASCleanupResult, outputCtx.RunIssues)
	}
	return nil
}

// handleStreamingFormat handles streaming format output
func handleStreamingFormat(cmd *cobra.Command, ctx *cli.Context, results []CheckResult, outputCtx *OutputResultsContext) error {
	if outputCtx.Handler == nil || !outputCtx.Handler.IsStreaming() {
		return nil
	}

	// Don't create file if there are no results and no buffer info
	// This prevents creating empty files when there's nothing to write
	if len(results) == 0 && outputCtx.BufferCount == 0 && outputCtx.OutputPath != emptyValue {
		// No results and no buffer info - don't create empty file
		return nil
	}

	writer, file, err := setupOutputWriter(cmd, outputCtx.OutputPath)
	if err != nil {
		return err
	}
	if file != nil {
		defer file.Close()
	}

	stdCtx := pkgctx.NewSystemContext()
	resultSlice := convertResultsToInterfaceSlice(results)
	syncInterval := calculateSyncInterval(len(resultSlice))

	if err := streamResults(stdCtx, outputCtx.Handler, resultSlice, writer, file, syncInterval, ctx); err != nil {
		return err
	}

	if err := streamBufferInfo(stdCtx, outputCtx.Handler, outputCtx.BufferCount, outputCtx.BufferSummary, writer); err != nil {
		return err
	}

	return nil
}

// setupOutputWriter sets up the output writer (stdout or file)
func setupOutputWriter(cmd *cobra.Command, outputPath string) (io.Writer, *fileutil.File, error) {
	if outputPath == emptyValue {
		if cmd != nil {
			return cmd.OutOrStdout(), nil, nil
		}
		return os.Stdout, nil, nil
	}

	file, err := fileutil.Create(outputPath)
	if err != nil {
		return nil, nil, errfmt.Newf("failed to create output file").Wrap(err)
	}
	return file, file, nil
}

// convertResultsToInterfaceSlice converts CheckResult slice to []any
func convertResultsToInterfaceSlice(results []CheckResult) []any {
	resultSlice := make([]any, len(results))
	for i := range results {
		resultSlice[i] = results[i]
	}
	return resultSlice
}

// calculateSyncInterval calculates the sync interval based on result count
func calculateSyncInterval(resultCount int) int {
	if resultCount < 1000 {
		return 1
	}
	return 100
}

// streamResults streams results to the writer
func streamResults(
	ctx context.Context,
	handler cli.FormatHandler,
	resultSlice []any,
	writer io.Writer,
	file *fileutil.File,
	syncInterval int,
	cliCtx *cli.Context,
) error {
	for i, result := range resultSlice {
		if err := handler.Stream(ctx, result, writer); err != nil {
			return errfmt.Newf("failed to stream result").Wrap(err)
		}

		if file != nil && (i+1)%syncInterval == 0 {
			if err := file.Sync(); err != nil {
				if cliCtx != nil {
					logger := logging.GetLoggerFromProfile(cliCtx.Profile)
					logging.Fluent(logger).Debug("Failed to sync file after write").
						WithError(err).
						Log()
				}
			}
		}
	}

	if file != nil {
		if err := file.Sync(); err != nil {
			if cliCtx != nil {
				logger := logging.GetLoggerFromProfile(cliCtx.Profile)
				logging.Fluent(logger).Debug("Failed to sync file after all writes").
					WithError(err).
					Log()
			}
		}
	}

	return nil
}

// streamBufferInfo streams buffer info if present
func streamBufferInfo(
	ctx context.Context,
	handler cli.FormatHandler,
	bufferCount int,
	bufferSummary map[string]int,
	writer io.Writer,
) error {
	if bufferCount == 0 {
		return nil
	}

	bufferInfo := map[string]any{
		objects.FieldKeyType:    "buffer_info",
		"audit_events_buffered": bufferCount,
		"audit_events_summary":  bufferSummary,
		"message":               fmt.Sprintf("Note: %d audit event(s) are being buffered and will be written/flushed after this operation completes. Run the command again to see final audit event counts.", bufferCount),
	}
	return handler.Stream(ctx, bufferInfo, writer)
}

// handleNonStreamingFormat handles non-streaming format output
func handleNonStreamingFormat(cmd *cobra.Command, results []CheckResult, outputCtx *OutputResultsContext) error {
	if outputCtx.Handler == nil || outputCtx.Handler.IsStreaming() {
		return nil
	}

	var outputData any
	if outputCtx.Format == cli.FormatJSON || outputCtx.FormatStr == "json" || string(outputCtx.Format) == "json" {
		projectRoot := ""
		if ctx := cli.GetContext(cmd); ctx != nil && ctx.ProjectRoot != emptyValue {
			projectRoot = ctx.ProjectRoot
		}
		projectRoot = ProjectRootOrResolve(projectRoot)
		outputData = PrepareCompactCheckOutputData(cmd, results, outputCtx.BufferCount, outputCtx.BufferSummary, projectRoot)
	} else {
		outputData = prepareOutputData(results, outputCtx.BufferCount, outputCtx.BufferSummary)
	}
	if err := cli.FormatOutput(cmd, outputData); err != nil {
		return errfmt.Newf("failed to format output").Wrap(err)
	}
	return nil
}

// handleLegacyFormat handles legacy format output (fallback)
func handleLegacyFormat(cmd *cobra.Command, ctx *cli.Context, results []CheckResult, outputCtx *OutputResultsContext) error {
	switch {
	case outputCtx.Format == cli.FormatJSONL || outputCtx.FormatStr == "jsonl" || string(outputCtx.Format) == "jsonl":
		return outputJSONL(cmd, results, outputCtx.BufferCount, outputCtx.BufferSummary)
	case outputCtx.Format == cli.FormatJSON || outputCtx.FormatStr == "json" || string(outputCtx.Format) == "json":
		return outputJSON(cmd, results, outputCtx.BufferCount, outputCtx.BufferSummary)
	case outputCtx.FormatStr == "yaml" || string(outputCtx.Format) == "yaml":
		return outputYAML(cmd, results, outputCtx.BufferCount, outputCtx.BufferSummary)
	case outputCtx.FormatStr == "table" || string(outputCtx.Format) == "table":
		return outputTable(cmd, ctx, results, outputCtx.BufferCount, outputCtx.BufferSummary, outputCtx.StaleCASCleanupResult, outputCtx.RunIssues)
	default:
		return outputTable(cmd, ctx, results, outputCtx.BufferCount, outputCtx.BufferSummary, outputCtx.StaleCASCleanupResult, outputCtx.RunIssues)
	}
}
