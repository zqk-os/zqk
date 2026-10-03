package mcp

import (
	"context"

	"github.com/zqk-os/zqk/pkg/mutation"
	"github.com/zqk-os/zqk/pkg/traversal"
)

// HandleQueryZPARQL handles declarative ZPARQL query tool execution.
func HandleQueryZPARQL(ctx context.Context, s *Server, args map[string]any) (any, error) {
	queryStr, ok := args["query"].(string)
	if !ok || queryStr == "" {
		return nil, NewElicitationError(
			"Missing required parameter: query",
			[]ElicitationParam{
				ElicitParamWithExample(
					"query",
					"The ZPARQL query string to execute (e.g., MATCH (b:backlog_item) WHERE b.status == 'planned' RETURN b.id, b.title;)",
					"string",
					true,
					"MATCH (b:backlog_item) WHERE b.status == 'planned' RETURN b.id, b.title;",
				),
			},
		)
	}

	ast, err := traversal.ParseZPARQL(queryStr)
	if err != nil {
		return map[string]any{
			"error": err.Error(),
			"query": queryStr,
		}, err
	}

	idx := traversal.NewGraphIndex()
	executor := traversal.NewQueryExecutor(idx)
	result, err := executor.Execute(ctx, ast)
	if err != nil {
		return map[string]any{
			"error": err.Error(),
			"query": queryStr,
		}, err
	}

	return map[string]any{
		"headers": result.Headers,
		"rows":    result.Rows,
		"total":   result.Total,
	}, nil
}

// HandleMutateZQL handles declarative ZQL mutation tool execution.
func HandleMutateZQL(ctx context.Context, s *Server, args map[string]any) (any, error) {
	scriptStr, ok := args["script"].(string)
	if !ok || scriptStr == "" {
		return nil, NewElicitationError(
			"Missing required parameter: script",
			[]ElicitationParam{
				ElicitParamWithExample(
					"script",
					"The ZQL mutation script to execute (e.g., BEGIN; LET $p = UPSERT priority_plan { title: 'New Plan', priority_tier: 'P1', status: 'in_progress' }; COMMIT;)",
					"string",
					true,
					"BEGIN; LET $p = UPSERT priority_plan { title: 'New Plan', priority_tier: 'P1', status: 'in_progress' }; COMMIT;",
				),
			},
		)
	}

	dryRun := false
	if dr, ok := args["dry_run"].(bool); ok {
		dryRun = dr
	}

	program, err := mutation.ParseZQL(scriptStr)
	if err != nil {
		return map[string]any{
			"error":  err.Error(),
			"script": scriptStr,
		}, err
	}

	if dryRun {
		program.ApplyDryRunIsolation()
	}

	executor := mutation.NewZQLExecutor(nil)

	receipt, err := executor.Execute(ctx, program)
	if err != nil {
		return map[string]any{
			"error":   err.Error(),
			"receipt": receipt,
		}, err
	}

	return receipt, nil
}
