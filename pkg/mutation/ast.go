package mutation

// StatementType identifies discrete AST statement node types.
type StatementType string

const (
	StmtBeginTransaction    StatementType = "BeginTransaction"
	StmtCommitTransaction   StatementType = "CommitTransaction"
	StmtRollbackTransaction StatementType = "RollbackTransaction"
	StmtLet                 StatementType = "Let"
	StmtUpsert              StatementType = "Upsert"
	StmtDelete              StatementType = "Delete"
)

// ZQLExpression marks any expression in a ZQL AST.
type ZQLExpression interface {
	isZQLExpr()
}

// LiteralExpr represents a literal scalar value (string, number, bool, null).
type LiteralExpr struct {
	Value any `json:"value"`
}

func (LiteralExpr) isZQLExpr() {}

// VariableRefExpr represents a variable reference like $plan.id.
type VariableRefExpr struct {
	VariableName string   `json:"variable_name"` // without leading $
	Path         []string `json:"path,omitempty"` // nested field path e.g. ["id"]
}

func (VariableRefExpr) isZQLExpr() {}

// ObjectLiteralExpr represents { field1: expr, field2: expr }.
type ObjectLiteralExpr struct {
	Fields map[string]ZQLExpression `json:"fields"`
}

func (ObjectLiteralExpr) isZQLExpr() {}

// ListLiteralExpr represents [ expr1, expr2, ... ].
type ListLiteralExpr struct {
	Items []ZQLExpression `json:"items"`
}

func (ListLiteralExpr) isZQLExpr() {}

// UpsertExpr represents an UPSERT mutation expression.
type UpsertExpr struct {
	Kind      string            `json:"kind"`
	ID        ZQLExpression     `json:"id,omitempty"`
	BindAs    string            `json:"bind_as,omitempty"`
	Payload   ObjectLiteralExpr `json:"payload"`
	Returning []string          `json:"returning,omitempty"`
}

func (UpsertExpr) isZQLExpr() {}

// Statement represents an individual executable ZQL statement conforming to Draft 2020-12 AST.
type Statement struct {
	NodeType       StatementType     `json:"node_type"`
	Line           int               `json:"line"`
	Column         int               `json:"column"`
	IsolationLevel IsolationMode     `json:"isolation_level,omitempty"`
	VariableName   string            `json:"variable_name,omitempty"` // For LET
	Expression     ZQLExpression     `json:"expression,omitempty"`    // For LET
	Upsert         *UpsertExpr       `json:"upsert,omitempty"`        // For UPSERT
	DeleteKind     string            `json:"kind,omitempty"`          // For DELETE
	DeleteID       ZQLExpression     `json:"id,omitempty"`            // For DELETE
	CascadeMode    string            `json:"cascade_mode,omitempty"`  // CASCADE or RESTRICT
}

// ZQLProgram represents the top-level parsed AST.
type ZQLProgram struct {
	Version    string      `json:"version"`
	Type       string      `json:"type"` // "Program"
	Statements []Statement `json:"statements"`
}
