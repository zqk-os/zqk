package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/zqk-os/zqk/internal/cli/errorsuggest"
	"github.com/zqk-os/zqk/internal/cli/flagutil"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/outputtypes"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

// contextRegistry stores context for commands using command pointer as key
var (
	contextRegistry = make(map[*cobra.Command]*Context)
	contextMu       sync.RWMutex
)

// CommonFlagNames holds standard flag names used across commands
const (
	FlagFormat                          = "format"
	FlagOutput                          = "output"
	FlagVerbose                         = "verbose"
	FlagQuiet                           = "quiet"
	FlagForce                           = "force"
	FlagDryRun                          = "dry-run"
	cliOutputFilePerm fileutil.FileMode = paths.FilePerm600
)

var supportedOutputFormats = []OutputFormat{
	FormatTable,
	FormatJSON,
	FormatJSONL,
	FormatYAML,
	FormatCSV,
	FormatJSONRPC,
	FormatStream,
	FormatAgentPrompt,
	FormatSemanticLink,
}

func supportedOutputFormatsHelpText() string {
	values := make([]string, 0, len(supportedOutputFormats))
	for _, f := range supportedOutputFormats {
		values = append(values, string(f))
	}
	return strings.Join(values, ", ")
}

// OutputFormat represents supported output formats
type OutputFormat string

const (
	FormatTable        OutputFormat = OutputFormat(outputtypes.IDTable)
	FormatJSON         OutputFormat = OutputFormat(outputtypes.IDJSON)
	FormatJSONL        OutputFormat = OutputFormat(outputtypes.IDJSONL) // JSON Lines (one JSON object per line)
	FormatYAML         OutputFormat = OutputFormat(outputtypes.IDYAML)
	FormatCSV          OutputFormat = OutputFormat(outputtypes.IDCSV)
	FormatJSONRPC      OutputFormat = "json-rpc"     // Stream JSON-RPC notifications (events)
	FormatStream       OutputFormat = "stream"       // Alias for json-rpc
	FormatAgentPrompt  OutputFormat = "agent-prompt" // Scheduler convergence measure --format agent-prompt: markdown for agent chat
	FormatSemanticLink OutputFormat = "semantic-link"
)

// AddCommonFlags adds common flags to a command.
// Idempotent: skips any flag already defined on the command (avoids "flag redefined" when
// the same command is initialized multiple times, e.g. in parallel tests or re-use).
func AddCommonFlags(cmd *cobra.Command) {
	fs := cmd.Flags()
	if fs.Lookup(FlagFormat) == nil {
		fs.StringP(FlagFormat, "f", string(FormatTable), fmt.Sprintf("Output format (%s)", supportedOutputFormatsHelpText()))
	}
	if fs.Lookup(FlagOutput) == nil {
		fs.StringP(FlagOutput, "o", "", "Write output to file")
	}
	if fs.Lookup(FlagVerbose) == nil {
		fs.BoolP(FlagVerbose, "v", false, "Verbose output")
	}
	if fs.Lookup(FlagQuiet) == nil {
		fs.BoolP(FlagQuiet, "q", false, "Suppress non-essential output")
	}
	if fs.Lookup(FlagTimeout) == nil {
		fs.Duration(FlagTimeout, 0, "Timeout for command execution (0 = auto-calculate)")
	}
	if fs.Lookup(FlagColumns) == nil {
		fs.String(FlagColumns, "", "Override column widths for table output (format: field:width)")
	}
	if fs.Lookup(FlagIgnoreSchedulerDown) == nil {
		fs.Bool(FlagIgnoreSchedulerDown, false, "Ignore scheduler down warning/lockdown")
	}
}

// Common flag names used for exclusion (must match AddCommonFlags).
const (
	FlagTimeout             = "timeout"
	FlagColumns             = "columns"
	FlagIgnoreSchedulerDown = "ignore-scheduler-down"
)

