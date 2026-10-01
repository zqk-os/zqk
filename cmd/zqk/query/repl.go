package query

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/traversal"
)

// RunInteractiveREPL starts an interactive ZPARQL query console.
func RunInteractiveREPL(cmd *cobra.Command, proc *cli.Processor, defaultVisualize bool) error {
	in := cmd.InOrStdin()
	out := cmd.OutOrStdout()
	errOut := cmd.ErrOrStderr()

	printREPLBanner(out)

	// Populate graph index from storage
	idx := traversal.NewGraphIndex()
	fmt.Fprintln(out, "⚡ Indexing knowledge kernel graph from storage...")
	indexStart := time.Now()
	if proc != nil && proc.Storage() != nil {
		_ = PopulateIndexFromStorage(cmd.Context(), proc, idx)
	}
	allNodeIDs := idx.GetAllNodeIDs()
	fmt.Fprintf(out, "✓ Indexed %d node(s) across kernel entities (%v).\n\n", len(allNodeIDs), time.Since(indexStart).Round(time.Millisecond))

	scanner := bufio.NewScanner(in)
	var queryBuffer strings.Builder
	visualizeMode := defaultVisualize
	currentFormat := "table"
	if defaultVisualize {
		currentFormat = "visual"
	}

	for {
		if queryBuffer.Len() == 0 {
			fmt.Fprint(out, "zparql> ")
		} else {
			fmt.Fprint(out, "   ...> ")
		}

		if !scanner.Scan() {
			// EOF reached
			break
		}

		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		// Check for meta-commands starting with dot
		if strings.HasPrefix(line, ".") {
			cmdParts := strings.Fields(line)
			metaCmd := strings.ToLower(cmdParts[0])

			switch metaCmd {
			case ".exit", ".quit", ".q":
				fmt.Fprintln(out, "Exiting ZPARQL console. Goodbye!")
				return nil

			case ".help", ".h", "?":
				printREPLHelp(out)
				continue

			case ".kinds":
				printIndexedKinds(out, idx)
				continue

			case ".tables", ".relations", ".edges":
				printIndexedRelations(out, idx)
				continue

			case ".schema":
				targetKind := ""
				if len(cmdParts) > 1 {
					targetKind = cmdParts[1]
				}
				printSchemaInfo(out, targetKind)
				continue

			case ".clear":
				queryBuffer.Reset()
				fmt.Fprintln(out, "Query buffer cleared.")
				continue

			case ".visualize":
				if len(cmdParts) > 1 {
					switch strings.ToLower(cmdParts[1]) {
					case "on", "true", "1", "yes":
						visualizeMode = true
					case "off", "false", "0", "no":
						visualizeMode = false
					default:
						visualizeMode = !visualizeMode
					}
				} else {
					visualizeMode = !visualizeMode
				}
				if visualizeMode {
					currentFormat = "visual"
					fmt.Fprintln(out, "Graph path visualizer mode enabled.")
				} else {
					currentFormat = "table"
					fmt.Fprintln(out, "Graph path visualizer mode disabled (tabular output active).")
				}
				continue

			case ".format":
				if len(cmdParts) > 1 {
					fmtChoice := strings.ToLower(cmdParts[1])
					switch fmtChoice {
					case "table", "json", "visual":
						currentFormat = fmtChoice
						visualizeMode = (fmtChoice == "visual")
						fmt.Fprintf(out, "Output format set to: %s\n", currentFormat)
					default:
						fmt.Fprintln(errOut, "Unknown format. Supported: table, json, visual")
					}
				} else {
					fmt.Fprintf(out, "Current format: %s (visualize=%v)\n", currentFormat, visualizeMode)
				}
				continue

			case ".refresh", ".reload":
				fmt.Fprintln(out, "⚡ Refreshing knowledge kernel graph from storage...")
				idx = traversal.NewGraphIndex()
				refStart := time.Now()
				if proc != nil && proc.Storage() != nil {
					_ = PopulateIndexFromStorage(cmd.Context(), proc, idx)
				}
				fmt.Fprintf(out, "✓ Re-indexed %d node(s) (%v).\n", len(idx.GetAllNodeIDs()), time.Since(refStart).Round(time.Millisecond))
				continue

			case ".run":
				// Fall through to execute whatever is in buffer
				if queryBuffer.Len() == 0 {
					fmt.Fprintln(errOut, "Query buffer is empty.")
					continue
				}

			default:
				fmt.Fprintf(errOut, "Unknown meta-command %q. Type .help for available commands.\n", metaCmd)
				continue
			}
		}

		if metaCmd := strings.ToLower(strings.Fields(line)[0]); metaCmd != ".run" {
			if queryBuffer.Len() > 0 {
				queryBuffer.WriteString(" ")
			}
			queryBuffer.WriteString(line)
		}

		rawQuery := strings.TrimSpace(queryBuffer.String())
		// Semicolon-terminated statement or explicit .run triggers query execution
		if strings.HasSuffix(rawQuery, ";") || strings.HasPrefix(line, ".run") {
			cleanQuery := strings.TrimSuffix(rawQuery, ";")
			cleanQuery = strings.TrimSpace(cleanQuery)
			if cleanQuery != "" {
				execStart := time.Now()
				ast, err := traversal.ParseZPARQL(rawQuery)
				if err != nil {
					fmt.Fprintf(errOut, "❌ ZPARQL Syntax Error: %v\n\n", err)
					queryBuffer.Reset()
					continue
				}

				executor := traversal.NewQueryExecutor(idx)
				result, err := executor.Execute(cmd.Context(), ast)
				execDuration := time.Since(execStart)

				if err != nil {
					fmt.Fprintf(errOut, "❌ Execution Error: %v\n\n", err)
					queryBuffer.Reset()
					continue
				}

				renderQueryResult(out, result, ast, currentFormat, execDuration)
			}
			queryBuffer.Reset()
		}
	}

	if err := scanner.Err(); err != nil && err != io.EOF {
		return fmt.Errorf("error reading interactive input: %w", err)
	}

	return nil
}

