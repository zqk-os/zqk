package search

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestPolyglotASTSearch_TypeScriptPythonRustGo(t *testing.T) {
	t.Parallel()
	root := t.TempDir()

	tsSrc := `
export interface TokenBudgetConfig {
  softLimit: number;
}

export type BudgetMode = "strict" | "advisory";

export class BudgetTracker {
  async evaluateBudget(tokens: number): Promise<boolean> {
    if (tokens > 1000) {
      return false;
    }
    return true;
  }
}

export const formatReport = (count: number): string => {
  return String(count);
};

export function createTracker(): BudgetTracker {
  return new BudgetTracker();
}
`
	pySrc := `
class PolicyEvaluator:
    def evaluate_rule(self, rule_id: str) -> bool:
        return True

    async def evaluate_batch(self, rules):
        return []

def build_default_evaluator():
    return PolicyEvaluator()
`
	rsSrc := `
pub trait Verifier {
    fn verify(&self) -> bool;
}

pub struct QuorumEngine {
    pub size: usize,
}

impl QuorumEngine {
    pub async fn elect_leader(&self) -> bool {
        true
    }
}

pub fn new_quorum(size: usize) -> QuorumEngine {
    QuorumEngine { size }
}
`
	goSrc := `package demo

type Runner struct{}

func (r *Runner) Execute() error { return nil }
`

	require.NoError(t, fileutil.WriteFile(filepath.Join(root, "budget.ts"), []byte(tsSrc), paths.FilePerm644))
	require.NoError(t, fileutil.WriteFile(filepath.Join(root, "policy.py"), []byte(pySrc), paths.FilePerm644))
	require.NoError(t, fileutil.WriteFile(filepath.Join(root, "quorum.rs"), []byte(rsSrc), paths.FilePerm644))
	require.NoError(t, fileutil.WriteFile(filepath.Join(root, "runner.go"), []byte(goSrc), paths.FilePerm644))

	engine := NewEngine(root)
	ctx := context.Background()

	// 1. All functions & methods across TS, Python, Rust, and Go
	resAll, err := engine.Search(ctx, SearchOptions{
		Mode:    ModeAST,
		ASTKind: "func",
	})
	require.NoError(t, err)
	names := make(map[string]string)
	for _, m := range resAll.Matches {
		names[m.SymbolName] = m.Receiver
	}
	assert.Contains(t, names, "evaluateBudget")
	assert.Equal(t, "BudgetTracker", names["evaluateBudget"])
	assert.Contains(t, names, "formatReport")
	assert.Contains(t, names, "createTracker")
	assert.Contains(t, names, "evaluate_rule")
	assert.Equal(t, "PolicyEvaluator", names["evaluate_rule"])
	assert.Contains(t, names, "evaluate_batch")
	assert.Contains(t, names, "build_default_evaluator")
	assert.Contains(t, names, "elect_leader")
	assert.Equal(t, "QuorumEngine", names["elect_leader"])
	assert.Contains(t, names, "new_quorum")
	assert.Contains(t, names, "Execute")
	assert.Equal(t, "Runner", names["Execute"])

	// 2. Receiver-scoped method lookup on Python class
	resPyRecv, err := engine.Search(ctx, SearchOptions{
		Mode:        ModeAST,
		ASTKind:     "method",
		ASTReceiver: "PolicyEvaluator",
	})
	require.NoError(t, err)
	assert.Len(t, resPyRecv.Matches, 2)

	// 3. Interface / Trait structural parity across TS and Rust
	resIfaces, err := engine.Search(ctx, SearchOptions{
		Mode:    ModeAST,
		ASTKind: "interface",
	})
	require.NoError(t, err)
	ifaceNames := make(map[string]bool)
	for _, m := range resIfaces.Matches {
		ifaceNames[m.SymbolName] = true
	}
	assert.True(t, ifaceNames["TokenBudgetConfig"])
	assert.True(t, ifaceNames["Verifier"])

	// 4. Token-budget truncation under ModeAST
	resBudget, err := engine.Search(ctx, SearchOptions{
		Mode:      ModeAST,
		MaxTokens: 40,
	})
	require.NoError(t, err)
	assert.True(t, resBudget.Truncated, "tight MaxTokens budget must truncate polyglot AST matches")
	assert.LessOrEqual(t, resBudget.EstimatedTokens, 60)
}
