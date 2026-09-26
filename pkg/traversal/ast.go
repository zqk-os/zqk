package traversal

// EdgeDirection represents edge traversal direction.
type EdgeDirection string

const (
	DirectionOutgoing   EdgeDirection = "OUTGOING"
	DirectionIncoming   EdgeDirection = "INCOMING"
	DirectionUndirected EdgeDirection = "UNDIRECTED"
)

// NodePattern represents a node match pattern: (var:Kind {field: val}).
type NodePattern struct {
	Variable string         `json:"variable,omitempty"`
	Kind     string         `json:"kind,omitempty"`
	Filters  map[string]any `json:"filters,omitempty"`
}

// EdgePattern represents an edge match pattern: -[var:type*min..max]->.
type EdgePattern struct {
	Variable  string        `json:"variable,omitempty"`
	EdgeType  string        `json:"edge_type,omitempty"`
	Direction EdgeDirection `json:"direction"`
	MinDepth  int           `json:"min_depth"`
	MaxDepth  int           `json:"max_depth"`
}

// PathPattern represents a path sequence of nodes connected by edges.
type PathPattern struct {
	Nodes []NodePattern `json:"nodes"`
	Edges []EdgePattern `json:"edges"`
}

// ComparisonOp defines binary comparison operators for WHERE predicates.
type ComparisonOp string

const (
	OpEq       ComparisonOp = "="
	OpNotEq    ComparisonOp = "!="
	OpLt       ComparisonOp = "<"
	OpLte      ComparisonOp = "<="
	OpGt       ComparisonOp = ">"
	OpGte      ComparisonOp = ">="
	OpContains ComparisonOp = "CONTAINS"
	OpIn       ComparisonOp = "IN"
)

// PropertyRef references variable.field.
type PropertyRef struct {
	Variable string `json:"variable"`
	Field    string `json:"field"`
}

// Expr represents a filter expression node in a WHERE clause.
type Expr interface {
	isExpr()
}

// ComparisonExpr evaluates property op literal.
type ComparisonExpr struct {
	Property PropertyRef  `json:"property"`
	Op       ComparisonOp `json:"op"`
	Value    any          `json:"value"`
}

func (ComparisonExpr) isExpr() {}

// LogicalExpr evaluates AND/OR over sub-expressions.
type LogicalExpr struct {
	Op       string `json:"op"` // "AND" or "OR"
	Left     Expr   `json:"left"`
	Right    Expr   `json:"right"`
}

func (LogicalExpr) isExpr() {}

// NotExpr negates an expression.
type NotExpr struct {
	Inner Expr `json:"inner"`
}

func (NotExpr) isExpr() {}

// ExistsExpr evaluates existence of a subgraph pattern.
type ExistsExpr struct {
	Pattern PathPattern `json:"pattern"`
}

func (ExistsExpr) isExpr() {}

// ProjectionItem represents an element in the RETURN clause.
type ProjectionItem struct {
	Expression string `json:"expression"`
	Alias      string `json:"alias,omitempty"`
	Distinct   bool   `json:"distinct,omitempty"`
	Aggregate  string `json:"aggregate,omitempty"` // COUNT, COLLECT, MIN, MAX
	Variable   string `json:"variable,omitempty"`
	Property   string `json:"property,omitempty"`
}

// OrderByItem represents an ordering specification.
type OrderByItem struct {
	Property  string `json:"property"`
	Direction string `json:"direction"` // "ASC" or "DESC"
}

// QueryAST represents the complete parsed ZPARQL query conforming to SPEC-ZPARQL-GRAPH-TRAVERSAL-GRAMMAR.
type QueryAST struct {
	Version  string           `json:"version"`
	Type     string           `json:"type"`
	Patterns []PathPattern    `json:"patterns"`
	Where    Expr             `json:"where,omitempty"`
	Returns  []ProjectionItem `json:"returns"`
	OrderBy  []OrderByItem    `json:"order_by,omitempty"`
	Limit    int              `json:"limit,omitempty"`
	Offset   int              `json:"offset,omitempty"`
}
