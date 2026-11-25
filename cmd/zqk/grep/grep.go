package grep

import (
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/search"
	"github.com/zqk-os/zqk/pkg/search/interceptor"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"gopkg.in/yaml.v3"
)

const (
	errQueryRequired = "search query required (e.g. zqk grep 'pattern')"
	fmtTextNormal    = "%s:%d:%d: %s\n"
	fmtTextSymbol    = "%s:%d:%d: [%s] %s%s: %s\n"
)

// NewGrepCmd creates the 'zqk grep' command for native in-process code search.
func NewGrepCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"In-process trigram and polyglot AST code search",
		"Fast in-process pure-Go code search engine with trigram indexing, polyglot AST structural queries (Go, TypeScript/JS, Python, Rust), token-budgeted JSON output, and pilot ROI compliance scorecards.",
		"",
		"Executes sub-15ms code searches without external ripgrep/tgrep binaries.",
		"Supports regular expression, literal text, trigram indexing, and polyglot AST symbol queries (functions, methods, structs, classes, interfaces, traits, enums).",
	).
		AddExample("Literal search", "%s grep 'MaterializedView'").
		AddExample("Case-insensitive search", "%s grep -i 'wal_subscriber'").
		AddExample("Regex search", "%s grep -e 'func.*Start\\('").
		AddExample("Polyglot AST search for structs/classes", "%s grep --ast --kind struct").
		AddExample("Polyglot AST search for methods on a receiver/class", "%s grep --ast --recv Engine").
		AddExample("Token-budgeted JSON output for AI agents", "%s grep 'error' --max-tokens 2000 -f json").
		AddExample("Pilot ROI compliance & token savings scorecard", "%s grep --roi").
		AddExample("Rebuild persistent index cache", "%s grep --reindex")

	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewGrepCommandBuilder(), &cobra.Command{
		Use:     "grep [query] [path]",
		Aliases: []string{"zgrep"},
		Short:   "In-process trigram and AST code search",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGrepExecution(cmd, args)
		},
	})

	helpBuilder.ApplyToCommand(cmd)
	cmd.Flags().Bool("vendor", false, "Include vendor directories in search")
	cmd.Flags().Bool("roi", false, "Display zgrep compliance ratio and estimated token savings ROI scorecard")
	cmd.Flags().Float64("min-compliance", 0, "With --roi, fail closed if zgrep compliance ratio is below this percentage (0-100)")
	cli.ConfigureRequirements(cmd, false, false)
	return cmd
}

func parseGrepOptions(cmd *cobra.Command, args []string) (search.SearchOptions, bool, string, error) {
	var flags clipkg.FlagBag
	ignoreCase := flags.Bool(cmd, "ignore-case")
	useRegex := flags.Bool(cmd, "regex")
	wordRegexp := flags.Bool(cmd, "word-regexp")
	astMode := flags.Bool(cmd, "ast")
	astKind := flags.String(cmd, "kind")
	astReceiver := flags.String(cmd, "recv")
	contextLines := flags.Int(cmd, "context")
	maxCount := flags.Int(cmd, "max-count")
	maxTokens := flags.Int(cmd, "max-tokens")
	rawExts := flags.StringArray(cmd, "ext")
	useIndex := flags.Bool(cmd, "use-index")
	reindex := flags.Bool(cmd, "reindex")
	includeHidden := flags.Bool(cmd, "hidden")
	pathFlag := flags.String(cmd, "path")
	format := flags.String(cmd, "format")
	if err := flags.Err(); err != nil {
		return search.SearchOptions{}, false, "", err
	}

	var fileExts []string
	for _, e := range rawExts {
		for _, part := range strings.Split(e, ",") {
			part = strings.TrimSpace(part)
			if part != "" {
				fileExts = append(fileExts, part)
			}
		}
	}

	query, searchPath := resolveQueryAndPath(args, pathFlag, astMode, astKind, astReceiver, reindex)
	if query == "" && pathFlag == "" && !astMode && astKind == "" && astReceiver == "" && !reindex {
		return search.SearchOptions{}, false, "", fmt.Errorf("%s", paths.RewriteCanonicalCLIInvocations(errQueryRequired))
	}

	mode := search.ModeText
	if astMode || astKind != "" || astReceiver != "" {
		mode = search.ModeAST
	}

	effectiveMaxTokens := maxTokens
	if effectiveMaxTokens <= 0 && (strings.EqualFold(format, "json") || strings.EqualFold(format, "jsonl")) {
		effectiveMaxTokens = search.DefaultMaxTokens
	}

	opts := search.SearchOptions{
		Query:           query,
		Path:            searchPath,
		Mode:            mode,
		CaseInsensitive: ignoreCase,
		Regex:           useRegex,
		WordMatch:       wordRegexp,
		ASTKind:         astKind,
		ASTReceiver:     astReceiver,
		FileExtensions:  fileExts,
		MaxMatches:      maxCount,
		MaxTokens:       effectiveMaxTokens,
		ContextLines:    contextLines,
		IncludeHidden:   includeHidden,
		UseIndex:        useIndex,
	}

	return opts, reindex, format, nil
}