func printREPLBanner(w io.Writer) {
	fmt.Fprintln(w, "╔════════════════════════════════════════════════════════════════════════════╗")
	fmt.Fprintln(w, "║                 ZPARQL Interactive Graph Query Console                     ║")
	fmt.Fprintln(w, "║      Type ZPARQL queries ending with a semicolon (;).                      ║")
	fmt.Fprintln(w, "║      Type .help for commands, .kinds for ontology nodes, .exit to quit.    ║")
	fmt.Fprintln(w, "╚════════════════════════════════════════════════════════════════════════════╝")
	fmt.Fprintln(w)
}

func printREPLHelp(w io.Writer) {
	fmt.Fprintln(w, "\nAvailable Meta-Commands:")
	fmt.Fprintln(w, "  .help, .h            Show this help guide")
	fmt.Fprintln(w, "  .kinds               List all indexed object kinds and node counts")
	fmt.Fprintln(w, "  .tables, .edges      List graph relationships and edge counts")
	fmt.Fprintln(w, "  .schema [kind]       Display schema definition and fields for an object kind")
	fmt.Fprintln(w, "  .visualize [on|off]  Toggle graph path visualizer mode")
	fmt.Fprintln(w, "  .format [fmt]        Set output format (table, json, visual)")
	fmt.Fprintln(w, "  .refresh             Re-index knowledge kernel storage graph")
	fmt.Fprintln(w, "  .clear               Clear current multi-line query buffer")
	fmt.Fprintln(w, "  .exit, .quit, .q     Exit interactive REPL")
	fmt.Fprintln(w, "\nExample ZPARQL Queries:")
	fmt.Fprintln(w, "  MATCH (b:backlog_item) RETURN b.id, b.status LIMIT 5;")
	fmt.Fprintln(w, "  MATCH (g:goal) WHERE g.status == 'active' RETURN g.id, g.title;")
	fmt.Fprintln(w, "  MATCH (p:priority_plan)-[:items]->(b:backlog_item) RETURN p.title, b.id;")
	fmt.Fprintln(w, "  MATCH (b:backlog_item) RETURN count(*);")
	fmt.Fprintln(w)
}

