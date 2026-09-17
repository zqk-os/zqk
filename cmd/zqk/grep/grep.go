package grep

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/lanceman/zqk/internal/cli"
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/lanceman/zqk/pkg/search"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// NewGrepCmd creates the 'zqk grep' command for native in-process code search.
func NewGrepCmd() *cobra.Command {
	var (
		pathFlag      string
		ignoreCase    bool
		useRegex      bool
		wordRegexp    bool
		astMode       bool
		astKind       string
		astReceiver   string
		contextLines  int
		maxCount      int
		maxTokens     int
		fileExts      []string
		useIndex      bool
		includeHidden bool
		reindex       bool
		format        string
	)

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

	cmd := &cobra.Command{
		Use:     "grep [query] [path]",
		Aliases: []string{"zgrep"},
		Short:   "In-process trigram and AST code search",
		RunE: func(cmd *cobra.Command, args []string) error {
			query := ""
			searchPath := "."
			if pathFlag != "" {
				searchPath = pathFlag
			}

			if len(args) == 1 {
				if pathFlag != "" {
					query = args[0]
				} else if (astKind != "" || astReceiver != "") && (fileutil.Exists(args[0]) || strings.Contains(args[0], "/")) {
					searchPath = args[0]
				} else {
					query = args[0]
				}
			} else if len(args) > 1 {
				query = args[0]
				if pathFlag == "" {
					searchPath = args[1]
				}
			}

			if query == "" && pathFlag == "" && !astMode && astKind == "" && astReceiver == "" && !reindex {
				return fmt.Errorf("search query required (e.g. zqk grep 'pattern')")
			}

			engine := search.NewEngine(".")

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

			if reindex {
				if err := engine.BuildTrigramIndex(opts); err != nil {
					return fmt.Errorf("failed rebuilding trigram index: %w", err)
				}
				fmt.Fprintln(cmd.OutOrStdout(), "✓ Trigram index successfully rebuilt and cached.")
				if query == "" && !astMode && astKind == "" && astReceiver == "" {
					return nil
				}
			}

			result, err := engine.Search(cmd.Context(), opts)
			if err != nil {
				return err
			}

			out := cmd.OutOrStdout()

			switch strings.ToLower(strings.TrimSpace(format)) {
			case "json":
				enc := json.NewEncoder(out)
				enc.SetIndent("", "  ")
				return enc.Encode(result)

			case "yaml":
				enc := yaml.NewEncoder(out)
				return enc.Encode(result)

			default:
				for _, m := range result.Matches {
					for _, before := range m.ContextBefore {
						fmt.Fprintf(out, "%s- %s\n", m.File, before)
					}

					if m.SymbolKind != "" {
						recvPart := ""
						if m.Receiver != "" {
							recvPart = fmt.Sprintf("(%s) ", m.Receiver)
						}
						fmt.Fprintf(out, "%s:%d:%d: [%s] %s%s: %s\n",
							m.File, m.Line, m.Column, m.SymbolKind, recvPart, m.SymbolName, strings.TrimSpace(m.LineContent))
					} else {
						fmt.Fprintf(out, "%s:%d:%d: %s\n", m.File, m.Line, m.Column, m.LineContent)
					}

					for _, after := range m.ContextAfter {
						fmt.Fprintf(out, "%s+ %s\n", m.File, after)
					}
				}

				if len(result.Matches) == 0 {
					return nil
				}

				truncNote := ""
				if result.Truncated {
					truncNote = fmt.Sprintf(" [truncated by %s]", result.TruncateReason)
				}
				_ = truncNote

				return nil
			}
		},
	}

	cmd.Flags().BoolVarP(&ignoreCase, "ignore-case", "i", false, "Case-insensitive search")
	cmd.Flags().BoolVarP(&useRegex, "regex", "e", false, "Treat query as regular expression")
	cmd.Flags().BoolVarP(&wordRegexp, "word-regexp", "w", false, "Match only whole words")
	cmd.Flags().BoolVar(&astMode, "ast", false, "Enable Go AST structural query mode")
	cmd.Flags().StringVar(&astKind, "kind", "", "Filter AST declarations by kind (func, method, struct, interface, type, var, const)")
	cmd.Flags().StringVar(&astReceiver, "recv", "", "Filter AST methods by receiver type name")
	cmd.Flags().IntVarP(&contextLines, "context", "C", 0, "Show NUM lines of surrounding context")
	cmd.Flags().IntVarP(&maxCount, "max-count", "m", search.DefaultMaxMatches, "Stop after NUM matches (default 100)")
	cmd.Flags().IntVar(&maxTokens, "max-tokens", search.DefaultMaxTokens, "Max estimated tokens in response payload (default 4000)")
	cmd.Flags().StringSliceVar(&fileExts, "ext", nil, "Filter files by extension (e.g. --ext .go,.yaml)")
	cmd.Flags().BoolVar(&useIndex, "use-index", true, "Use trigram index acceleration (default: true)")
	cmd.Flags().BoolVar(&reindex, "reindex", false, "Rebuild and persist trigram index cache")
	cmd.Flags().BoolVar(&includeHidden, "hidden", false, "Search hidden files and directories")
	cmd.Flags().StringVarP(&pathFlag, "path", "p", "", "Target file or directory path")
	cmd.Flags().StringVarP(&format, "format", "f", "lines", "Output format (lines, json, yaml)")

	helpBuilder.ApplyToCommand(cmd)
	cli.RequireSession(cmd, false)
	cli.RequireStorage(cmd, false)
	return cmd
}