func resolveQueryAndPath(args []string, pathFlag string, astMode bool, astKind, astReceiver string, reindex bool) (string, string) {
	searchPath := defaultSearchPath(pathFlag)
	if len(args) == 0 {
		return "", searchPath
	}

	if len(args) == 1 {
		if isDirOrPathTarget(args[0], astMode, astKind, astReceiver, reindex) && pathFlag == "" {
			return "", args[0]
		}
		return args[0], searchPath
	}

	if pathFlag == "" {
		searchPath = args[1]
	}
	return args[0], searchPath
}

func defaultSearchPath(pathFlag string) string {
	if pathFlag != "" {
		return pathFlag
	}
	return "."
}

func isDirOrPathTarget(target string, astMode bool, astKind, astReceiver string, reindex bool) bool {
	isStructural := astMode || astKind != "" || astReceiver != "" || reindex
	isDir := fileutil.Exists(target) && !fileutil.IsRegularFile(target)
	hasSlash := strings.Contains(target, "/")
	return (isStructural || isDir) && (fileutil.Exists(target) || hasSlash)
}

func runGrepExecution(cmd *cobra.Command, args []string) error {
	var roiFlags clipkg.FlagBag
	roiMode := roiFlags.Bool(cmd, "roi")
	if err := roiFlags.Err(); err != nil {
		return err
	}
	if roiMode {
		format := roiFlags.String(cmd, "format")
		pathFlag := roiFlags.String(cmd, "path")
		if err := roiFlags.Err(); err != nil {
			return err
		}
		minCompliance, err := cmd.Flags().GetFloat64("min-compliance")
		if err != nil {
			return err
		}
		if minCompliance < 0 || minCompliance > 100 {
			return fmt.Errorf("--min-compliance must be between 0 and 100 (got %.2f)", minCompliance)
		}
		projectRoot := resolveROIProjectRoot(pathFlag, args)
		summary, err := interceptor.SummarizeCompliance(projectRoot)
		if err != nil {
			return fmt.Errorf("failed computing zgrep ROI compliance summary: %w", err)
		}
		if err := renderComplianceROI(cmd.OutOrStdout(), format, projectRoot, summary); err != nil {
			return err
		}
		if minCompliance > 0 && summary.TotalSearchInvocations > 0 && summary.ComplianceRatioPct < minCompliance {
			return fmt.Errorf("zgrep compliance %.1f%% is below required --min-compliance threshold %.1f%%", summary.ComplianceRatioPct, minCompliance)
		}
		return nil
	}

	opts, reindex, format, err := parseGrepOptions(cmd, args)
	if err != nil {
		return err
	}

	projectRoot := paths.ResolveProjectRoot(".")
	if projectRoot == "" {
		projectRoot = "."
	}
	engineRoot := projectRoot
	if opts.Path != "" && filepath.IsAbs(opts.Path) {
		engineRoot = opts.Path
	}
	engine := search.NewEngine(engineRoot)

	if reindex {
		if idxErr := engine.BuildTrigramIndex(opts); idxErr != nil {
			return fmt.Errorf("failed rebuilding trigram index: %w", idxErr)
		}
		fmt.Fprintln(cmd.OutOrStdout(), "✓ Trigram index successfully rebuilt and cached.")
		if opts.Query == "" && opts.Mode != search.ModeAST && opts.ASTKind == "" && opts.ASTReceiver == "" {
			return nil
		}
	}

	result, err := engine.Search(cmd.Context(), opts)
	if err != nil {
		return err
	}

	return renderGrepResult(cmd.OutOrStdout(), format, result)
}

