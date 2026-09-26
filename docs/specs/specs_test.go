package specs_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"
)

func findSpecsDir() string {
	candidates := []string{
		filepath.Join("docs", "specs"),
		filepath.Join("..", "docs", "specs"),
		filepath.Join("..", "..", "docs", "specs"),
		".",
	}
	for _, c := range candidates {
		if fi, err := os.Stat(c); err == nil && fi.IsDir() {
			if _, err := os.Stat(filepath.Join(c, "SPEC-ZQL-DECLARATIVE-MUTATION-GRAMMAR.md")); err == nil {
				return c
			}
		}
	}
	return "docs/specs"
}

func extractJSONBlock(content string) string {
	re := regexp.MustCompile("(?s)```json\n(.*?)\n```")
	matches := re.FindStringSubmatch(content)
	if len(matches) > 1 {
		return matches[1]
	}
	return ""
}

func TestZQLDeclarativeMutationSpec(t *testing.T) {
	specsDir := findSpecsDir()
	specPath := filepath.Join(specsDir, "SPEC-ZQL-DECLARATIVE-MUTATION-GRAMMAR.md")
	data, err := os.ReadFile(specPath)
	require.NoError(t, err, "ZQL specification file must exist: %s", specPath)
	content := string(data)
	require.NotEmpty(t, content, "ZQL specification content must not be empty")

	// Static Grammar verification (CRIT-ZQL-GRAMMAR-STATIC-SPEC)
	require.Contains(t, content, "ISO/IEC 14977 Formal EBNF Grammar")
	require.Contains(t, content, "BEGIN")
	require.Contains(t, content, "TRANSACTION")
	require.Contains(t, content, "LET")
	require.Contains(t, content, "UPSERT")
	require.Contains(t, content, "DELETE")
	require.Contains(t, content, "COMMIT")
	require.Contains(t, content, "ROLLBACK")

	// JSON Schema AST verification (CRIT-ZQL-GRAMMAR-STATIC-SPEC)
	jsonBlock := extractJSONBlock(content)
	require.NotEmpty(t, jsonBlock, "ZQL specification must contain an AST JSON Schema code block")
	var schemaMap map[string]any
	err = json.Unmarshal([]byte(jsonBlock), &schemaMap)
	require.NoError(t, err, "ZQL AST schema block must be valid JSON")
	require.Equal(t, "https://json-schema.org/draft/2020-12/schema", schemaMap["$schema"])

	// Variable Resolution verification (CRIT-ZQL-AST-VARIABLE-RESOLUTION)
	require.Contains(t, content, "Kahn's algorithm")
	require.Contains(t, content, "Deterministic Variable Binding")

	// Negative Invariant verification (CRIT-ZQL-UNBOUND-VARIABLE-NEGATIVE)
	require.Contains(t, content, "ERR_ZQL_CIRCULAR_DEPENDENCY")
	require.Contains(t, content, "ERR_ZQL_UNBOUND_VARIABLE")
	require.Contains(t, content, "ERR_ZQL_SYNTAX_ERROR")
}

func TestZPARQLGraphTraversalSpec(t *testing.T) {
	specsDir := findSpecsDir()
	specPath := filepath.Join(specsDir, "SPEC-ZPARQL-GRAPH-TRAVERSAL-GRAMMAR.md")
	data, err := os.ReadFile(specPath)
	require.NoError(t, err, "ZPARQL specification file must exist: %s", specPath)
	content := string(data)
	require.NotEmpty(t, content, "ZPARQL specification content must not be empty")

	// Static Grammar verification (CRIT-ZPARQL-GRAMMAR-SYNTAX-SPEC)
	require.Contains(t, content, "ISO/IEC 14977 Formal EBNF Grammar")
	require.Contains(t, content, "MATCH")
	require.Contains(t, content, "node_pattern")
	require.Contains(t, content, "edge_pattern")
	require.Contains(t, content, "WHERE")
	require.Contains(t, content, "RETURN")

	// JSON Schema AST verification (CRIT-ZPARQL-GRAMMAR-SYNTAX-SPEC)
	jsonBlock := extractJSONBlock(content)
	require.NotEmpty(t, jsonBlock, "ZPARQL specification must contain an AST JSON Schema code block")
	var schemaMap map[string]any
	err = json.Unmarshal([]byte(jsonBlock), &schemaMap)
	require.NoError(t, err, "ZPARQL AST schema block must be valid JSON")
	require.Equal(t, "https://json-schema.org/draft/2020-12/schema", schemaMap["$schema"])

	// Relational Subgraph Algebra verification (CRIT-ZPARQL-PATTERN-MATCHING-PROOF)
	require.Contains(t, content, "Relational Subgraph Algebra")
	require.Contains(t, content, "Cycle Safety")

	// Negative Invariant verification (CRIT-ZPARQL-MALFORMED-PATTERN-NEGATIVE)
	require.Contains(t, content, "ERR_ZPARQL_DISCONNECTED_VARIABLE")
	require.Contains(t, content, "ERR_ZPARQL_SYNTAX_ERROR")
	require.Contains(t, content, "ERR_ZPARQL_INVALID_EDGE_TYPE")
}