// AddCommonFlagsExcluding adds common flags to a command, skipping any whose name is in exclude.
// Idempotent. Use this when a command spec lists exclude_flags so those flags are not added
// and do not appear in Cobra's help.
func AddCommonFlagsExcluding(cmd *cobra.Command, exclude []string) {
	set := make(map[string]bool, len(exclude))
	for _, name := range exclude {
		set[name] = true
	}
	fs := cmd.Flags()
	if !set[FlagFormat] && fs.Lookup(FlagFormat) == nil {
		fs.StringP(FlagFormat, "f", string(FormatTable), fmt.Sprintf("Output format (%s)", supportedOutputFormatsHelpText()))
	}
	if !set[FlagOutput] && fs.Lookup(FlagOutput) == nil {
		fs.StringP(FlagOutput, "o", "", "Write output to file")
	}
	if !set[FlagVerbose] && fs.Lookup(FlagVerbose) == nil {
		fs.BoolP(FlagVerbose, "v", false, "Verbose output")
	}
	if !set[FlagQuiet] && fs.Lookup(FlagQuiet) == nil {
		fs.BoolP(FlagQuiet, "q", false, "Suppress non-essential output")
	}
	if !set[FlagTimeout] && fs.Lookup(FlagTimeout) == nil {
		fs.Duration(FlagTimeout, 0, "Timeout for command execution (0 = auto-calculate)")
	}
	if !set[FlagColumns] && fs.Lookup(FlagColumns) == nil {
		fs.String(FlagColumns, "", "Override column widths for table output (format: field:width)")
	}
}

// AddValidationFlags adds validation-related flags
func AddValidationFlags(cmd *cobra.Command) {
	cmd.Flags().Bool(FlagDryRun, false, "Validate without executing")
	cmd.Flags().BoolP(FlagForce, "F", false, "Skip confirmation")
}

// GetFlagFromChain finds a flag by name by walking up the command chain, checking both
// Flags() and PersistentFlags() at each level. This ensures root persistent flags (e.g.
// --format, --timeout) are found when invoked from nested subcommands (e.g. object get).
func GetFlagFromChain(cmd *cobra.Command, name string) *pflag.Flag {
	for c := cmd; c != nil; c = c.Parent() {
		if f := c.Flags().Lookup(name); f != nil {
			return f
		}
		if f := c.PersistentFlags().Lookup(name); f != nil {
			return f
		}
	}
	return nil
}

// getFlagFromChainChanged returns the first flag in the chain (from cmd up to root) that
// has Changed set. Use this when the same flag exists on multiple levels (e.g. root
// persistent and child local) so the user's explicit value is used (Cobra sets only one).
func getFlagFromChainChanged(cmd *cobra.Command, name string) *pflag.Flag {
	for c := cmd; c != nil; c = c.Parent() {
		if f := c.Flags().Lookup(name); f != nil && f.Changed {
			return f
		}
		if f := c.PersistentFlags().Lookup(name); f != nil && f.Changed {
			return f
		}
	}
	return nil
}

// FormatFlagExplicitlySet reports whether --format was set on the command chain (any level).
func FormatFlagExplicitlySet(cmd *cobra.Command) bool {
	return getFlagFromChainChanged(cmd, FlagFormat) != nil
}

// GetFormat returns the output format from command flags or context
// Priority: explicit --format flag > context format (from profile/config) > default (table)
// The --format flag overrides context/config settings when explicitly set
func GetFormat(cmd *cobra.Command) OutputFormat {
	// Prefer a format flag that was explicitly set (handles root persistent vs child local)
	formatFlag := getFlagFromChainChanged(cmd, FlagFormat)
	if formatFlag == nil {
		formatFlag = GetFlagFromChain(cmd, FlagFormat)
	}

	if formatFlag != nil {
		formatValue := formatFlag.Value.String()
		if formatValue != emptyValue {
			// Use flag value when: (1) user explicitly set it (Changed), or
			// (2) flag default is "" (root's persistent format) and value is non-empty.
			// (2) fixes --format yaml being ignored when the flag lives on root and the
			// current command is a child, where Changed may not be set on the child's view.
			if formatFlag.Changed || formatFlag.DefValue == emptyValue {
				return OutputFormat(formatValue)
			}
		}
	}

	// Get context to check if it has an explicit format (from profile or config)
	ctx := GetContext(cmd)

	// Only use context format if a PROFILE is explicitly set
	// Project config format should not override default table unless a profile is set
	// This ensures that project config doesn't silently change default behavior
	if ctx != nil && ctx.Format != emptyValue && ctx.Profile != emptyValue {
		// Profile is set - use profile's format
		return ctx.Format
	}

	// If no profile is set, ignore config format and use default table

	// Fallback to default
	return FormatTable
}

// GetContext retrieves the context from the registry
// Context is set during PersistentPreRunE in root.go
func GetContext(cmd *cobra.Command) *Context {
	contextMu.RLock()
	defer contextMu.RUnlock()

	// Walk up the command tree to find context (parent commands may have it)
	for c := cmd; c != nil; c = c.Parent() {
		if ctx, ok := contextRegistry[c]; ok {
			return ctx
		}
	}

	// Fallback: if no context found, try to load it (shouldn't happen in normal flow)
	// Use CliInitializationContext - no more one-off conditional checks
	initCtx := pkgctx.NewCliInitializationContext(ResolveProjectRoot, ".")
	ctx, err := GetContextFromCommand(cmd, initCtx)
	if err != nil {
		return nil
	}
	return ctx
}

