package mutation_test

import (
	"context"
	"testing"

	"github.com/zqk-os/zqk/pkg/mutation"
)

func BenchmarkZQL_Parse(b *testing.B) {
	script := `
		BEGIN TRANSACTION ISOLATION LEVEL STAGED_SNAPSHOT;
		LET $ws_title = "Swarm Telemetry Overhaul";
		LET $plan = UPSERT priority_plan {
			title: $ws_title,
			priority_tier: "P1",
			status: "in_progress",
			description: "Multi-seat event streaming pipeline"
		} RETURNING id;
		UPSERT backlog_item {
			title: "Implement Stream Consumer Daemon",
			priority_plan_ref: $plan.id,
			priority_tier: "P1",
			status: "planned"
		};
		COMMIT TRANSACTION;
	`
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := mutation.ParseZQL(script)
		if err != nil {
			b.Fatalf("parse failed: %v", err)
		}
	}
}

func BenchmarkZQL_TopologicalSort(b *testing.B) {
	stmts := []mutation.Statement{
		{NodeType: mutation.StmtLet, VariableName: "child4", Expression: mutation.VariableRefExpr{VariableName: "child3"}},
		{NodeType: mutation.StmtLet, VariableName: "child3", Expression: mutation.VariableRefExpr{VariableName: "child2"}},
		{NodeType: mutation.StmtLet, VariableName: "child2", Expression: mutation.VariableRefExpr{VariableName: "child1"}},
		{NodeType: mutation.StmtLet, VariableName: "child1", Expression: mutation.VariableRefExpr{VariableName: "root"}},
		{NodeType: mutation.StmtLet, VariableName: "root", Expression: mutation.LiteralExpr{Value: "root_val"}},
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := mutation.ResolveVariableDependencies(stmts)
		if err != nil {
			b.Fatalf("resolve failed: %v", err)
		}
	}
}

func BenchmarkZQL_Execution(b *testing.B) {
	script := `
		BEGIN TRANSACTION ISOLATION LEVEL STAGED_SNAPSHOT;
		LET $plan = UPSERT priority_plan {
			title: "Benchmark Plan",
			priority_tier: "P1",
			status: "in_progress"
		};
		UPSERT backlog_item {
			title: "Benchmark BLI",
			priority_plan_ref: $plan.id,
			priority_tier: "P1",
			status: "planned"
		};
		COMMIT TRANSACTION;
	`
	program, _ := mutation.ParseZQL(script)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		engine := mutation.NewTransactionEngine()
		executor := mutation.NewZQLExecutor(engine)
		_, err := executor.Execute(ctx, program)
		if err != nil {
			b.Fatalf("execution failed: %v", err)
		}
	}
}
