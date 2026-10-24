package traversal

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCycle152_ASTExpressions verifies Expr interface implementation invariants on all AST node types.
func TestCycle152_ASTExpressions(t *testing.T) {
	comp := ComparisonExpr{
		Property: PropertyRef{Variable: "n", Field: "status"},
		Op:       OpEq,
		Value:    "active",
	}
	comp.isExpr()

	log := LogicalExpr{
		Op:    "AND",
		Left:  comp,
		Right: comp,
	}
	log.isExpr()

	not := NotExpr{Inner: comp}
	not.isExpr()

	ex := ExistsExpr{
		Pattern: PathPattern{
			Nodes: []NodePattern{{Variable: "a"}},
		},
	}
	ex.isExpr()

	var expr Expr = comp
	require.NotNil(t, expr)
	expr = log
	require.NotNil(t, expr)
	expr = not
	require.NotNil(t, expr)
	expr = ex
	require.NotNil(t, expr)
}

// TestCycle152_GraphIndexExtended verifies in-edge indexing, total node enumerations, and edge lookups.
func TestCycle152_GraphIndexExtended(t *testing.T) {
	idx := NewGraphIndex()
	require.Equal(t, 0, idx.TotalNodes())
	require.Empty(t, idx.GetAllNodeIDs())

	idx.AddNode("node-1", "goal", map[string]any{"title": "Bootstrap"})
	idx.AddNode("node-2", "task", map[string]any{"title": "Impl"})
	idx.AddNode("node-3", "task", map[string]any{"title": "Verify"})

	idx.AddEdge("node-1", "child_of", "node-2")
	idx.AddEdge("node-1", "child_of", "node-3")

	require.Equal(t, 3, idx.TotalNodes())
	allIDs := idx.GetAllNodeIDs()
	require.Len(t, allIDs, 3)
	assert.Contains(t, allIDs, "node-1")
	assert.Contains(t, allIDs, "node-2")
	assert.Contains(t, allIDs, "node-3")

	inEdges := idx.GetInEdges("node-2", "child_of")
	require.Equal(t, []string{"node-1"}, inEdges)

	noIn := idx.GetInEdges("node-1", "child_of")
	require.Nil(t, noIn)

	noOut := idx.GetOutEdges("node-2", "unknown_rel")
	require.Nil(t, noOut)
}

// TestCycle152_LexerTokensAndComments verifies comment stripping, arrows, delimiters, and operators.
func TestCycle152_LexerTokensAndComments(t *testing.T) {
	input := `
		// Single line slash comment
		-- Single line dash comment
		/* Multi-line
		   block comment */
		<- <= < -> - -42 12.34 .. . != = == >= >
		( ) [ ] { } : , ; *
		"hello \n\t\"world\"" 'single \'quote\''
		true false null
	`
	lexer := NewLexer(input)
	var tokens []Token
	for {
		tok, err := lexer.NextToken()
		require.NoError(t, err)
		if tok.Type == TokenEOF {
			break
		}
		tokens = append(tokens, tok)
	}

	expectedTypes := []TokenType{
		TokenArrowL, TokenLte, TokenLt, TokenArrowR, TokenMinus, TokenNumber, TokenNumber,
		TokenDotDot, TokenDot, TokenNotEq, TokenEq, TokenEq, TokenGte, TokenGt,
		TokenLParen, TokenRParen, TokenLBracket, TokenRBracket, TokenLBrace, TokenRBrace,
		TokenColon, TokenComma, TokenSemicolon, TokenStar,
		TokenString, TokenString,
		TokenBool, TokenBool, TokenNull,
	}

	require.Len(t, tokens, len(expectedTypes))
	for i, expected := range expectedTypes {
		assert.Equal(t, expected, tokens[i].Type, "mismatch at token index %d", i)
	}
}