func renderGrepResult(out io.Writer, format string, result *search.SearchResult) error {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "json":
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(result)

	case "yaml":
		enc := yaml.NewEncoder(out)
		return enc.Encode(result)

	default:
		return renderTextMatches(out, result)
	}
}

func renderTextMatches(out io.Writer, result *search.SearchResult) error {
	for _, m := range result.Matches {
		for _, before := range m.ContextBefore {
			fmt.Fprintf(out, "%s- %s\n", m.File, before)
		}

		if m.SymbolKind != "" {
			recvPart := ""
			if m.Receiver != "" {
				recvPart = fmt.Sprintf("(%s) ", m.Receiver)
			}
			fmt.Fprintf(out, fmtTextSymbol,
				m.File, m.Line, m.Column, m.SymbolKind, recvPart, m.SymbolName, strings.TrimSpace(m.LineContent))
		} else {
			fmt.Fprintf(out, fmtTextNormal, m.File, m.Line, m.Column, m.LineContent)
		}

		for _, after := range m.ContextAfter {
			fmt.Fprintf(out, "%s+ %s\n", m.File, after)
		}
	}

	return nil
}

func resolveROIProjectRoot(pathFlag string, args []string) string {
	if strings.TrimSpace(pathFlag) != "" {
		return strings.TrimSpace(pathFlag)
	}
	if len(args) > 0 && strings.TrimSpace(args[0]) != "" {
		return strings.TrimSpace(args[0])
	}
	if root := paths.ResolveProjectRoot("."); root != "" {
		return root
	}
	return "."
}

func renderComplianceROI(out io.Writer, format, projectRoot string, summary interceptor.ComplianceSummary) error {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "json":
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(summary)
	case "yaml":
		enc := yaml.NewEncoder(out)
		return enc.Encode(summary)
	case "markdown", "md":
		fmt.Fprintf(out, "## zgrep Pilot ROI Scorecard\n\n")
		fmt.Fprintf(out, "- **Repository:** `%s`\n", projectRoot)
		fmt.Fprintf(out, "- **Telemetry Source:** `%s`\n\n", interceptor.ComplianceLogPath(projectRoot))
		fmt.Fprintf(out, "| Metric | Value |\n")
		fmt.Fprintf(out, "|---|---:|\n")
		fmt.Fprintf(out, "| Total Search Invocations | %d |\n", summary.TotalSearchInvocations)
		fmt.Fprintf(out, "| zgrep Compliant Invocations | %d |\n", summary.ZgrepCompliantCount)
		fmt.Fprintf(out, "| Raw Shell Searches Intercepted | %d |\n", summary.RawSearchNonCompliantCount)
		fmt.Fprintf(out, "| **zgrep Compliance Ratio** | **%.1f%%** |\n", summary.ComplianceRatioPct)
		fmt.Fprintf(out, "| **Estimated Context Tokens Saved** | **%d** |\n", summary.EstimatedTokensSaved)
		return nil
	default:
		fmt.Fprintf(out, "zgrep Pilot ROI Scorecard (%s)\n", projectRoot)
		fmt.Fprintf(out, "------------------------------------------------------------\n")
		fmt.Fprintf(out, "Total Search Invocations:      %d\n", summary.TotalSearchInvocations)
		fmt.Fprintf(out, "zgrep Compliant Invocations:   %d\n", summary.ZgrepCompliantCount)
		fmt.Fprintf(out, "Raw Shell Search Intercepted:  %d\n", summary.RawSearchNonCompliantCount)
		fmt.Fprintf(out, "zgrep Compliance Ratio:        %.1f%%\n", summary.ComplianceRatioPct)
		fmt.Fprintf(out, "Estimated Tokens Saved:        %d\n", summary.EstimatedTokensSaved)
		fmt.Fprintf(out, "Telemetry Log:                 %s\n", interceptor.ComplianceLogPath(projectRoot))
		return nil
	}
}
