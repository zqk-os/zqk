package query

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/kindnames"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/traversal"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// NewQueryCmd creates the top-level 'zqk query' command for declarative ZPARQL graph queries.
func NewQueryCmd() *cobra.Command {
	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewQueryCommandBuilder(), &cobra.Command{
		RunE: runQuery,
	})
	cmd.Flags().BoolP("interactive", "i", false, "Start interactive ZPARQL REPL console")
	cmd.Flags().Bool("visualize", false, "Format query results as graph edge path visualization")
	cli.BindAsyncProgress(cmd, runQuery)
	return cmd
}

func runQuery(cmd *cobra.Command, args []string) error {
	filePath, _ := cmd.Flags().GetString("file")
	format, _ := cmd.Flags().GetString("format")
	interactive, _ := cmd.Flags().GetBool("interactive")
	visualize, _ := cmd.Flags().GetBool("visualize")

	if interactive || (len(args) > 0 && args[0] == "repl") {
		proc, err := cli.NewProcessor(cmd)
		if err != nil {
			return fmt.Errorf("failed to initialize processor for interactive query: %w", err)
		}
		return RunInteractiveREPL(cmd, proc, visualize)
	}

	var queryStr string

	if filePath != "" {
		if filePath == "-" {
			b, err := io.ReadAll(cmd.InOrStdin())
			if err != nil {
				return fmt.Errorf("failed to read query from stdin: %w", err)
			}
			queryStr = string(b)
		} else {
			data, err := fileutil.ReadFile(filePath)
			if err != nil {
				return fmt.Errorf("failed to read query file: %w", err)
			}
			queryStr = string(data)
		}
	} else if len(args) > 0 {
		if args[0] == "-" {
			b, err := io.ReadAll(cmd.InOrStdin())
			if err != nil {
				return fmt.Errorf("failed to read query from stdin: %w", err)
			}
			queryStr = string(b)
		} else {
			queryStr = args[0]
		}
	} else {
		return fmt.Errorf("query string required as argument or via -f/--file (or pass -i / --interactive for console)")
	}

	ast, err := traversal.ParseZPARQL(queryStr)
	if err != nil {
		return fmt.Errorf("ZPARQL query syntax error: %w", err)
	}

	// Build graph index from kernel storage
	idx := traversal.NewGraphIndex()
	targetKinds := extractTargetKinds(ast)
	if proc, procErr := cli.NewProcessor(cmd); procErr == nil && proc.Storage() != nil {
		_ = PopulateIndexFromStorage(cmd.Context(), proc, idx, targetKinds...)
	}

	executor := traversal.NewQueryExecutor(idx)
	result, err := executor.Execute(cmd.Context(), ast)
	if err != nil {
		return fmt.Errorf("query execution failed: %w", err)
	}

	out := cmd.OutOrStdout()
	if strings.EqualFold(format, "json") {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	}

	if visualize || strings.EqualFold(format, "visual") {
		fmt.Fprint(out, FormatVisualizer(result, ast))
		return nil
	}

	// Table / human-readable output
	if len(result.Rows) == 0 {
		fmt.Fprintln(out, "No matches found.")
		return nil
	}

	// Print header
	headerLine := strings.Join(result.Headers, "\t| ")
	fmt.Fprintln(out, headerLine)
	fmt.Fprintln(out, strings.Repeat("-", len(headerLine)+10))

	for _, row := range result.Rows {
		rowVals := make([]string, len(result.Headers))
		for i, h := range result.Headers {
			rowVals[i] = fmt.Sprintf("%v", row[h])
		}
		fmt.Fprintln(out, strings.Join(rowVals, "\t| "))
	}

	fmt.Fprintf(out, "\n(%d rows)\n", len(result.Rows))
	return nil
}

func extractTargetKinds(ast *traversal.QueryAST) []string {
	if ast == nil || len(ast.Patterns) == 0 {
		return nil
	}
	kindsSet := make(map[string]struct{})
	for _, p := range ast.Patterns {
		for _, node := range p.Nodes {
			if node.Kind == "" {
				return nil // Wildcard match requires scanning all kinds
			}
			kindsSet[node.Kind] = struct{}{}
		}
	}
	var res []string
	for k := range kindsSet {
		res = append(res, k)
	}
	return res
}

// PopulateIndexFromStorage scans known object kinds and indexes nodes and relationships.
func PopulateIndexFromStorage(ctx context.Context, proc *cli.Processor, idx *traversal.GraphIndex, targetKinds ...string) error {
	var kinds []string
	if len(targetKinds) > 0 {
		kinds = targetKinds
	} else {
		registry := objects.GetGlobalFieldRegistry()
		_ = registry.LoadFields()
		var err error
		kinds, err = registry.GetAllKinds()
		if err != nil || len(kinds) == 0 {
			kinds = []string{
				kindnames.Goal,
				kindnames.Milestone,
				kindnames.PriorityPlan,
				kindnames.Workstream,
				kindnames.BacklogItem,
				kindnames.Criteria,
				kindnames.TestCase,
				kindnames.Policy,
				kindnames.Persona,
				kindnames.PromptTemplate,
				kindnames.AuditEvent,
				kindnames.TechnicalDebt,
			}
		}
	}

	storageCtx := proc.StorageContext()
	for _, kind := range kinds {
		filter := storage.ListFilter{Kind: kind}
		res, err := proc.Storage().List(proc.OperationContext(), proc.SecurityContext(), storageCtx, filter)
		if err != nil || res == nil {
			continue
		}
		for _, obj := range res.Objects {
			id, _ := obj[objects.FieldKeyID].(string)
			if id == "" {
				continue
			}
			idx.AddNode(id, kind, obj)

			for fieldName, fieldVal := range obj {
				if fieldName == "depends_on" {
					addEdges(idx, id, "depends_on", fieldVal)
				} else if strings.HasSuffix(fieldName, "_refs") {
					addEdges(idx, id, fieldName, fieldVal)
					shortRel := strings.TrimSuffix(fieldName, "_refs")
					if shortRel != fieldName {
						addEdges(idx, id, shortRel, fieldVal)
					}
				} else if strings.HasSuffix(fieldName, "_ref") {
					addEdges(idx, id, fieldName, fieldVal)
					shortRel := strings.TrimSuffix(fieldName, "_ref")
					if shortRel != fieldName {
						addEdges(idx, id, shortRel, fieldVal)
					}
				}
			}
		}
	}
	return nil
}

func addEdges(idx *traversal.GraphIndex, sourceID, relation string, val any) {
	switch v := val.(type) {
	case string:
		if v != "" {
			idx.AddEdge(sourceID, relation, v)
		}
	case []string:
		for _, target := range v {
			if target != "" {
				idx.AddEdge(sourceID, relation, target)
			}
		}
	case []any:
		for _, item := range v {
			if target, ok := item.(string); ok && target != "" {
				idx.AddEdge(sourceID, relation, target)
			}
		}
	}
}
