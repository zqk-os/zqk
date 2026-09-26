package query

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/traversal"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// NewQueryCmd creates the top-level 'zqk query' command for declarative ZPARQL graph queries.
func NewQueryCmd() *cobra.Command {
	var (
		filePath string
		format   string
	)

	helpBuilder := clipkg.DynamicHelpBuilder(
		"Execute declarative graph queries across the knowledge kernel",
		"Declarative ZPARQL graph pattern matching and relational traversal engine with cycle safety and predicate pushdown.",
		"",
		"Enables agents and operators to perform multi-hop topological queries directly over local CAS storage without requiring an external graph database.",
	).
		AddExample("Find all planned backlog items", `%s query "MATCH (b:backlog_item) WHERE b.status = 'planned' RETURN b.id, b.title;"`).
		AddExample("Multi-hop plan and criteria traversal", `%s query "MATCH (p:priority_plan)-[:items]->(b:backlog_item)-[:criteria_refs]->(c:criteria) RETURN p.title AS plan, b.id AS bli, c.title AS criterion;"`).
		AddExample("Query from file formatted as JSON", `%s query -f query.zparql --format json`)

	cmd := &cobra.Command{
		Use:     "query [query-string]",
		Short:   "Execute declarative ZPARQL graph queries",
		Aliases: []string{"zparql"},
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
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
				return fmt.Errorf("query string required as argument or via -f/--file")
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
		},
	}

	cmd.Flags().StringVarP(&filePath, "file", "f", "", "Path to file containing ZPARQL query")
	cmd.Flags().StringVar(&format, "format", "table", "Output format (table, json)")

	helpBuilder.ApplyToCommand(cmd)
	cli.RequireSession(cmd, false)
	cli.RequireStorage(cmd, false)

	return cmd
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
				"goal", "milestone", "priority_plan", "workstream", "backlog_item",
				"criteria", "test_case", "policy", "persona", "prompt", "audit_event",
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
			id, _ := obj["id"].(string)
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
