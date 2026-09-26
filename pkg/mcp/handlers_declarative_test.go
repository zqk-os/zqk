package mcp

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/mutation"
)

func TestHandleQueryZPARQL(t *testing.T) {
	ctx := context.Background()
	server := NewServer()

	t.Run("missing query parameter", func(t *testing.T) {
		_, err := HandleQueryZPARQL(ctx, server, map[string]any{})
		require.Error(t, err)
		require.Contains(t, err.Error(), "Missing required parameter")
	})

	t.Run("syntax error query", func(t *testing.T) {
		_, err := HandleQueryZPARQL(ctx, server, map[string]any{
			"query": "INVALID SYNTAX;",
		})
		require.Error(t, err)
	})

	t.Run("valid query execution", func(t *testing.T) {
		res, err := HandleQueryZPARQL(ctx, server, map[string]any{
			"query": "MATCH (b:backlog_item) RETURN b.id AS id, b.title AS title;",
		})
		require.NoError(t, err)
		m, ok := res.(map[string]any)
		require.True(t, ok)
		require.Contains(t, m, "headers")
		require.Contains(t, m, "rows")
		require.Contains(t, m, "total")
	})
}

func TestHandleMutateZQL(t *testing.T) {
	ctx := context.Background()
	server := NewServer()

	t.Run("missing script parameter", func(t *testing.T) {
		_, err := HandleMutateZQL(ctx, server, map[string]any{})
		require.Error(t, err)
		require.Contains(t, err.Error(), "Missing required parameter")
	})

	t.Run("syntax error script", func(t *testing.T) {
		_, err := HandleMutateZQL(ctx, server, map[string]any{
			"script": "INVALID MUTATION;",
		})
		require.Error(t, err)
	})

	t.Run("valid dry-run execution", func(t *testing.T) {
		script := `
			BEGIN;
			LET $plan = UPSERT priority_plan {
				title: "MCP Plan",
				priority_tier: "P1",
				status: "in_progress"
			};
			COMMIT;
		`
		res, err := HandleMutateZQL(ctx, server, map[string]any{
			"script":  script,
			"dry_run": true,
		})
		require.NoError(t, err)
		receipt, ok := res.(*mutation.ZQLExecutionReceipt)
		require.True(t, ok)
		require.Equal(t, mutation.IsolationDryRun, receipt.IsolationLevel)
		require.Len(t, receipt.Receipts, 1)
		require.Equal(t, mutation.ReceiptStatusDryRunValidated, receipt.Receipts[0].Status)
	})

	t.Run("valid live execution", func(t *testing.T) {
		script := `
			BEGIN;
			LET $plan = UPSERT priority_plan {
				title: "Live MCP Plan",
				priority_tier: "P1",
				status: "in_progress"
			};
			COMMIT;
		`
		res, err := HandleMutateZQL(ctx, server, map[string]any{
			"script":  script,
			"dry_run": false,
		})
		require.NoError(t, err)
		receipt, ok := res.(*mutation.ZQLExecutionReceipt)
		require.True(t, ok)
		require.True(t, receipt.Committed)
		require.Len(t, receipt.Receipts, 1)
		require.Equal(t, mutation.ReceiptStatusCommitted, receipt.Receipts[0].Status)
	})
}
