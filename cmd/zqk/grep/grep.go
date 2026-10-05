package grep

import (
	"encoding/json"
	"fmt"
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
			query := ""
			searchPath := "."
			ignoreCase, _ := cmd.Flags().GetBool("ignore-case")
			useRegex, _ := cmd.Flags().GetBool("regex")
			wordRegexp, _ := cmd.Flags().GetBool("word-regexp")
			astMode, _ := cmd.Flags().GetBool("ast")
			astKind, _ := cmd.Flags().GetString("kind")
			astReceiver, _ := cmd.Flags().GetString("recv")
			contextLines, _ := cmd.Flags().GetInt("context")
			maxCount, _ := cmd.Flags().GetInt("max-count")
			maxTokens, _ := cmd.Flags().GetInt("max-tokens")
			rawExts, _ := cmd.Flags().GetStringArray("ext")
			var fileExts []string
			for _, e := range rawExts {
				for _, part := range strings.Split(e, ",") {
					part = strings.TrimSpace(part)
					if part != "" {
						fileExts = append(fileExts, part)
					}
				}
			}
			useIndex, _ := cmd.Flags().GetBool("use-index")
			reindex, _ := cmd.Flags().GetBool("reindex")
			includeHidden, _ := cmd.Flags().GetBool("hidden")
			pathFlag, _ := cmd.Flags().GetString("path")
			format, _ := cmd.Flags().GetString("format")

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
				return fmt.Errorf("%s", paths.RewriteCanonicalCLIInvocations("search query required (e.g. zqk grep 'pattern')"))
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
	})

	helpBuilder.ApplyToCommand(cmd)
	cli.RequireSession(cmd, false)
	cli.RequireStorage(cmd, false)
	return cmd
}