// TestCycle152_LexerEscapeAndErrors tests unclosed string literals and unexpected characters in lexer.
func TestCycle152_LexerEscapeAndErrors(t *testing.T) {
	unclosed := `"unclosed string literal without end`
	lex1 := NewLexer(unclosed)
	_, err := lex1.NextToken()
	require.Error(t, err)
	assert.Contains(t, err.Error(), ErrCodeZPARQLSyntaxError)

	unexpected := `@unexpected_symbol`
	lex2 := NewLexer(unexpected)
	_, err = lex2.NextToken()
	require.Error(t, err)
	assert.Contains(t, err.Error(), ErrCodeZPARQLSyntaxError)
}

// TestCycle152_ParserPredicatesAndLiterals tests WHERE expressions with NOT, parentheses, and literals.
func TestCycle152_ParserPredicatesAndLiterals(t *testing.T) {
	query := `
		MATCH (n:Task)
		WHERE NOT (n.status = "done" OR n.priority < 2)
		  AND n.active = true
		  AND n.score >= 90.5
		  AND n.tags IN ["p0", "blocker"]
		  AND n.meta = null
		RETURN n.title AS title
	`
	ast, err := ParseZPARQL(query)
	require.NoError(t, err)
	require.NotNil(t, ast)
	assert.Equal(t, 1, len(ast.Returns))
	assert.Equal(t, "title", ast.Returns[0].Alias)
	assert.NotNil(t, ast.Where)
}

// TestCycle152_ParserEdgesAndProjections tests undirected edges, quantifiers, COUNT, COLLECT, ORDER BY.
func TestCycle152_ParserEdgesAndProjections(t *testing.T) {
	query := `
		MATCH (a:Goal)-[r:rel*1..3]-(b:Task)
		WHERE a.title CONTAINS "Kernel"
		RETURN DISTINCT COUNT(b.id) AS total, COLLECT(b.title) AS titles
		ORDER BY total DESC, titles ASC
		LIMIT 10 OFFSET 5
	`
	ast, err := ParseZPARQL(query)
	require.NoError(t, err)
	require.NotNil(t, ast)
	require.Len(t, ast.Patterns, 1)
	assert.Equal(t, DirectionUndirected, ast.Patterns[0].Edges[0].Direction)
	assert.Equal(t, 1, ast.Patterns[0].Edges[0].MinDepth)
	assert.Equal(t, 3, ast.Patterns[0].Edges[0].MaxDepth)
	assert.Equal(t, 10, ast.Limit)
	assert.Equal(t, 5, ast.Offset)
	require.Len(t, ast.OrderBy, 2)
	assert.Equal(t, "DESC", ast.OrderBy[0].Direction)
	assert.Equal(t, "ASC", ast.OrderBy[1].Direction)
}

// TestCycle152_ParserErrorsAndEdgeCases tests parser syntax errors and malformed queries.
func TestCycle152_ParserErrorsAndEdgeCases(t *testing.T) {
	tests := []struct {
		name      string
		query     string
		errSubstr string
	}{
		{
			name:      "Missing RETURN",
			query:     `MATCH (n:Goal)`,
			errSubstr: "expected RETURN clause",
		},
		{
			name:      "Invalid Comparison Op",
			query:     `MATCH (n:Goal) WHERE n.val @ 5 RETURN n`,
			errSubstr: ErrCodeZPARQLSyntaxError,
		},
		{
			name:      "Invalid Literal in IN clause",
			query:     `MATCH (n:Goal) WHERE n.val IN [1, @] RETURN n`,
			errSubstr: ErrCodeZPARQLSyntaxError,
		},
		{
			name:      "Unclosed list literal",
			query:     `MATCH (n:Goal) WHERE n.val IN [1, 2 RETURN n`,
			errSubstr: ErrCodeZPARQLSyntaxError,
		},
		{
			name:      "Invalid edge arrow token",
			query:     `MATCH (a)-[r:rel]?(b) RETURN a`,
			errSubstr: ErrCodeZPARQLSyntaxError,
		},
		{
			name:      "Malformed node property filter",
			query:     `MATCH (a:Goal {key}) RETURN a`,
			errSubstr: ErrCodeZPARQLSyntaxError,
		},
		{
			name:      "Missing comma in node property filter",
			query:     `MATCH (a:Goal {k1: 1 k2: 2}) RETURN a`,
			errSubstr: ErrCodeZPARQLSyntaxError,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseZPARQL(tc.query)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.errSubstr)
		})
	}
}