// SetContext stores context in the registry for the command
// This is called during PersistentPreRunE in root.go
func SetContext(cmd *cobra.Command, ctx *Context) {
	contextMu.Lock()
	defer contextMu.Unlock()
	contextRegistry[cmd] = ctx
}

// GetOutputPath returns the output file path from context or command flags
func GetOutputPath(cmd *cobra.Command) string {
	ctx := GetContext(cmd)
	if ctx != nil {
		// Context doesn't have output path yet, fall back to flag
		// TODO: Add output path to context
	}
	f := GetFlagFromChain(cmd, FlagOutput)
	if f == nil {
		return ""
	}
	return f.Value.String()
}

// IsVerbose returns whether verbose mode is enabled from context
func IsVerbose(cmd *cobra.Command) bool {
	ctx := GetContext(cmd)
	if ctx != nil {
		return ctx.Verbose
	}
	return isTrueFlagSet(cmd, FlagVerbose)
}

// IsQuiet returns whether quiet mode is enabled from context
func IsQuiet(cmd *cobra.Command) bool {
	ctx := GetContext(cmd)
	if ctx != nil {
		return ctx.Quiet
	}
	return isTrueFlagSet(cmd, FlagQuiet)
}

// IsDryRun returns whether dry-run mode is enabled
func IsDryRun(cmd *cobra.Command) bool {
	return isTrueFlagSet(cmd, FlagDryRun)
}

// IsForce returns whether force mode is enabled
func IsForce(cmd *cobra.Command) bool {
	return isTrueFlagSet(cmd, FlagForce)
}

func isTrueFlagSet(cmd *cobra.Command, name string) bool {
	return flagutil.IsTrue(GetFlagFromChain(cmd, name))
}

// GetTimeout returns the timeout duration from command flags (root persistent or local).
// Returns 0 if not set or on error.
func GetTimeout(cmd *cobra.Command) time.Duration {
	f := GetFlagFromChain(cmd, FlagTimeout)
	if f == nil {
		return 0
	}
	d, err := time.ParseDuration(f.Value.String())
	if err != nil || d <= 0 {
		return 0
	}
	return d
}

// CommandContextOr returns cmd.Context() when cmd is non-nil and that context is non-nil;
// otherwise it returns fallback. If fallback is nil, it returns pkgctx.NewSystemContext().
//
// Use this (or [CommandOutputWriter]) before logging.GetCommandOutputWriter for POL-CODE-007
// routing, instead of repeating "outCtx := cmd.Context(); if outCtx == nil { ... }" at every call site.
// Tests inject output capture via pkgctx.WithCommandOutputWriter on the same context that Execute sets on the command.
func CommandContextOr(cmd *cobra.Command, fallback context.Context) context.Context {
	if cmd != nil {
		if c := cmd.Context(); c != nil {
			return c
		}
	}
	if fallback != nil {
		return fallback
	}
	return pkgctx.NewSystemContext()
}

// CommandOutputWriter returns logging.GetCommandOutputWriter(CommandContextOr(cmd, fallback)).
// Prefer this over duplicating nil-check logic when streaming fmt.Fprintf to the policy-compliant writer.
func CommandOutputWriter(cmd *cobra.Command, fallback context.Context) io.Writer {
	return logging.GetCommandOutputWriter(CommandContextOr(cmd, fallback))
}

// WriteOutput writes output to the appropriate destination based on flags and context.
// POL-CODE-007: All stdio output is routed through the logging package's command output
// writer (logging.GetCommandOutputWriter) so the policy is followed and MCP protocol
// is protected (suppress when serving, stdout for result in MCP subprocess so parent can capture it).
func WriteOutput(cmd *cobra.Command, data []byte) error {
	outputPath := GetOutputPath(cmd)
	// Treat "-" like common Unix tools: write to the command output writer, not a file named "-".
	if outputPath != emptyValue && outputPath != "-" {
		return fileutil.WriteFile(outputPath, data, cliOutputFilePerm) //nolint:gosec // Output files - 0600 is acceptable
	}

	ctx := CommandContextOr(cmd, nil)
	writer := logging.GetCommandOutputWriter(ctx)
	_, err := writer.Write(data)
	return err
}