func printIndexedKinds(w io.Writer, idx *traversal.GraphIndex) {
	if idx == nil {
		fmt.Fprintln(w, "Graph index uninitialized.")
		return
	}
	allKinds := make([]string, 0)
	for _, id := range idx.GetAllNodeIDs() {
		if node, ok := idx.GetNode(id); ok {
			if k, ok := node["kind"].(string); ok && k != "" {
				allKinds = append(allKinds, k)
			}
		}
	}
	kindCounts := make(map[string]int)
	for _, k := range allKinds {
		kindCounts[k]++
	}
	sortedKinds := make([]string, 0, len(kindCounts))
	for k := range kindCounts {
		sortedKinds = append(sortedKinds, k)
	}
	sort.Strings(sortedKinds)

	fmt.Fprintln(w, "\nIndexed Object Kinds:")
	fmt.Fprintln(w, "KIND\t\t\t\tCOUNT")
	fmt.Fprintln(w, "----------------------------------------------------")
	for _, k := range sortedKinds {
		fmt.Fprintf(w, "%-30s\t%d\n", k, kindCounts[k])
	}
	fmt.Fprintf(w, "\n(%d unique kinds, %d total nodes)\n\n", len(sortedKinds), len(allKinds))
}

func printIndexedRelations(w io.Writer, idx *traversal.GraphIndex) {
	if idx == nil {
		fmt.Fprintln(w, "Graph index uninitialized.")
		return
	}
	fmt.Fprintln(w, "\nIndexed Graph Relationships & Edges:")
	fmt.Fprintln(w, "Common Traversal Edges: items, criteria_refs, requirement_refs, milestone_refs, depends_on, persona_refs, goal_refs")
	fmt.Fprintln(w, "Example: MATCH (p:priority_plan)-[:items]->(b:backlog_item)-[:criteria_refs]->(c:criteria) RETURN p.title, b.id, c.id;")
	fmt.Fprintln(w)
}

func printSchemaInfo(w io.Writer, kind string) {
	registry := objects.GetGlobalFieldRegistry()
	_ = registry.LoadFields()

	if kind == "" {
		allKinds, err := registry.GetAllKinds()
		if err != nil {
			fmt.Fprintf(w, "Failed to load kinds: %v\n", err)
			return
		}
		sort.Strings(allKinds)
		fmt.Fprintln(w, "Available Schemas: Use '.schema <kind>' to inspect fields.")
		fmt.Fprintf(w, "%s\n\n", strings.Join(allKinds, ", "))
		return
	}

	kindFields, err := registry.GetFieldsForKind(kind)
	if err != nil || kindFields == nil || len(kindFields.AllFields) == 0 {
		fmt.Fprintf(w, "No schema fields found for kind %q.\n\n", kind)
		return
	}

	fmt.Fprintf(w, "\nSchema Fields for %q (%d fields):\n", kind, len(kindFields.AllFields))
	for _, info := range kindFields.AllFields {
		reqStr := ""
		if info.Required {
			reqStr = " [REQUIRED]"
		}
		fmt.Fprintf(w, "  %-25s %-15s%s\n", info.Name, info.Type, reqStr)
	}
	fmt.Fprintln(w)
}

func renderQueryResult(w io.Writer, result *traversal.QueryResult, ast *traversal.QueryAST, format string, elapsed time.Duration) {
	if strings.EqualFold(format, "json") {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		_ = enc.Encode(result)
		return
	}

	if strings.EqualFold(format, "visual") {
		fmt.Fprint(w, FormatVisualizer(result, ast))
		fmt.Fprintf(w, "Query executed in %v\n\n", elapsed.Round(time.Microsecond))
		return
	}

	// Tabular format
	if len(result.Rows) == 0 {
		fmt.Fprintf(w, "No matches found. (0 rows in %v)\n\n", elapsed.Round(time.Microsecond))
		return
	}

	headerLine := strings.Join(result.Headers, "\t| ")
	fmt.Fprintln(w, headerLine)
	fmt.Fprintln(w, strings.Repeat("-", len(headerLine)+10))

	for _, row := range result.Rows {
		rowVals := make([]string, len(result.Headers))
		for i, h := range result.Headers {
			rowVals[i] = fmt.Sprintf("%v", row[h])
		}
		fmt.Fprintln(w, strings.Join(rowVals, "\t| "))
	}

	fmt.Fprintf(w, "\n(%d rows in %v)\n\n", len(result.Rows), elapsed.Round(time.Microsecond))
}