// TestCycle152_ExecutorHeaderAndSlicing verifies formatHeaderName, buildHeaders, and sliceRows.
func TestCycle152_ExecutorHeaderAndSlicing(t *testing.T) {
	items := []ProjectionItem{
		{Alias: "customAlias"},
		{Variable: "task", Property: "title"},
		{Variable: "goal"},
		{Expression: "COUNT(*)"},
	}

	headers := buildHeaders(items)
	require.Equal(t, []string{"customAlias", "task.title", "goal", "COUNT(*)"}, headers)

	rows := []map[string]any{
		{"id": 1},
		{"id": 2},
		{"id": 3},
		{"id": 4},
		{"id": 5},
	}

	sliced1 := sliceRows(rows, 10, 2)
	assert.Empty(t, sliced1)

	sliced2 := sliceRows(rows, 1, 2)
	assert.Len(t, sliced2, 2)
	assert.Equal(t, 2, sliced2[0]["id"])
	assert.Equal(t, 3, sliced2[1]["id"])

	sliced3 := sliceRows(rows, 3, 0)
	assert.Len(t, sliced3, 2)
	assert.Equal(t, 4, sliced3[0]["id"])
}

// TestCycle152_ExecutorComparisonsAndValues exercises compareValues, compareNumeric, toFloat, valuesEqual.
func TestCycle152_ExecutorComparisonsAndValues(t *testing.T) {
	eq, err := compareValues(10, OpEq, 10)
	require.NoError(t, err)
	assert.True(t, eq)

	neq, err := compareValues("alpha", OpNotEq, "beta")
	require.NoError(t, err)
	assert.True(t, neq)

	cont, err := compareValues("knowledge kernel", OpContains, "edge")
	require.NoError(t, err)
	assert.True(t, cont)

	inSlice, err := compareValues(2, OpIn, []any{1, 2, 3})
	require.NoError(t, err)
	assert.True(t, inSlice)

	inStrSlice, err := compareValues("done", OpIn, []string{"todo", "done"})
	require.NoError(t, err)
	assert.True(t, inStrSlice)

	_, err = compareValues(5, OpIn, "invalid_non_slice")
	require.Error(t, err)

	ltNum, err := compareValues(int64(5), OpLt, float64(10.5))
	require.NoError(t, err)
	assert.True(t, ltNum)

	lteNum, err := compareValues(float32(10), OpLte, 10)
	require.NoError(t, err)
	assert.True(t, lteNum)

	gtNum, err := compareValues("25", OpGt, "15")
	require.NoError(t, err)
	assert.True(t, gtNum)

	gteNum, err := compareValues("15", OpGte, "15")
	require.NoError(t, err)
	assert.True(t, gteNum)

	strComp, err := compareNumeric("beta", OpGt, "alpha")
	require.NoError(t, err)
	assert.True(t, strComp)

	assert.True(t, valuesEqual(nil, nil))
	assert.False(t, valuesEqual(nil, "something"))
	assert.False(t, valuesEqual("something", nil))
}

