package callback

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/internal/cli"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/outputtypes"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

const emptyValue = ""

// NewCallbackCmd creates the callback command
func NewCallbackCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Handle scheduler job callbacks and notifications",
		"Handle scheduler job callbacks and notifications.",
		"",
		"This command receives callback events from scheduler jobs and routes them",
		"to designated log files based on the current account or specified path.",
	).
		ExcludeCommonFlags()

	callbackCmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewCallbackCommandBuilder(

	// Apply help builder to command
	), &cobra.Command{
		Use: "callback",
	})

	helpBuilder.ApplyToCommand(callbackCmd)

	callbackCmd.AddCommand(NewNotifyCmd())

	return callbackCmd
}

// NewNotifyCmd creates the notify command for receiving callback events
func NewNotifyCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Receive and log scheduler job callback events",
		"Receive scheduler job callback events and write them to a log file.",
		"",
		"This command reads JSON payload from stdin (provided by scheduler callback system)",
		"and writes formatted output to a log file. The log file location can be:",
		"- Account-based: Automatically determined from current account (default)",
		"- Custom: Specified via --log-file flag",
		"",
		"The command is designed to be used as a callback handler:",
		"  %s scheduler submit \"command\" --callback-completion \"%s callback notify\"",
		"",
		"Output formats:",
		"  - text (default): Human-readable formatted logs",
		"  - jsonl: JSONL format (newline-delimited JSON) - ideal for log aggregation",
		"  - json: Pretty-printed JSON format",
	).
		AddExample("Use account-based log file with default text format", "%s callback notify < callback-payload.json").
		AddExample("Use JSONL format for structured logging", "%s callback notify --format jsonl --log-file event-log.json < callback-payload.json").
		AddExample("Use custom log file with text format", "%s callback notify --log-file /tmp/job-results.log < callback-payload.json").
		AddExample("Use as callback in scheduler submit with JSONL format", "%s scheduler submit \"echo test\" --callback-completion \"%s callback notify --format jsonl\"").
		ExcludeCommonFlags()

	notifyCmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewCallbackNotifyCommandBuilder(), &cobra.Command{
		Use: "notify",
	})
	cli.BindAsyncProgress(notifyCmd, func(cmd *cobra.Command, args []string) error {
		ctx := cli.GetContext(cmd)
		if ctx == nil {
			return errfmt.Errorf("failed to get context")
		}
		return handleCallback(ctx, cmd)
	})

	// Apply help builder to command
	helpBuilder.ApplyToCommand(notifyCmd)

	notifyCmd.Flags().StringP("log-file", "l", "", fmt.Sprintf("Log file path (default: system.log in %s/%s/)", paths.ProjectDataDir, paths.CallbackDir))
	notifyCmd.Flags().BoolP("append", "a", true, "Append to log file (default: true)")
	notifyCmd.Flags().Int("queue-size", 0, "Queue size for buffering (0 = disabled, no queue)")
	notifyCmd.Flags().String("sort", callbackSortTimestamp, "Sorting strategy: timestamp (oldest first) or priority (highest first)")

	cli.AddCommonFlags(notifyCmd)
	return notifyCmd
}

func handleCallback(cliCtx *cli.Context, cmd *cobra.Command) error {
	projectRoot := cliCtx.ProjectRoot
	if projectRoot == emptyValue {
		projectRoot = cli.ResolveProjectRoot(".")
		if projectRoot == emptyValue {
			return errfmt.Errorf("project root not found")
		}
	}

	logFile, _ := cmd.Flags().GetString("log-file")
	appendMode, _ := cmd.Flags().GetBool("append")
	queueSize, _ := cmd.Flags().GetInt("queue-size")
	sortStrategy, _ := cmd.Flags().GetString("sort")

	// Determine log file location
	if logFile == emptyValue {
		// Use system account as default (standard pattern in CLI commands)
		// Users can specify custom log file via --log-file if needed
		accountID := callbackSystemAccountID

		// Create callback logs directory
		logsDir := filepath.Join(projectRoot, paths.ProjectDataDir, paths.CallbackDir)
		if err := fileutil.MkdirAll(logsDir, paths.DirPerm755); err != nil {
			return errfmt.Newf("failed to create logs directory").Wrap(err)
		}

		logFile = filepath.Join(logsDir, fmt.Sprintf("%s.log", accountID))
	}

	// Read JSON payload from stdin (or context override in tests)
	in := io.Reader(os.Stdin)
	if cmd != nil {
		if r, ok := pkgctx.GetStdinReaderFromContext(cmd.Context()); ok && r != nil {
			in = r
		}
	}
	payloadBytes, err := io.ReadAll(in)
	if err != nil {
		return errfmt.Newf("failed to read stdin").Wrap(err)
	}

	var payload map[string]any
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		return errfmt.Newf("failed to parse JSON payload").Wrap(err)
	}

	// Determine sorter
	var sorter Sorter
	switch sortStrategy {
	case callbackSortPriority:
		sorter = &PrioritySorter{}
	case callbackSortTimestamp, "":
		sorter = &TimestampSorter{}
	default:
		return errfmt.Errorf("invalid sort strategy: %s (must be '%s' or '%s')", sortStrategy, callbackSortTimestamp, callbackSortPriority)
	}

	// Get output format from CLI context (defaults to table/text)
	outputFormat := cli.GetFormat(cmd)
	// Map CLI format to callback format (table -> text for logs)
	var format string
	switch outputFormat {
	case cli.FormatJSONL:
		format = outputtypes.IDJSONL
	case cli.FormatJSON:
		format = outputtypes.IDJSON
	case cli.FormatTable:
		format = outputtypes.IDText
	default:
		format = outputtypes.IDText // Default to text for logs
	}

	// Get processor
	processor := GetProcessor()

	// Initialize processor if queue is enabled
	if queueSize > 0 {
		if err := processor.Initialize(projectRoot, logFile, appendMode, queueSize, sorter, format); err != nil {
			return errfmt.Newf("failed to initialize processor").Wrap(err)
		}

		// Enqueue the callback
		if err := processor.Enqueue(payload); err != nil {
			return errfmt.Newf("failed to enqueue callback").Wrap(err)
		}

		// Queue processing happens in background
		return nil
	}

	// No queue - process directly
	return processor.ProcessDirect(projectRoot, logFile, appendMode, payload, format)
}