// FormatOutput formats and writes command results using the registered format handlers.
// It is the canonical path for structured payloads (maps, slices, objects): one implementation
// honors --format (json, yaml, table, etc.) and logging/MCP output routing via WriteOutput.
// Prefer FormatOutput over pairing json.MarshalIndent or yaml.Marshal with WriteOutput, which
// duplicates logic and previously allowed YAML to diverge from the shared handler. Commands
// that intentionally emit bespoke plain text or fixed-column tables may build []byte and call
// WriteOutput directly when the result is not a generic structured document.
func FormatOutput(cmd *cobra.Command, data any) error {
	return FormatOutputWithPermissionChecker(cmd, data, nil)
}

// FormatOutputAs formats data with an explicit output format, ignoring --format on cmd.
// Use when the caller's API fixes the wire encoding (e.g. outputJSON vs outputYAML) while still
// using shared handlers and WriteOutput (MCP routing, unit tests with bare cobra.Command).
func FormatOutputAs(cmd *cobra.Command, format OutputFormat, data any) error {
	return formatOutputWithPermissionChecker(cmd, format, data, nil)
}

// FormatOutputWithPermissionChecker formats and writes output with permission checking
// The permission checker is called before data access to short-circuit unauthorized access
// This is similar to how dry-run validates before execution
func FormatOutputWithPermissionChecker(cmd *cobra.Command, data any, permissionChecker PermissionChecker) error {
	return formatOutputWithPermissionChecker(cmd, GetFormat(cmd), data, permissionChecker)
}

func formatOutputWithPermissionChecker(cmd *cobra.Command, format OutputFormat, data any, permissionChecker PermissionChecker) error {
	handler := GetFormatHandler(format)
	if handler == nil {
		// Fallback to JSON if handler not found
		handler = GetFormatHandler(FormatJSON)
		if handler == nil {
			// Last resort: marshal as JSON
			jsonData, err := json.MarshalIndent(data, "", "  ")
			if err != nil {
				return errfmt.Newf("failed to marshal output").Wrap(err)
			}
			jsonData = append(jsonData, '\n')
			return WriteOutput(cmd, jsonData)
		}
	}

	// Wrap handler with permission checker if provided
	if permissionChecker != nil {
		handler = NewFormatHandlerWithPermissions(handler, permissionChecker)
	}

	// Validate data before formatting
	if err := handler.Validate(data); err != nil {
		return errfmt.Newf("data validation failed").Wrap(err)
	}

	// Check if streaming format
	if handler.IsStreaming() {
		// For streaming formats, use Stream method
		ctx := cmd.Context()
		if ctx == nil {
			ctx = pkgctx.NewSystemContext()
		}

		// Get writer (stdout or stderr based on MCP mode)
		var writer io.Writer = os.Stdout
		if zqkenv.MCPAccountID().Get() != emptyValue {
			writer = os.Stderr
		}

		return handler.Stream(ctx, data, writer)
	}

	// Single response format
	formatted, err := handler.Format(data)
	if err != nil {
		return errfmt.Newf("failed to format output").Wrap(err)
	}

	return WriteOutput(cmd, formatted)
}

// PermissionChecker interface for permission validation before data access
// This allows format handlers to check permissions early (like dry-run)
// This short-circuits data access if permissions are insufficient
type PermissionChecker interface {
	// CheckPermission checks if the security context has permission for the operation
	// Returns (allowed, reason) - reason is empty if allowed
	CheckPermission(ctx context.Context, operation string, resource string) (bool, string)

	// CheckFormatPermission checks if format is allowed for the security context
	// Some formats (like json-rpc streaming) may require special permissions
	CheckFormatPermission(ctx context.Context, format OutputFormat) (bool, string)

	// CheckDataAccess checks if the security context can access the data
	// This is called before formatting to short-circuit unauthorized access
	CheckDataAccess(ctx context.Context, data any) (bool, string)
}

// HandleError handles errors consistently across commands.
// Uses the error-suggestion service (BLI-659) when available to add actionable hints
// based on verbosity and context profile (experience level).
func HandleError(cmd *cobra.Command, err error) error {
	if err == nil {
		return nil
	}
	return EnhanceError(cmd, err)
}