// TestCycle152_ExecutorAggregationAndDeduplication tests aggregate projection and distinct deduplication.
func TestCycle152_ExecutorAggregationAndDeduplication(t *testing.T) {
	idx := NewGraphIndex()
	idx.AddNode("t1", "task", map[string]any{"status": "done", "cost": 10})
	idx.AddNode("t2", "task", map[string]any{"status": "done", "cost": 20})
	idx.AddNode("t3", "task", map[string]any{"status": "todo", "cost": 30})

	exec := NewQueryExecutor(idx)
	bindings := []map[string]string{
		{"t": "t1"},
		{"t": "t2"},
		{"t": "t3"},
	}

	returns := []ProjectionItem{
		{Aggregate: "COUNT", Expression: "*"},
		{Aggregate: "COUNT", Variable: "t", Property: "cost"},
		{Aggregate: "COLLECT", Variable: "t", Property: "status"},
		{Aggregate: "UNKNOWN", Variable: "t", Property: "kind"},
	}
	headers := buildHeaders(returns)

	aggRows := exec.projectAggregates(bindings, returns, headers)
	require.Len(t, aggRows, 1)
	assert.Equal(t, 3, aggRows[0][headers[0]])
	assert.Equal(t, 3, aggRows[0][headers[1]])
	assert.Equal(t, []any{"done", "done", "todo"}, aggRows[0][headers[2]])
	assert.Equal(t, "task", aggRows[0][headers[3]])

	rowsToDedup := []map[string]any{
		{"kind": "task", "status": "done"},
		{"kind": "task", "status": "done"},
		{"kind": "task", "status": "todo"},
	}
	unique := deduplicateRows(rowsToDedup, []string{"kind", "status"})
	require.Len(t, unique, 2)
}

// TestCycle152_ExecutorSortingAndOrdering tests ascending, descending, and nil sorting behavior.
func TestCycle152_ExecutorSortingAndOrdering(t *testing.T) {
	exec := NewQueryExecutor(NewGraphIndex())
	rows := []map[string]any{
		{"name": "Charlie", "score": 80},
		{"name": "Alice", "score": nil},
		{"name": "Bob", "score": 95},
		{"name": "David", "score": 80},
	}

	orderBy := []OrderByItem{
		{Property: "score", Direction: "ASC"},
		{Property: "name", Direction: "ASC"},
	}
	headers := []string{"name", "score"}

	exec.sortRows(rows, orderBy, headers)
	require.Len(t, rows, 4)
	assert.Equal(t, "Alice", rows[0]["name"])
	assert.Equal(t, "Charlie", rows[1]["name"])
	assert.Equal(t, "David", rows[2]["name"])
	assert.Equal(t, "Bob", rows[3]["name"])

	orderByDesc := []OrderByItem{
		{Property: "score", Direction: "DESC"},
	}
	exec.sortRows(rows, orderByDesc, headers)
	assert.Equal(t, "Bob", rows[0]["name"])
}

// TestCycle152_ExecutorTraversalAndMatching tests candidate finding, nodeMatches, and traverseEdge directions.
func TestCycle152_ExecutorTraversalAndMatching(t *testing.T) {
	idx := NewGraphIndex()
	idx.AddNode("g1", "goal", map[string]any{"archived": false})
	idx.AddNode("t1", "task", map[string]any{"priority": "high"})
	idx.AddNode("t2", "task", map[string]any{"priority": "low"})
	idx.AddEdge("g1", "contains", "t1")
	idx.AddEdge("t1", "depends_on", "t2")

	exec := NewQueryExecutor(idx)

	candidates := exec.findCandidateNodes(NodePattern{Kind: "task", Filters: map[string]any{"priority": "high"}})
	require.Equal(t, []string{"t1"}, candidates)

	allCandidates := exec.findCandidateNodes(NodePattern{})
	require.Len(t, allCandidates, 3)

	assert.False(t, exec.nodeMatches("non_existent", NodePattern{Kind: "task"}))
	assert.False(t, exec.nodeMatches("g1", NodePattern{Kind: "task"}))
	assert.True(t, exec.nodeMatches("g1", NodePattern{Kind: "goal", Filters: map[string]any{"archived": false}}))
	assert.False(t, exec.nodeMatches("g1", NodePattern{Kind: "goal", Filters: map[string]any{"archived": true}}))

	outEdges := exec.traverseEdge("g1", EdgePattern{EdgeType: "contains", Direction: DirectionOutgoing, MinDepth: 1, MaxDepth: 1})
	require.Equal(t, []string{"t1"}, outEdges)

	inEdges := exec.traverseEdge("t1", EdgePattern{EdgeType: "contains", Direction: DirectionIncoming, MinDepth: 1, MaxDepth: 1})
	require.Equal(t, []string{"g1"}, inEdges)

	undirEdges := exec.traverseEdge("t1", EdgePattern{EdgeType: "contains", Direction: DirectionUndirected, MinDepth: 1, MaxDepth: 1})
	require.Equal(t, []string{"g1"}, undirEdges)
}

