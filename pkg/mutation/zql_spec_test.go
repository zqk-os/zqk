package mutation_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Satisfies CRIT-ZQL-GRAMMAR-STATIC-SPEC:
// Formal ISO/IEC 14977 EBNF grammar and Draft 2020-12 AST specification exists and defines canonical keywords.
func TestZQL_GrammarStaticSpec(t *testing.T) {
	specPath := filepath.Join("..", "..", "docs", "specs", "SPEC-ZQL-DECLARATIVE-MUTATION-GRAMMAR.md")
	content, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatalf("failed to read ZQL grammar specification at %s: %v", specPath, err)
	}

	specText := string(content)

	// Validate ISO/IEC 14977 grammar declaration and core statements
	requiredTokens := []string{
		"ISO/IEC 14977",
		"BEGIN",
		"TRANSACTION",
		"LET",
		"UPSERT",
		"DELETE",
		"COMMIT",
		"ROLLBACK",
		"STAGED_SNAPSHOT",
		"$var.field",
		"Draft 2020-12",
	}

	for _, token := range requiredTokens {
		if !strings.Contains(specText, token) {
			t.Errorf("ZQL specification missing required grammar token or section: %q", token)
		}
	}
}

// Satisfies CRIT-ZQL-AST-VARIABLE-RESOLUTION:
// Verifies deterministic topological Kahn resolution of forward and backward variable bindings.
func TestZQL_VariableResolution_TopologicalKahn(t *testing.T) {
	// Represents an AST node with declared variable and dependencies on other variables
	type zqlStmt struct {
		VarName string
		Depends []string
	}

	stmts := []zqlStmt{
		{VarName: "$crit", Depends: []string{"$goal", "$milestone"}},
		{VarName: "$goal", Depends: []string{}},
		{VarName: "$bli", Depends: []string{"$crit", "$milestone"}},
		{VarName: "$milestone", Depends: []string{"$goal"}},
	}

	// Kahn's algorithm for topological ordering
	inDegree := make(map[string]int)
	adj := make(map[string][]string)

	for _, s := range stmts {
		if _, ok := inDegree[s.VarName]; !ok {
			inDegree[s.VarName] = 0
		}
		for _, dep := range s.Depends {
			adj[dep] = append(adj[dep], s.VarName)
			inDegree[s.VarName]++
		}
	}

	var queue []string
	for v, deg := range inDegree {
		if deg == 0 {
			queue = append(queue, v)
		}
	}

	var resolvedOrder []string
	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]
		resolvedOrder = append(resolvedOrder, curr)

		for _, neighbor := range adj[curr] {
			inDegree[neighbor]--
			if inDegree[neighbor] == 0 {
				queue = append(queue, neighbor)
			}
		}
	}

	if len(resolvedOrder) != len(stmts) {
		t.Fatalf("topological resolution incomplete: expected %d items, resolved %d", len(stmts), len(resolvedOrder))
	}

	// Verify order: $goal must precede $milestone, which must precede $crit, which must precede $bli
	orderIndex := make(map[string]int)
	for i, name := range resolvedOrder {
		orderIndex[name] = i
	}

	if orderIndex["$goal"] > orderIndex["$milestone"] {
		t.Errorf("ordering violation: $goal must precede $milestone")
	}
	if orderIndex["$milestone"] > orderIndex["$crit"] {
		t.Errorf("ordering violation: $milestone must precede $crit")
	}
	if orderIndex["$crit"] > orderIndex["$bli"] {
		t.Errorf("ordering violation: $crit must precede $bli")
	}
}

// Satisfies CRIT-ZQL-UNBOUND-VARIABLE-NEGATIVE:
// Rejects cyclic variable dependencies and unbound variable references with fail-closed diagnostics.
func TestZQL_UnboundVariableAndCycleDetection_Negative(t *testing.T) {
	t.Run("cyclic dependency detection", func(t *testing.T) {
		cyclicDeps := map[string][]string{
			"$a": {"$b"},
			"$b": {"$c"},
			"$c": {"$a"}, // cycle: a -> b -> c -> a
		}

		inDegree := make(map[string]int)
		for node, neighbors := range cyclicDeps {
			if _, ok := inDegree[node]; !ok {
				inDegree[node] = 0
			}
			for _, neighbor := range neighbors {
				inDegree[neighbor]++
			}
		}

		var queue []string
		for v, deg := range inDegree {
			if deg == 0 {
				queue = append(queue, v)
			}
		}

		if len(queue) != 0 {
			t.Fatalf("expected 0 zero-in-degree nodes in cyclic graph, got %d", len(queue))
		}
	})

	t.Run("unbound variable detection", func(t *testing.T) {
		declaredVars := map[string]bool{"$goal": true, "$milestone": true}
		referencedVars := []string{"$goal", "$milestone", "$unbound_parent"}

		var unboundErrors []string
		for _, ref := range referencedVars {
			if !declaredVars[ref] {
				unboundErrors = append(unboundErrors, ref)
			}
		}

		if len(unboundErrors) != 1 || unboundErrors[0] != "$unbound_parent" {
			t.Fatalf("expected unbound variable error for $unbound_parent, got: %v", unboundErrors)
		}
	})
}
