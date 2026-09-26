package mutation_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/mutation"
)

func TestParseZQL_ConcreteWorkstreamExample(t *testing.T) {
	script := `
		BEGIN TRANSACTION ISOLATION LEVEL STAGED_SNAPSHOT;

		LET $ws_title = "Swarm Telemetry Overhaul";

		LET $plan = UPSERT priority_plan {
			title: $ws_title,
			priority_tier: "P1",
			status: "in_progress",
			description: "Multi-seat event streaming pipeline"
		} RETURNING id, version_context;

		UPSERT backlog_item {
			title: "Implement Stream Consumer Daemon",
			priority_plan_ref: $plan.id,
			priority_tier: "P1",
			status: "planned",
			estimated_effort: "4h"
		};

		UPSERT backlog_item {
			title: "Stream Buffer Shockwave Verification",
			priority_plan_ref: $plan.id,
			priority_tier: "P1",
			status: "planned",
			estimated_effort: "2h"
		};

		COMMIT TRANSACTION;
	`

	program, err := mutation.ParseZQL(script)
	require.NoError(t, err)
	require.NotNil(t, program)
	require.Equal(t, "1.0.0", program.Version)
	require.Equal(t, "Program", program.Type)
	require.Len(t, program.Statements, 6)

	require.Equal(t, mutation.StmtBeginTransaction, program.Statements[0].NodeType)
	require.Equal(t, mutation.IsolationStagedSnapshot, program.Statements[0].IsolationLevel)
	require.Equal(t, mutation.StmtLet, program.Statements[1].NodeType)
	require.Equal(t, "ws_title", program.Statements[1].VariableName)
	require.Equal(t, mutation.StmtLet, program.Statements[2].NodeType)
	require.Equal(t, "plan", program.Statements[2].VariableName)
	require.Equal(t, mutation.StmtUpsert, program.Statements[3].NodeType)
	require.Equal(t, mutation.StmtUpsert, program.Statements[4].NodeType)
	require.Equal(t, mutation.StmtCommitTransaction, program.Statements[5].NodeType)
}

func TestParseZQL_KahnTopologicalResolution(t *testing.T) {
	// Notice: $child references $parent, but is declared first
	script := `
		LET $child_title = $parent_title;
		LET $parent_title = "Parent Epic";
	`
	program, err := mutation.ParseZQL(script)
	require.NoError(t, err)
	require.Len(t, program.Statements, 2)
	// After Kahn's resolution, $parent_title must precede $child_title
	require.Equal(t, "parent_title", program.Statements[0].VariableName)
	require.Equal(t, "child_title", program.Statements[1].VariableName)
}

func TestParseZQL_NegativeInvariants(t *testing.T) {
	t.Run("Syntax Error", func(t *testing.T) {
		_, err := mutation.ParseZQL(`INVALID KEYWORD FOOBAR;`)
		require.Error(t, err)
		require.Contains(t, err.Error(), mutation.ErrCodeZQLSyntaxError)
	})

	t.Run("Unbound Variable Reference", func(t *testing.T) {
		script := `
			UPSERT backlog_item {
				title: "Task",
				parent_ref: $unknown_parent.id
			};
		`
		_, err := mutation.ParseZQL(script)
		require.Error(t, err)
		require.Contains(t, err.Error(), mutation.ErrCodeZQLUnboundVariable)
	})

	t.Run("Circular Dependency Detection", func(t *testing.T) {
		script := `
			LET $a = $b;
			LET $b = $a;
		`
		_, err := mutation.ParseZQL(script)
		require.Error(t, err)
		require.Contains(t, err.Error(), mutation.ErrCodeZQLCircularDependency)
	})

	t.Run("Self Reference Circular Dependency", func(t *testing.T) {
		script := `
			LET $self = $self;
		`
		_, err := mutation.ParseZQL(script)
		require.Error(t, err)
		require.Contains(t, err.Error(), mutation.ErrCodeZQLCircularDependency)
	})
}

func TestExecuteZQL_EndToEnd(t *testing.T) {
	engine := mutation.NewTransactionEngine()
	executor := mutation.NewZQLExecutor(engine)

	script := `
		BEGIN TRANSACTION ISOLATION LEVEL STAGED_SNAPSHOT;

		LET $plan = UPSERT priority_plan {
			title: "Declarative Engines Delivery",
			priority_tier: "P1",
			status: "in_progress"
		};

		UPSERT backlog_item {
			title: "Implement ZPARQL Frontend",
			priority_plan_ref: $plan.id,
			priority_tier: "P1",
			status: "planned"
		};

		COMMIT TRANSACTION;
	`

	program, err := mutation.ParseZQL(script)
	require.NoError(t, err)

	receipt, err := executor.Execute(context.Background(), program)
	require.NoError(t, err)
	require.NotNil(t, receipt)
	require.True(t, receipt.Committed)
	require.Len(t, receipt.Receipts, 2)
	require.Equal(t, mutation.ReceiptStatusCommitted, receipt.Receipts[0].Status)
	require.Equal(t, mutation.ReceiptStatusCommitted, receipt.Receipts[1].Status)

	// Verify committed items in engine
	require.Equal(t, 2, engine.Count(context.Background()))
}
