package traversal

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/zqk-os/zqk/pkg/objects"
)

// QueryResult holds tabular result data from a ZPARQL query execution.
type QueryResult struct {
	Headers []string         `json:"headers"`
	Rows    []map[string]any `json:"rows"`
	Total   int              `json:"total"`
}

// QueryExecutor executes a parsed QueryAST against an in-memory GraphIndex.
type QueryExecutor struct {
	index *GraphIndex
}

// NewQueryExecutor creates an executor over a GraphIndex.
func NewQueryExecutor(idx *GraphIndex) *QueryExecutor {
	return &QueryExecutor{index: idx}
}

func formatHeaderName(ret ProjectionItem) string {
	if ret.Alias != "" {
		return ret.Alias
	}
	if ret.Property != "" {
		return fmt.Sprintf("%s.%s", ret.Variable, ret.Property)
	}
	if ret.Variable != "" {
		return ret.Variable
	}
	return ret.Expression
}

func buildHeaders(returns []ProjectionItem) []string {
	headers := make([]string, 0, len(returns))
	for _, ret := range returns {
		headers = append(headers, formatHeaderName(ret))
	}
	return headers
}

func (e *QueryExecutor) solvePathPatterns(ctx context.Context, patterns []PathPattern) ([]map[string]string, error) {
	var allBindings []map[string]string
	for i, pattern := range patterns {
		patternBindings, err := e.matchPathPattern(ctx, pattern)
		if err != nil {
			return nil, err
		}
		if i == 0 {
			allBindings = patternBindings
		} else {
			allBindings = joinBindings(allBindings, patternBindings)
		}
		if len(allBindings) == 0 {
			break
		}
	}
	return allBindings, nil
}

func (e *QueryExecutor) filterBindings(ctx context.Context, bindings []map[string]string, where Expr) ([]map[string]string, error) {
	filtered := make([]map[string]string, 0, len(bindings))
	for _, b := range bindings {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		if where != nil {
			matched, err := e.evaluateExpr(b, where)
			if err != nil {
				return nil, err
			}
			if !matched {
				continue
			}
		}
		filtered = append(filtered, b)
	}
	return filtered, nil
}

func (e *QueryExecutor) projectRows(filtered []map[string]string, returns []ProjectionItem, headers []string) []map[string]any {
	hasAggregates := false
	for _, ret := range returns {
		if ret.Aggregate != "" {
			hasAggregates = true
			break
		}
	}
	if hasAggregates {
		return e.projectAggregates(filtered, returns, headers)
	}
	outputRows := make([]map[string]any, 0, len(filtered))
	for _, b := range filtered {
		row := make(map[string]any, len(returns))
		for i, ret := range returns {
			row[headers[i]] = e.resolveProjectionValue(b, ret)
		}
		outputRows = append(outputRows, row)
	}
	return outputRows
}

func sliceRows(outputRows []map[string]any, offset, limit int) []map[string]any {
	total := len(outputRows)
	start := offset
	if start > total {
		start = total
	}
	end := total
	if limit > 0 && start+limit < total {
		end = start + limit
	}
	return outputRows[start:end]
}

// Execute parses and runs a ZPARQL query string against the GraphIndex.
func (e *QueryExecutor) Execute(ctx context.Context, ast *QueryAST) (*QueryResult, error) {
	if ast == nil {
		return nil, fmt.Errorf("nil QueryAST")
	}

	allBindings, err := e.solvePathPatterns(ctx, ast.Patterns)
	if err != nil {
		return nil, err
	}

	filteredBindings, err := e.filterBindings(ctx, allBindings, ast.Where)
	if err != nil {
		return nil, err
	}

	headers := buildHeaders(ast.Returns)
	outputRows := e.projectRows(filteredBindings, ast.Returns, headers)

	if len(ast.Returns) > 0 && ast.Returns[0].Distinct {
		outputRows = deduplicateRows(outputRows, headers)
	}

	if len(ast.OrderBy) > 0 {
		e.sortRows(outputRows, ast.OrderBy, headers)
	}

	return &QueryResult{
		Headers: headers,
		Rows:    sliceRows(outputRows, ast.Offset, ast.Limit),
		Total:   len(outputRows),
	}, nil
}