// TestCycle152_ExecutorExpressionsAndJoins verifies logical operators, not expressions, and hash join permutations.
func TestCycle152_ExecutorExpressionsAndJoins(t *testing.T) {
	idx := NewGraphIndex()
	idx.AddNode("g1", "goal", map[string]any{"status": "active", "points": 10})
	exec := NewQueryExecutor(idx)

	bindings := map[string]string{"g": "g1"}

	matched, err := exec.evaluateExpr(bindings, LogicalExpr{
		Op:    "OR",
		Left:  ComparisonExpr{Property: PropertyRef{Variable: "g", Field: "status"}, Op: OpEq, Value: "active"},
		Right: ComparisonExpr{Property: PropertyRef{Variable: "g", Field: "points"}, Op: OpEq, Value: 99},
	})
	require.NoError(t, err)
	assert.True(t, matched)

	_, err = exec.evaluateExpr(bindings, LogicalExpr{Op: "XOR"})
	require.Error(t, err)

	notMatched, err := exec.evaluateExpr(bindings, NotExpr{
		Inner: ComparisonExpr{Property: PropertyRef{Variable: "g", Field: "status"}, Op: OpEq, Value: "closed"},
	})
	require.NoError(t, err)
	assert.True(t, notMatched)

	setA := []map[string]string{{"id": "1", "val": "a"}, {"id": "2", "val": "b"}}
	setB := []map[string]string{{"id": "2", "extra": "x"}}
	joined := joinBindings(setA, setB)
	require.Len(t, joined, 1)
	assert.Equal(t, "b", joined[0]["val"])
	assert.Equal(t, "x", joined[0]["extra"])

	joinedSwapped := joinBindings(setB, setA)
	require.Len(t, joinedSwapped, 1)

	crossA := []map[string]string{{"foo": "bar"}}
	crossB := []map[string]string{{"baz": "qux"}}
	cartesian := joinBindings(crossA, crossB)
	require.Len(t, cartesian, 1)
	assert.Equal(t, "bar", cartesian[0]["foo"])
	assert.Equal(t, "qux", cartesian[0]["baz"])

	require.Nil(t, joinBindings(nil, setB))
	require.Nil(t, joinBindings(setA, nil))
}