// EnhanceError returns an error with an actionable suggestion when possible.
// Use this when returning errors from command RunE so users see consistent,
// experience-appropriate hints (e.g. "Check ID or path; use list to see available items.").
// Respects --verbose and context profile (ai-agent/debug = terse, human = standard).
func EnhanceError(cmd *cobra.Command, err error) error {
	if err == nil {
		return nil
	}
	opts := errorsuggest.Options{
		Verbose:         IsVerbose(cmd),
		ExperienceLevel: errorsuggest.ExperienceFromProfile(GetProfile(cmd)),
		CommandPath:     cmd.CommandPath(),
	}
	s := errorsuggest.Suggest(err, opts)
	if s.Hint == emptyValue || strings.Contains(err.Error(), s.Hint) {
		return errfmt.Errorf("%w", err)
	}
	// Return error that includes suggestion (single line for scripting, two-line for readability)
	if opts.Verbose {
		return errfmt.Errorf("%w\n  %s", err, s.Hint)
	}
	// If not verbose, we still want to wrap so errors.As works, but we also want the simplified message
	// For now, we will just use %w so the type is preserved, even if the text might be slightly different than s.Message.
	return errfmt.Errorf("%w\n  %s", err, s.Hint)
}

// ApplyFlagErrorSuggestions wires cobra FlagErrorFunc on cmd and all subcommands
// so unknown-flag errors get the same actionable hints as EnhanceError (e.g. --field → --fields).
func ApplyFlagErrorSuggestions(cmd *cobra.Command) {
	if cmd == nil {
		return
	}
	cmd.SetFlagErrorFunc(func(c *cobra.Command, err error) error {
		return EnhanceError(c, err)
	})
	for _, sub := range cmd.Commands() {
		ApplyFlagErrorSuggestions(sub)
	}
}

// GetProfile returns the context profile string for the command (e.g. "human", "ai-agent").
func GetProfile(cmd *cobra.Command) string {
	ctx := GetContext(cmd)
	if ctx != nil && ctx.Profile != emptyValue {
		return ctx.Profile
	}
	return ""
}

// ValidateFormat validates the output format
func ValidateFormat(format OutputFormat) error {
	if slices.Contains(supportedOutputFormats, format) {
		return nil
	}

	valid := make([]string, 0, len(supportedOutputFormats))
	for _, f := range supportedOutputFormats {
		valid = append(valid, string(f))
	}
	return errfmt.Errorf("invalid format: %s (valid: %s)", format, strings.Join(valid, ", "))
}

// CreateContextWithLoggingProfile creates a context with LoggingContext embedded from profile string
// This ensures coordinator logging events respect --context profile settings
// Uses constants from pkg/context instead of hardcoded strings
func CreateContextWithLoggingProfile(ctx context.Context, profile string) context.Context {
	if ctx == nil {
		ctx = pkgctx.NewSystemContext()
	}

	if profile == emptyValue {
		profile = string(pkgctx.ProfileHuman) // Default
	}

	// Convert profile string to LoggingProfile enum using constants
	var loggingCtx *pkgctx.LoggingContext
	switch profile {
	case string(pkgctx.ProfileMCP):
		loggingCtx = pkgctx.NewLoggingContext(pkgctx.ProfileMCP)
	case string(pkgctx.ProfileSystem):
		loggingCtx = pkgctx.NewSystemLoggingContext()
	case string(pkgctx.ProfileAIAgent):
		loggingCtx = pkgctx.NewLoggingContext(pkgctx.ProfileAIAgent)
	case string(pkgctx.ProfileDebug):
		loggingCtx = pkgctx.NewLoggingContext(pkgctx.ProfileDebug)
	case string(pkgctx.ProfileHuman), "":
		loggingCtx = pkgctx.NewHumanLoggingContext()
	default:
		loggingCtx = pkgctx.NewHumanLoggingContext()
	}

	return pkgctx.WithLoggingContext(ctx, loggingCtx)
}

// ReadRequiredFileFlag reads the content of a file specified by a flag.
// It returns an error if the flag is missing, empty, or the file cannot be read.
func ReadRequiredFileFlag(cmd *cobra.Command, flagName string) ([]byte, error) {
	filePath, _ := cmd.Flags().GetString(flagName)
	if filePath == "" {
		return nil, errfmt.Errorf("--%s is required", flagName)
	}
	data, err := fileutil.ReadFile(filePath)
	if err != nil {
		return nil, errfmt.Errorf("failed to read file %s: %w", filePath, err)
	}
	return data, nil
}

func WriteOutputStream(cmd *cobra.Command, writeFunc func(io.Writer) error) error {
	outputPath := GetOutputPath(cmd)
	if outputPath != emptyValue && outputPath != "-" {
		f, err := os.Create(filepath.Clean(outputPath)) //nolint:gosec // user-specified output path flag
		if err != nil {
			return err
		}
		defer f.Close()
		return writeFunc(f)
	}
	ctx := CommandContextOr(cmd, nil)
	return writeFunc(logging.GetCommandOutputWriter(ctx))
}