func (e *QueryExecutor) matchPathPattern(ctx context.Context, p PathPattern) ([]map[string]string, error) {
	if len(p.Nodes) == 0 {
		return nil, nil
	}

	// Find candidate nodes for first node pattern
	firstNodePattern := p.Nodes[0]
	candidates := e.findCandidateNodes(firstNodePattern)

	var currentPaths [][]string
	for _, c := range candidates {
		currentPaths = append(currentPaths, []string{c})
	}

	// Traverse edges sequentially
	for i := 0; i < len(p.Edges); i++ {
		edge := p.Edges[i]
		targetPattern := p.Nodes[i+1]
		var nextPaths [][]string

		for _, path := range currentPaths {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
			}
			currID := path[len(path)-1]

			// Traverse edge up to max_depth
			reachable := e.traverseEdge(currID, edge)
			for _, targetID := range reachable {
				// Check target node constraints
				if e.nodeMatches(targetID, targetPattern) {
					newPath := make([]string, len(path)+1)
					copy(newPath, path)
					newPath[len(path)] = targetID
					nextPaths = append(nextPaths, newPath)
				}
			}
		}
		currentPaths = nextPaths
		if len(currentPaths) == 0 {
			break
		}
	}

	// Convert paths to bindings map
	var bindings []map[string]string
	for _, path := range currentPaths {
		b := make(map[string]string)
		for i, n := range p.Nodes {
			if n.Variable != "" {
				b[n.Variable] = path[i]
			}
		}
		bindings = append(bindings, b)
	}

	return bindings, nil
}

func (e *QueryExecutor) findCandidateNodes(pattern NodePattern) []string {
	var candidates []string
	if pattern.Kind != "" {
		candidates = e.index.GetNodesByKind(pattern.Kind)
	} else {
		candidates = e.index.GetAllNodeIDs()
	}

	var matched []string
	for _, id := range candidates {
		if e.nodeMatches(id, pattern) {
			matched = append(matched, id)
		}
	}
	return matched
}

func (e *QueryExecutor) nodeMatches(nodeID string, pattern NodePattern) bool {
	node, exists := e.index.GetNode(nodeID)
	if !exists {
		return false
	}
	if pattern.Kind != "" {
		k := objects.GetString(node, "kind")
		if !strings.EqualFold(k, pattern.Kind) {
			return false
		}
	}
	for k, expected := range pattern.Filters {
		actual, ok := node[k]
		if !ok || !valuesEqual(actual, expected) {
			return false
		}
	}
	return true
}

func (e *QueryExecutor) traverseEdge(startID string, edge EdgePattern) []string {
	results := make(map[string]bool)
	visitedGlobal := make(map[string]bool)

	type step struct {
		id    string
		depth int
	}

	queue := []step{{id: startID, depth: 0}}
	visitedGlobal[startID] = true

	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]

		if curr.depth >= edge.MaxDepth {
			continue
		}

		var neighbors []string
		switch edge.Direction {
		case DirectionOutgoing:
			neighbors = e.index.GetOutEdges(curr.id, edge.EdgeType)
		case DirectionIncoming:
			neighbors = e.index.GetInEdges(curr.id, edge.EdgeType)
		case DirectionUndirected:
			out := e.index.GetOutEdges(curr.id, edge.EdgeType)
			in := e.index.GetInEdges(curr.id, edge.EdgeType)
			combined := make(map[string]bool)
			for _, n := range out {
				combined[n] = true
			}
			for _, n := range in {
				combined[n] = true
			}
			for n := range combined {
				neighbors = append(neighbors, n)
			}
		}

		for _, nID := range neighbors {
			nextDepth := curr.depth + 1
			if nextDepth >= edge.MinDepth && nextDepth <= edge.MaxDepth {
				results[nID] = true
			}
			if !visitedGlobal[nID] && nextDepth < edge.MaxDepth {
				visitedGlobal[nID] = true
				queue = append(queue, step{id: nID, depth: nextDepth})
			}
		}
	}

	out := make([]string, 0, len(results))
	for id := range results {
		out = append(out, id)
	}
	return out
}

