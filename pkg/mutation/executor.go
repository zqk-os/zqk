package mutation

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// ZQLExecutionReceipt aggregates transaction disposition and individual mutation outcomes.
type ZQLExecutionReceipt struct {
	TransactionID  string              `json:"transaction_id"`
	IsolationLevel IsolationMode       `json:"isolation_level"`
	Committed      bool                `json:"committed"`
	Receipts       []DiagnosticReceipt `json:"receipts"`
	Bindings       map[string]any      `json:"bindings"`
	Error          string              `json:"error,omitempty"`
}

// ZQLExecutor compiles and executes a parsed ZQLProgram against a TransactionEngine or store.
type ZQLExecutor struct {
	engine    *TransactionEngine
	validator PreflightValidator
}

// NewZQLExecutor creates a new ZQL executor.
func NewZQLExecutor(engine *TransactionEngine) *ZQLExecutor {
	if engine == nil {
		engine = NewTransactionEngine()
	}
	return &ZQLExecutor{
		engine:    engine,
		validator: NewDefaultPreflightValidator(),
	}
}

// Execute evaluates a ZQLProgram atomically.
func (e *ZQLExecutor) Execute(ctx context.Context, program *ZQLProgram) (*ZQLExecutionReceipt, error) {
	if program == nil {
		return nil, fmt.Errorf("nil ZQLProgram")
	}

	txID := fmt.Sprintf("tx-%d-%s", time.Now().UnixNano(), randomHex(4))
	isolationMode := IsolationStagedSnapshot

	// Check if BEGIN statement specifies isolation mode
	for _, s := range program.Statements {
		if s.NodeType == StmtBeginTransaction && s.IsolationLevel != "" {
			isolationMode = s.IsolationLevel
			break
		}
	}

	tx := NewTransaction(txID, isolationMode)
	bindings := make(map[string]any)

	receipt := &ZQLExecutionReceipt{
		TransactionID:  txID,
		IsolationLevel: isolationMode,
		Bindings:       bindings,
	}

	if err := tx.Begin(); err != nil {
		receipt.Error = err.Error()
		return receipt, err
	}

	// Process statements
	for _, stmt := range program.Statements {
		select {
		case <-ctx.Done():
			_ = tx.Rollback(ctx)
			receipt.Error = ctx.Err().Error()
			return receipt, ctx.Err()
		default:
		}

		switch stmt.NodeType {
		case StmtBeginTransaction:
			// Handled

		case StmtCommitTransaction:
			// Handled at end of batch

		case StmtRollbackTransaction:
			_ = tx.Rollback(ctx)
			receipt.Receipts = tx.Receipts()
			receipt.Committed = false
			return receipt, nil

		case StmtLet:
			// Resolve expression
			val, err := e.resolveExpression(stmt.Expression, bindings)
			if err != nil {
				_ = tx.Rollback(ctx)
				receipt.Error = err.Error()
				return receipt, err
			}
			bindings[stmt.VariableName] = val

			// If expression was an UPSERT, also stage the mutation
			if stmt.Upsert != nil {
				mut, err := e.buildMutation(stmt.Upsert, bindings)
				if err != nil {
					_ = tx.Rollback(ctx)
					receipt.Error = err.Error()
					return receipt, err
				}
				if err := tx.Stage(mut); err != nil {
					_ = tx.Rollback(ctx)
					receipt.Error = err.Error()
					return receipt, err
				}
				// Bind ID to variable if not already bound
				if objMap, ok := val.(map[string]any); ok {
					objMap["id"] = mut.TargetID
					objMap["kind"] = mut.TargetKind
					bindings[stmt.VariableName] = objMap
				}
			}

		case StmtUpsert:
			if stmt.Upsert != nil {
				mut, err := e.buildMutation(stmt.Upsert, bindings)
				if err != nil {
					_ = tx.Rollback(ctx)
					receipt.Error = err.Error()
					return receipt, err
				}
				if err := tx.Stage(mut); err != nil {
					_ = tx.Rollback(ctx)
					receipt.Error = err.Error()
					return receipt, err
				}
				if stmt.Upsert.BindAs != "" {
					bindings[stmt.Upsert.BindAs] = map[string]any{
						"id":   mut.TargetID,
						"kind": mut.TargetKind,
					}
				}
			}

		case StmtDelete:
			idVal, err := e.resolveExpression(stmt.DeleteID, bindings)
			if err != nil {
				_ = tx.Rollback(ctx)
				receipt.Error = err.Error()
				return receipt, err
			}
			idStr := fmt.Sprintf("%v", idVal)
			mut := Mutation{
				Action:      ActionRemoveEdge, // or delete
				TargetKind:  stmt.DeleteKind,
				TargetID:    idStr,
				SafetyClass: SafetyDestructive,
			}
			if err := tx.Stage(mut); err != nil {
				_ = tx.Rollback(ctx)
				receipt.Error = err.Error()
				return receipt, err
			}
		}
	}

	// Execute transaction through TransactionEngine
	if err := e.engine.ExecuteTransaction(ctx, tx); err != nil {
		receipt.Error = err.Error()
		receipt.Receipts = tx.Receipts()
		receipt.Committed = false
		return receipt, err
	}

	receipt.Receipts = tx.Receipts()
	receipt.Committed = (tx.State() == StateCommit)
	return receipt, nil
}