func TestZQLTransactionExecutionSpec(t *testing.T) {
	specsDir := findSpecsDir()
	specPath := filepath.Join(specsDir, "SPEC-ZQL-TRANSACTION-EXECUTION.md")
	data, err := os.ReadFile(specPath)
	require.NoError(t, err, "ZQL transaction execution specification file must exist: %s", specPath)
	content := string(data)
	require.NotEmpty(t, content, "ZQL transaction execution specification content must not be empty")

	// State machine lifecycle verification (CRIT-ZQL-TXN-ISOLATION-SPEC)
	require.Contains(t, content, "BEGIN")
	require.Contains(t, content, "STAGE")
	require.Contains(t, content, "PRE_CHECK")
	require.Contains(t, content, "COMMIT")
	require.Contains(t, content, "ROLLBACK")

	// Isolation modes verification (CRIT-ZQL-TXN-ISOLATION-SPEC)
	require.Contains(t, content, "all_or_nothing")
	require.Contains(t, content, "partial_commit")
	require.Contains(t, content, "dry_run")

	// Atomic rollback verification (CRIT-ZQL-ALL-OR-NOTHING-ROLLBACK-PROOF)
	require.Contains(t, content, "CRIT-ZQL-ALL-OR-NOTHING-ROLLBACK-PROOF")
	require.Contains(t, content, "Zero Orphan Guarantee")

	// Write isolation and dirty-read concurrency verification (CRIT-ZQL-DIRTY-READ-CONCURRENCY-NEGATIVE)
	require.Contains(t, content, "CRIT-ZQL-DIRTY-READ-CONCURRENCY-NEGATIVE")
	require.Contains(t, content, "Zero Dirty-Read Invariant")
}

func TestZQLPreflightValidationSpec(t *testing.T) {
	specsDir := findSpecsDir()
	specPath := filepath.Join(specsDir, "SPEC-ZQL-PREFLIGHT-VALIDATION.md")
	data, err := os.ReadFile(specPath)
	require.NoError(t, err, "ZQL preflight validation specification file must exist: %s", specPath)
	content := string(data)
	require.NotEmpty(t, content, "ZQL preflight validation specification content must not be empty")

	// Semantic Validator Contract (CRIT-ZQL-INMEMORY-VALIDATION-CONTRACT)
	require.Contains(t, content, "PreflightValidator")
	require.Contains(t, content, "Zero Disk I/O Invariant")
	require.Contains(t, content, "CRIT-ZQL-INMEMORY-VALIDATION-CONTRACT")

	// Diagnostic Receipt Protocol (CRIT-ZQL-PREFLIGHT-DIAGNOSTIC-RECEIPT)
	require.Contains(t, content, "Preflight Diagnostic Receipt")
	require.Contains(t, content, "SchemaViolation")
	require.Contains(t, content, "field_path")
	require.Contains(t, content, "failing_constraint")
	require.Contains(t, content, "remediation")

	// Fail-closed negative invariant (CRIT-ZQL-SCHEMA-CORRUPTION-FAILCLOSED-NEGATIVE)
	require.Contains(t, content, "rejected_failclosed")
	require.Contains(t, content, "CRIT-ZQL-SCHEMA-CORRUPTION-FAILCLOSED-NEGATIVE")
	require.Contains(t, content, "Fail-Closed Boundary")
}

func TestZPARQLIndexedQueryPlannerSpec(t *testing.T) {
	specsDir := findSpecsDir()
	specPath := filepath.Join(specsDir, "SPEC-ZPARQL-INDEXED-QUERY-PLANNER.md")
	data, err := os.ReadFile(specPath)
	require.NoError(t, err, "ZPARQL query planner specification file must exist: %s", specPath)
	content := string(data)
	require.NotEmpty(t, content, "ZPARQL query planner specification content must not be empty")

	// Planner execution contract (CRIT-ZPARQL-PLANNER-CONTRACT-SPEC)
	require.Contains(t, content, "CRIT-ZPARQL-PLANNER-CONTRACT-SPEC")
	require.Contains(t, content, "IndexSeek")
	require.Contains(t, content, "KindScan")
	require.Contains(t, content, "Predicate Pushdown")

	// Complexity bound verification (CRIT-ZPARQL-INDEX-SCAN-COMPLEXITY-PROOF)
	require.Contains(t, content, "CRIT-ZPARQL-INDEX-SCAN-COMPLEXITY-PROOF")
	require.Contains(t, content, "O(K)")

	// Cycle safety and depth bound (CRIT-ZPARQL-CYCLIC-TRAVERSAL-RECURSION-NEGATIVE)
	require.Contains(t, content, "CRIT-ZPARQL-CYCLIC-TRAVERSAL-RECURSION-NEGATIVE")
	require.Contains(t, content, "VisitedSet")
	require.Contains(t, content, "ERR_ZPARQL_CYCLIC_RECURSION_LIMIT")
}