func joinBindings(a, b []map[string]string) []map[string]string {
	if len(a) == 0 || len(b) == 0 {
		return nil
	}

	// Discover shared join keys between the two binding sets.
	var commonKeys []string
	firstA, firstB := a[0], b[0]
	for k := range firstA {
		if _, ok := firstB[k]; ok {
			commonKeys = append(commonKeys, k)
		}
	}

	// If no common keys, perform Cartesian product.
	if len(commonKeys) == 0 {
		joined := make([]map[string]string, 0, len(a)*len(b))
		for _, rowA := range a {
			for _, rowB := range b {
				merged := make(map[string]string, len(rowA)+len(rowB))
				for k, v := range rowA {
					merged[k] = v
				}
				for k, v := range rowB {
					merged[k] = v
				}
				joined = append(joined, merged)
			}
		}
		return joined
	}

	// Indexed Hash-Join (F-PERF-002):
	// Build hash index on the smaller binding set, then probe with the larger set.
	buildSet, probeSet := a, b
	if len(a) > len(b) {
		buildSet, probeSet = b, a
	}

	hashKey := func(row map[string]string) string {
		if len(commonKeys) == 1 {
			return row[commonKeys[0]]
		}
		var sb strings.Builder
		for _, k := range commonKeys {
			sb.WriteString(row[k])
			sb.WriteByte(0)
		}
		return sb.String()
	}

	index := make(map[string][]map[string]string, len(buildSet))
	for _, row := range buildSet {
		key := hashKey(row)
		index[key] = append(index[key], row)
	}

	var joined []map[string]string
	for _, probeRow := range probeSet {
		key := hashKey(probeRow)
		matches := index[key]
		for _, buildRow := range matches {
			merged := make(map[string]string, len(buildRow)+len(probeRow))
			for k, v := range buildRow {
				merged[k] = v
			}
			for k, v := range probeRow {
				merged[k] = v
			}
			joined = append(joined, merged)
		}
	}
	return joined
}

func (e *QueryExecutor) evaluateExpr(bindings map[string]string, expr Expr) (bool, error) {
	switch x := expr.(type) {
	case ComparisonExpr:
		nodeID, ok := bindings[x.Property.Variable]
		if !ok {
			return false, nil
		}
		node, ok := e.index.GetNode(nodeID)
		if !ok {
			return false, nil
		}
		actualVal := node[x.Property.Field]
		return compareValues(actualVal, x.Op, x.Value)

	case LogicalExpr:
		if strings.EqualFold(x.Op, "AND") {
			leftMatch, err := e.evaluateExpr(bindings, x.Left)
			if err != nil || !leftMatch {
				return false, err
			}
			return e.evaluateExpr(bindings, x.Right)
		} else if strings.EqualFold(x.Op, "OR") {
			leftMatch, err := e.evaluateExpr(bindings, x.Left)
			if err != nil {
				return false, err
			}
			if leftMatch {
				return true, nil
			}
			return e.evaluateExpr(bindings, x.Right)
		}
		return false, fmt.Errorf("unknown logical operator %s", x.Op)

	case NotExpr:
		innerMatch, err := e.evaluateExpr(bindings, x.Inner)
		if err != nil {
			return false, err
		}
		return !innerMatch, nil

	case ExistsExpr:
		subBindings, err := e.matchPathPattern(context.Background(), x.Pattern)
		if err != nil {
			return false, err
		}
		joined := joinBindings([]map[string]string{bindings}, subBindings)
		return len(joined) > 0, nil
	}

	return false, fmt.Errorf("unsupported expression type %T", expr)
}

func compareValues(actual any, op ComparisonOp, expected any) (bool, error) {
	switch op {
	case OpEq:
		return valuesEqual(actual, expected), nil
	case OpNotEq:
		return !valuesEqual(actual, expected), nil
	case OpContains:
		actualStr := fmt.Sprintf("%v", actual)
		expectedStr := fmt.Sprintf("%v", expected)
		return strings.Contains(actualStr, expectedStr), nil
	case OpIn:
		// Expected should be a slice
		switch expList := expected.(type) {
		case []any:
			for _, item := range expList {
				if valuesEqual(actual, item) {
					return true, nil
				}
			}
			return false, nil
		case []string:
			actStr := fmt.Sprintf("%v", actual)
			for _, item := range expList {
				if actStr == item {
					return true, nil
				}
			}
			return false, nil
		default:
			return false, fmt.Errorf("IN operator requires list literal")
		}
	case OpLt, OpLte, OpGt, OpGte:
		return compareNumeric(actual, op, expected)
	}
	return false, nil
}

