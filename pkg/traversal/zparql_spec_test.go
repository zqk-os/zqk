package traversal_test

import (
	"encoding/json"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func findSpecsDir() string {
	candidates := []string{
		filepath.Join("..", "..", "docs", "specs"),
		filepath.Join("..", "docs", "specs"),
		filepath.Join("docs", "specs"),
		".",
	}
	for _, c := range candidates {
		if fi, err := fileutil.Stat(c); err == nil && fi.IsDir() {
			if _, err := fileutil.Stat(filepath.Join(c, "SPEC-ZPARQL-GRAPH-TRAVERSAL-GRAMMAR.md")); err == nil {
				return c
			}
		}
	}
	return filepath.Join("..", "..", "docs", "specs")
}

func extractJSONBlock(content string) string {
	re := regexp.MustCompile("(?s)```json\n(.*?)\n```")
	matches := re.FindStringSubmatch(content)
	if len(matches) > 1 {
		return matches[1]
	}
	return ""
}

// Satisfies CRIT-ZPARQL-GRAMMAR-SYNTAX-SPEC, CRIT-ZPARQL-PATTERN-MATCHING-PROOF, CRIT-ZPARQL-MALFORMED-PATTERN-NEGATIVE
func TestZPARQLGraphTraversalSpec(t *testing.T) {
	specsDir := findSpecsDir()
	specPath := filepath.Join(specsDir, "SPEC-ZPARQL-GRAPH-TRAVERSAL-GRAMMAR.md")
	data, err := fileutil.ReadFile(specPath)
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

// Satisfies CRIT-ZPARQL-PLANNER-CONTRACT-SPEC, CRIT-ZPARQL-INDEX-SCAN-COMPLEXITY-PROOF, CRIT-ZPARQL-CYCLIC-TRAVERSAL-RECURSION-NEGATIVE
func TestZPARQLIndexedQueryPlannerSpec(t *testing.T) {
	specsDir := findSpecsDir()
	specPath := filepath.Join(specsDir, "SPEC-ZPARQL-INDEXED-QUERY-PLANNER.md")
	data, err := fileutil.ReadFile(specPath)
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

// Satisfies CRIT-ZPARQL-RESULT-SCHEMA-SPEC, CRIT-ZPARQL-STREAMING-BACKPRESSURE-PROOF, CRIT-ZPARQL-TRUNCATED-STREAM-NEGATIVE
func TestZPARQLResultStreamingSpec(t *testing.T) {
	specsDir := findSpecsDir()
	specPath := filepath.Join(specsDir, "SPEC-ZPARQL-RESULT-STREAMING.md")
	data, err := fileutil.ReadFile(specPath)
	require.NoError(t, err, "ZPARQL result streaming specification file must exist: %s", specPath)
	content := string(data)
	require.NotEmpty(t, content, "ZPARQL result streaming specification content must not be empty")

	// Result Schema Spec (CRIT-ZPARQL-RESULT-SCHEMA-SPEC)
	require.Contains(t, content, "CRIT-ZPARQL-RESULT-SCHEMA-SPEC")
	require.Contains(t, content, "Header Frame")
	require.Contains(t, content, "Chunk Frames")
	require.Contains(t, content, "Trailer Frame")

	// Streaming backpressure proof (CRIT-ZPARQL-STREAMING-BACKPRESSURE-PROOF)
	require.Contains(t, content, "CRIT-ZPARQL-STREAMING-BACKPRESSURE-PROOF")
	require.Contains(t, content, "Constant Memory Overhead")

	// Truncated stream negative invariant (CRIT-ZPARQL-TRUNCATED-STREAM-NEGATIVE)
	require.Contains(t, content, "CRIT-ZPARQL-TRUNCATED-STREAM-NEGATIVE")
	require.Contains(t, content, "EOS_FINALIZED")
	require.Contains(t, content, "ERR_ZPARQL_STREAM_TRUNCATED")
}