// TestCycle152_ExecutorContextCancellation tests fail-closed cancellation in executor queries.
func TestCycle152_ExecutorContextCancellation(t *testing.T) {
	idx := NewGraphIndex()
	idx.AddNode("n1", "task", nil)
	exec := NewQueryExecutor(idx)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	ast := &QueryAST{
		Patterns: []PathPattern{
			{Nodes: []NodePattern{{Variable: "n", Kind: "task"}}},
		},
		Returns: []ProjectionItem{{Variable: "n"}},
	}
	_, err := exec.Execute(ctx, ast)
	require.Error(t, err)
	assert.True(t, errors.Is(err, context.Canceled))

	_, err = exec.Execute(context.Background(), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nil QueryAST")
}

// TestCycle152_QueryPlannerBranching tests planner index seek vs kind scan and empty query validation.
func TestCycle152_QueryPlannerBranching(t *testing.T) {
	idx := NewGraphIndex()
	planner := NewQueryPlanner(idx)

	plan1, err := planner.Plan(QuerySpec{
		StartID:      "node-1",
		Relation:     "child_of",
		FieldFilters: map[string]any{"status": "active"},
	})
	require.NoError(t, err)
	assert.True(t, plan1.IndexSeekUsed)
	assert.True(t, plan1.PushdownDone)
	assert.Equal(t, 3, plan1.TotalCost)

	plan2, err := planner.Plan(QuerySpec{
		TargetKind: "task",
	})
	require.NoError(t, err)
	assert.False(t, plan2.IndexSeekUsed)
	assert.Equal(t, 5, plan2.TotalCost)

	_, err = planner.Plan(QuerySpec{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "query spec must specify StartID or TargetKind")
}

// TestCycle152_StreamingProducerEdgeCases tests chunk and buffer capacity normalization and cancellation.
func TestCycle152_StreamingProducerEdgeCases(t *testing.T) {
	producer := NewStreamProducer()
	header := HeaderFrame{
		Columns: []string{"id", "title"},
	}
	records := []map[string]any{
		{"id": 1, "title": "alpha"},
		{"id": 2, "title": "beta"},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	ch := producer.Stream(ctx, header, records, 0, 0)
	for range ch {
		// Drain if any
	}

	validCtx := context.Background()
	chValid := producer.Stream(validCtx, header, records, 1, 1)
	consumer := NewStreamConsumer()
	result, err := consumer.Consume(validCtx, chValid)
	require.NoError(t, err)
	require.True(t, result.Finalized)
	assert.Equal(t, 2, result.TotalRecords)
}

// TestCycle152_StreamingConsumerFailures exercises fail-closed stream consumer error branches.
func TestCycle152_StreamingConsumerFailures(t *testing.T) {
	consumer := NewStreamConsumer()

	t.Run("Nil Header Frame", func(t *testing.T) {
		ch := make(chan StreamFrame, 1)
		ch <- StreamFrame{Type: FrameTypeHeader, Header: nil}
		close(ch)
		_, err := consumer.Consume(context.Background(), ch)
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrStreamTruncated))
	})

	t.Run("Chunk Before Header", func(t *testing.T) {
		ch := make(chan StreamFrame, 1)
		ch <- StreamFrame{Type: FrameTypeChunk, Chunk: &ChunkFrame{Sequence: 1}}
		close(ch)
		_, err := consumer.Consume(context.Background(), ch)
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrStreamTruncated))
	})

	t.Run("Nil Chunk Frame", func(t *testing.T) {
		ch := make(chan StreamFrame, 2)
		ch <- StreamFrame{Type: FrameTypeHeader, Header: &HeaderFrame{}}
		ch <- StreamFrame{Type: FrameTypeChunk, Chunk: nil}
		close(ch)
		_, err := consumer.Consume(context.Background(), ch)
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrStreamTruncated))
	})

	t.Run("Sequence Gap in Chunks", func(t *testing.T) {
		ch := make(chan StreamFrame, 2)
		ch <- StreamFrame{Type: FrameTypeHeader, Header: &HeaderFrame{}}
		ch <- StreamFrame{Type: FrameTypeChunk, Chunk: &ChunkFrame{Sequence: 2}}
		close(ch)
		_, err := consumer.Consume(context.Background(), ch)
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrStreamSequenceGap))
	})

	t.Run("Nil Trailer Frame", func(t *testing.T) {
		ch := make(chan StreamFrame, 2)
		ch <- StreamFrame{Type: FrameTypeHeader, Header: &HeaderFrame{}}
		ch <- StreamFrame{Type: FrameTypeTrailer, Trailer: nil}
		close(ch)
		_, err := consumer.Consume(context.Background(), ch)
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrStreamTruncated))
	})

	t.Run("Unknown Frame Type", func(t *testing.T) {
		ch := make(chan StreamFrame, 1)
		ch <- StreamFrame{Type: FrameType("UNKNOWN_FRAME")}
		close(ch)
		_, err := consumer.Consume(context.Background(), ch)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown frame type")
	})

	t.Run("Context Cancelled during Consume", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		ch := make(chan StreamFrame)
		_, err := consumer.Consume(ctx, ch)
		require.Error(t, err)
		assert.True(t, errors.Is(err, context.Canceled))
	})
}