func valuesEqual(a, b any) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b)
}

func compareNumeric(a any, op ComparisonOp, b any) (bool, error) {
	aNum, okA := toFloat(a)
	bNum, okB := toFloat(b)
	if !okA || !okB {
		// Fallback to lexicographic string comparison
		aStr := fmt.Sprintf("%v", a)
		bStr := fmt.Sprintf("%v", b)
		switch op {
		case OpLt:
			return aStr < bStr, nil
		case OpLte:
			return aStr <= bStr, nil
		case OpGt:
			return aStr > bStr, nil
		case OpGte:
			return aStr >= bStr, nil
		}
		return false, nil
	}

	switch op {
	case OpLt:
		return aNum < bNum, nil
	case OpLte:
		return aNum <= bNum, nil
	case OpGt:
		return aNum > bNum, nil
	case OpGte:
		return aNum >= bNum, nil
	}
	return false, nil
}

func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case string:
		var f float64
		_, err := fmt.Sscanf(n, "%f", &f)
		return f, err == nil
	}
	return 0, false
}

func (e *QueryExecutor) resolveProjectionValue(b map[string]string, ret ProjectionItem) any {
	nodeID, ok := b[ret.Variable]
	if !ok {
		return nil
	}
	node, exists := e.index.GetNode(nodeID)
	if !exists {
		return nil
	}
	if ret.Property != "" {
		return node[ret.Property]
	}
	return node
}

func (e *QueryExecutor) projectAggregates(bindings []map[string]string, returns []ProjectionItem, headers []string) []map[string]any {
	row := make(map[string]any, len(returns))
	for i, ret := range returns {
		hdr := headers[i]
		switch ret.Aggregate {
		case "COUNT":
			if ret.Expression == "*" || ret.Property == "" {
				row[hdr] = len(bindings)
			} else {
				cnt := 0
				for _, b := range bindings {
					val := e.resolveProjectionValue(b, ret)
					if val != nil {
						cnt++
					}
				}
				row[hdr] = cnt
			}
		case "COLLECT":
			collected := make([]any, 0, len(bindings))
			for _, b := range bindings {
				val := e.resolveProjectionValue(b, ret)
				if val != nil {
					collected = append(collected, val)
				}
			}
			row[hdr] = collected
		default:
			if len(bindings) > 0 {
				row[hdr] = e.resolveProjectionValue(bindings[0], ret)
			} else {
				row[hdr] = nil
			}
		}
	}
	return []map[string]any{row}
}

func deduplicateRows(rows []map[string]any, headers []string) []map[string]any {
	seen := make(map[string]bool)
	var unique []map[string]any

	for _, r := range rows {
		var keyBuilder strings.Builder
		for _, h := range headers {
			keyBuilder.WriteString(fmt.Sprintf("%v::", r[h]))
		}
		k := keyBuilder.String()
		if !seen[k] {
			seen[k] = true
			unique = append(unique, r)
		}
	}
	return unique
}

func (e *QueryExecutor) sortRows(rows []map[string]any, orderBy []OrderByItem, headers []string) {
	sort.SliceStable(rows, func(i, j int) bool {
		for _, ob := range orderBy {
			hdr := ob.Property
			// Match header alias or property
			valI := rows[i][hdr]
			valJ := rows[j][hdr]
			if valI == nil && valJ == nil {
				continue
			}
			if valI == nil {
				return ob.Direction == "ASC"
			}
			if valJ == nil {
				return ob.Direction != "ASC"
			}

			strI := fmt.Sprintf("%v", valI)
			strJ := fmt.Sprintf("%v", valJ)
			if strI != strJ {
				if ob.Direction == "DESC" {
					return strI > strJ
				}
				return strI < strJ
			}
		}
		return false
	})
}
