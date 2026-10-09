package grep

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/search"
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
		"In-process trigram and AST code search",
		"Fast in-process pure-Go code search engine with trigram indexing, Go AST structural queries, and token-budgeted JSON output.",
		"",
		"Executes sub-15ms code searches without external ripgrep/tgrep binaries.",
		"Supports regular expression, literal text, trigram indexing, and Go AST symbol queries (functions, methods, structs, interfaces).",
	).
		AddExample("Literal search", "%s grep 'MaterializedView'").
		AddExample("Case-insensitive search", "%s grep -i 'wal_subscriber'").
		AddExample("Regex search", "%s grep -e 'func.*Start\\('").
		AddExample("AST search for all structs", "%s grep --ast --kind struct").
		AddExample("AST search for methods on a receiver", "%s grep --ast --recv Engine").
		AddExample("Token-budgeted JSON output for AI agents", "%s grep 'error' --max-tokens 2000 -f json").
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
	cli.RequireSession(cmd, false)
	cli.RequireStorage(cmd, false)
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
	opts, reindex, format, err := parseGrepOptions(cmd, args)
	if err != nil {
		return err
	}

	engineRoot := "."
	if opts.Path != "" && opts.Path != "." {
		engineRoot = opts.Path
	} else {
		engineRoot = paths.ResolveProjectRoot(".")
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