type dummyExpr struct{}

func (dummyExpr) isExpr() {}

// TestCycle152_ExecutorDeepExpressionsAndEdgeCases exercises ExistsExpr, missing variables/nodes, and string comparators.
func TestCycle152_ExecutorDeepExpressionsAndEdgeCases(t *testing.T) {
	idx := NewGraphIndex()
	idx.AddNode("g1", "goal", map[string]any{"status": "active"})
	idx.AddNode("t1", "task", map[string]any{"priority": "high"})
	idx.AddEdge("g1", "contains", "t1")
	exec := NewQueryExecutor(idx)

	// ExistsExpr evaluation
	existsMatch, err := exec.evaluateExpr(map[string]string{"g": "g1"}, ExistsExpr{
		Pattern: PathPattern{
			Nodes: []NodePattern{{Variable: "g", Kind: "goal"}, {Variable: "t", Kind: "task"}},
			Edges: []EdgePattern{{EdgeType: "contains", Direction: DirectionOutgoing, MinDepth: 1, MaxDepth: 1}},
		},
	})
	require.NoError(t, err)
	assert.True(t, existsMatch)

	// Unsupported expression error
	_, err = exec.evaluateExpr(map[string]string{"g": "g1"}, dummyExpr{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported expression type")

	// Missing variable and node checks in comparison
	missingVarMatch, err := exec.evaluateExpr(map[string]string{"other": "g1"}, ComparisonExpr{
		Property: PropertyRef{Variable: "g", Field: "status"},
		Op:       OpEq,
		Value:    "active",
	})
	require.NoError(t, err)
	assert.False(t, missingVarMatch)

	missingNodeMatch, err := exec.evaluateExpr(map[string]string{"g": "missing"}, ComparisonExpr{
		Property: PropertyRef{Variable: "g", Field: "status"},
		Op:       OpEq,
		Value:    "active",
	})
	require.NoError(t, err)
	assert.False(t, missingNodeMatch)

	// Lexicographic comparisons
	ltStr, err := compareNumeric("apple", OpLt, "banana")
	require.NoError(t, err)
	assert.True(t, ltStr)

	lteStr, err := compareNumeric("apple", OpLte, "apple")
	require.NoError(t, err)
	assert.True(t, lteStr)

	gtStr, err := compareNumeric("banana", OpGt, "apple")
	require.NoError(t, err)
	assert.True(t, gtStr)

	gteStr, err := compareNumeric("banana", OpGte, "banana")
	require.NoError(t, err)
	assert.True(t, gteStr)

	// resolveProjectionValue resolution
	require.NotNil(t, exec.resolveProjectionValue(map[string]string{"g": "g1"}, ProjectionItem{Variable: "g"}))
	require.Nil(t, exec.resolveProjectionValue(map[string]string{"x": "g1"}, ProjectionItem{Variable: "g"}))
	require.Nil(t, exec.resolveProjectionValue(map[string]string{"g": "missing"}, ProjectionItem{Variable: "g"}))
}

// TestCycle152_ParserAggregatesMinMax verifies MIN and MAX aggregate parsing and empty aggregate fallbacks.
func TestCycle152_ParserAggregatesMinMax(t *testing.T) {
	ast, err := ParseZPARQL(`
		MATCH (n:Task)
		RETURN MIN(n.score) AS min_s, MAX(n.score) AS max_s
	`)
	require.NoError(t, err)
	require.Len(t, ast.Returns, 2)
	assert.Equal(t, "MIN", ast.Returns[0].Aggregate)
	assert.Equal(t, "MAX", ast.Returns[1].Aggregate)

	// Empty bindings aggregate fallback
	exec := NewQueryExecutor(NewGraphIndex())
	rows := exec.projectAggregates(nil, []ProjectionItem{
		{Aggregate: "CUSTOM", Variable: "n", Property: "score"},
	}, []string{"custom_score"})
	require.Len(t, rows, 1)
	assert.Nil(t, rows[0]["custom_score"])
}