func (e *ZQLExecutor) buildMutation(u *UpsertExpr, bindings map[string]any) (Mutation, error) {
	// Generate or resolve ID
	targetID := ""
	if u.ID != nil {
		idVal, err := e.resolveExpression(u.ID, bindings)
		if err != nil {
			return Mutation{}, err
		}
		targetID = fmt.Sprintf("%v", idVal)
	} else {
		targetID = fmt.Sprintf("%s-%d-%s", strings.ToUpper(u.Kind[:min(len(u.Kind), 3)]), time.Now().UnixNano(), randomHex(3))
	}

	fields := make(map[string]any, len(u.Payload.Fields))
	for k, fExpr := range u.Payload.Fields {
		val, err := e.resolveExpression(fExpr, bindings)
		if err != nil {
			return Mutation{}, err
		}
		fields[k] = val
	}

	action := ActionCreateNode
	// If node exists in engine, treat as update
	if _, exists := e.engine.Get(context.Background(), targetID); exists {
		action = ActionUpdateNode
	}

	return Mutation{
		Action:      action,
		TargetKind:  u.Kind,
		TargetID:    targetID,
		Fields:      fields,
		SafetyClass: SafetyWrite,
	}, nil
}

func (e *ZQLExecutor) resolveExpression(expr ZQLExpression, bindings map[string]any) (any, error) {
	if expr == nil {
		return nil, nil
	}

	switch x := expr.(type) {
	case LiteralExpr:
		return x.Value, nil

	case VariableRefExpr:
		val, ok := bindings[x.VariableName]
		if !ok {
			return nil, fmt.Errorf("%s: variable $%s not found in execution bindings",
				ErrCodeZQLUnboundVariable, x.VariableName)
		}
		// Resolve path segments
		current := val
		for _, seg := range x.Path {
			m, ok := current.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("%s: cannot access path segment %q on non-object value %v",
					ErrCodeZQLUnresolvedPath, seg, current)
			}
			current, ok = m[seg]
			if !ok {
				return nil, fmt.Errorf("%s: field %q not found in object for variable $%s",
					ErrCodeZQLUnresolvedPath, seg, x.VariableName)
			}
		}
		return current, nil

	case ObjectLiteralExpr:
		res := make(map[string]any, len(x.Fields))
		for k, fExpr := range x.Fields {
			val, err := e.resolveExpression(fExpr, bindings)
			if err != nil {
				return nil, err
			}
			res[k] = val
		}
		return res, nil

	case ListLiteralExpr:
		res := make([]any, len(x.Items))
		for i, item := range x.Items {
			val, err := e.resolveExpression(item, bindings)
			if err != nil {
				return nil, err
			}
			res[i] = val
		}
		return res, nil

	case UpsertExpr:
		// Build payload object map
		res := make(map[string]any, len(x.Payload.Fields))
		for k, fExpr := range x.Payload.Fields {
			val, err := e.resolveExpression(fExpr, bindings)
			if err != nil {
				return nil, err
			}
			res[k] = val
		}
		return res, nil
	}

	return nil, fmt.Errorf("unsupported expression type %T", expr)
}

func randomHex(bytes int) string {
	b := make([]byte, bytes)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
