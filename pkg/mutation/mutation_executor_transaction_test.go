// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package mutation

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

func TestAST_ExpressionsAndDryRun(t *testing.T) {
	t.Parallel()

	// 1. Verify isZQLExpr marker methods
	LiteralExpr{Value: "val"}.isZQLExpr()
	VariableRefExpr{VariableName: "x"}.isZQLExpr()
	ObjectLiteralExpr{Fields: map[string]ZQLExpression{}}.isZQLExpr()
	ListLiteralExpr{Items: []ZQLExpression{}}.isZQLExpr()
	UpsertExpr{Kind: "task"}.isZQLExpr()

	// 2. ApplyDryRunIsolation on nil program
	var nilProg *ZQLProgram
	nilProg.ApplyDryRunIsolation()

	// 3. ApplyDryRunIsolation on populated program
	prog := &ZQLProgram{
		Version: "1.0.0",
		Type:    "Program",
		Statements: []Statement{
			{NodeType: StmtBeginTransaction, IsolationLevel: IsolationStagedSnapshot},
			{NodeType: StmtCommitTransaction},
		},
	}
	prog.ApplyDryRunIsolation()
	require.Equal(t, IsolationDryRun, prog.Statements[0].IsolationLevel)
	require.Equal(t, StmtCommitTransaction, prog.Statements[1].NodeType)
}

func TestExecutor_InitializationAndResolvers(t *testing.T) {
	t.Parallel()

	// 1. NewZQLExecutor with nil and non-nil engine
	eNil := NewZQLExecutor(nil)
	require.NotNil(t, eNil)
	require.NotNil(t, eNil.Engine())

	eng := NewTransactionEngine()
	e := NewZQLExecutor(eng)
	require.Equal(t, eng, e.Engine())

	// 2. Test status resolver wired into executor
	eng.Seed("OBJ-1", map[string]any{"status": "in_progress"})
	defVal, ok := e.validator.(*DefaultPreflightValidator)
	require.True(t, ok)
	require.NotNil(t, defVal.statusResolver)

	st, found := defVal.statusResolver(context.Background(), "OBJ-1")
	require.True(t, found)
	require.Equal(t, "in_progress", st)

	_, foundMissing := defVal.statusResolver(context.Background(), "OBJ-MISSING")
	require.False(t, foundMissing)

	// 3. WithObjectResolver
	e.WithObjectResolver(func(ctx context.Context, id string) (map[string]any, bool, error) {
		if id == "EXT-1" {
			return map[string]any{"status": "completed"}, true, nil
		}
		return nil, false, nil
	})

	stExt, foundExt := defVal.statusResolver(context.Background(), "EXT-1")
	require.True(t, foundExt)
	require.Equal(t, "completed", stExt)
}

func TestExecutor_ExecuteEdgeCases(t *testing.T) {
	t.Parallel()
	e := NewZQLExecutor(NewTransactionEngine())

	// Nil program
	receipt, err := e.Execute(context.Background(), nil)
	require.Error(t, err)
	require.Nil(t, receipt)

	// Context cancellation
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	prog := &ZQLProgram{
		Statements: []Statement{
			{NodeType: StmtBeginTransaction},
			{NodeType: StmtCommitTransaction},
		},
	}
	receipt, err = e.Execute(ctx, prog)
	require.Error(t, err)
	require.NotNil(t, receipt)
	require.False(t, receipt.Committed)

	// StmtRollbackTransaction
	rbProg := &ZQLProgram{
		Statements: []Statement{
			{NodeType: StmtBeginTransaction},
			{NodeType: StmtRollbackTransaction},
		},
	}
	rbReceipt, rbErr := e.Execute(context.Background(), rbProg)
	require.NoError(t, rbErr)
	require.False(t, rbReceipt.Committed)
}

func TestExecutor_ExecuteLetAndUpsert(t *testing.T) {
	t.Parallel()
	e := NewZQLExecutor(NewTransactionEngine())

	// LET with unbound variable error
	letFailProg := &ZQLProgram{
		Statements: []Statement{
			{
				NodeType:     StmtLet,
				VariableName: "foo",
				Expression:   VariableRefExpr{VariableName: "missing"},
			},
		},
	}
	receipt, err := e.Execute(context.Background(), letFailProg)
	require.Error(t, err)
	require.NotNil(t, receipt)

	// LET with Upsert
	letUpsertProg := &ZQLProgram{
		Statements: []Statement{
			{
				NodeType:     StmtLet,
				VariableName: "plan",
				Expression: LiteralExpr{Value: map[string]any{
					"custom": "value",
				}},
				Upsert: &UpsertExpr{
					Kind: "priority_plan",
					ID:   LiteralExpr{Value: "PRI-101"},
					Payload: ObjectLiteralExpr{
						Fields: map[string]ZQLExpression{
							"title": LiteralExpr{Value: "Plan Title"},
						},
					},
				},
			},
			{
				NodeType: StmtUpsert,
				Upsert: &UpsertExpr{
					Kind:   "backlog_item",
					ID:     LiteralExpr{Value: "BLI-101"},
					BindAs: "bli_ref",
					Payload: ObjectLiteralExpr{
						Fields: map[string]ZQLExpression{
							"title": LiteralExpr{Value: "Item Title"},
						},
					},
				},
			},
			{NodeType: StmtCommitTransaction},
		},
	}
	receipt, err = e.Execute(context.Background(), letUpsertProg)
	require.NoError(t, err)
	require.True(t, receipt.Committed)
	require.NotNil(t, receipt.Bindings["plan"])
	require.NotNil(t, receipt.Bindings["bli_ref"])
}

func TestExecutor_ExecuteDeleteAndErrors(t *testing.T) {
	t.Parallel()
	eng := NewTransactionEngine()
	eng.Seed("DEL-1", map[string]any{"status": "planned"})
	e := NewZQLExecutor(eng)

	// StmtDelete success
	delProg := &ZQLProgram{
		Statements: []Statement{
			{
				NodeType:   StmtDelete,
				DeleteKind: "priority_plan",
				DeleteID:   LiteralExpr{Value: "DEL-1"},
			},
			{NodeType: StmtCommitTransaction},
		},
	}
	receipt, err := e.Execute(context.Background(), delProg)
	require.NoError(t, err)
	require.True(t, receipt.Committed)

	// StmtDelete expression resolution error
	delErrProg := &ZQLProgram{
		Statements: []Statement{
			{
				NodeType:   StmtDelete,
				DeleteKind: "priority_plan",
				DeleteID:   VariableRefExpr{VariableName: "missing_var"},
			},
		},
	}
	receipt, err = e.Execute(context.Background(), delErrProg)
	require.Error(t, err)
	require.NotNil(t, receipt)

	// Preflight validation failure during Execute
	invalidProg := &ZQLProgram{
		Statements: []Statement{
			{
				NodeType: StmtUpsert,
				Upsert: &UpsertExpr{
					Kind: "backlog_item",
					ID:   LiteralExpr{Value: "invalid id format"},
					Payload: ObjectLiteralExpr{
						Fields: map[string]ZQLExpression{
							"title": LiteralExpr{Value: "Bad Item"},
						},
					},
				},
			},
		},
	}
	receipt, err = e.Execute(context.Background(), invalidProg)
	require.Error(t, err)
	require.False(t, receipt.Committed)
}

func TestExecutor_BuildMutationVariants(t *testing.T) {
	t.Parallel()
	eng := NewTransactionEngine()
	eng.Seed("ENG-EXISTING", map[string]any{"title": "Existing"})
	e := NewZQLExecutor(eng)

	// 1. Target ID from u.Payload.Fields["id"]
	u1 := &UpsertExpr{
		Kind: "goal",
		Payload: ObjectLiteralExpr{
			Fields: map[string]ZQLExpression{
				"id":    LiteralExpr{Value: "GOAL-99"},
				"title": LiteralExpr{Value: "Goal Title"},
			},
		},
	}
	mut1, err := e.buildMutation(u1, map[string]any{})
	require.NoError(t, err)
	require.Equal(t, "GOAL-99", mut1.TargetID)
	require.Equal(t, ActionCreateNode, mut1.Action)

	// 2. Auto-generated ID when neither u.ID nor payload has ID
	u2 := &UpsertExpr{
		Kind: "milestone",
		Payload: ObjectLiteralExpr{
			Fields: map[string]ZQLExpression{
				"title": LiteralExpr{Value: "Milestone Title"},
			},
		},
	}
	mut2, err := e.buildMutation(u2, map[string]any{})
	require.NoError(t, err)
	require.NotEmpty(t, mut2.TargetID)
	require.Equal(t, ActionCreateNode, mut2.Action)

	// 3. Existing node in engine sets ActionUpdateNode
	u3 := &UpsertExpr{
		Kind: "goal",
		ID:   LiteralExpr{Value: "ENG-EXISTING"},
		Payload: ObjectLiteralExpr{
			Fields: map[string]ZQLExpression{
				"title": LiteralExpr{Value: "Updated Title"},
			},
		},
	}
	mut3, err := e.buildMutation(u3, map[string]any{})
	require.NoError(t, err)
	require.Equal(t, ActionUpdateNode, mut3.Action)

	// 4. ObjectResolver match sets ActionUpdateNode and seeds engine
	e.WithObjectResolver(func(ctx context.Context, id string) (map[string]any, bool, error) {
		if id == "EXT-EXISTING" {
			return map[string]any{"title": "External"}, true, nil
		}
		return nil, false, nil
	})
	u4 := &UpsertExpr{
		Kind: "goal",
		ID:   LiteralExpr{Value: "EXT-EXISTING"},
		Payload: ObjectLiteralExpr{
			Fields: map[string]ZQLExpression{
				"title": LiteralExpr{Value: "Ext Update"},
			},
		},
	}
	mut4, err := e.buildMutation(u4, map[string]any{})
	require.NoError(t, err)
	require.Equal(t, ActionUpdateNode, mut4.Action)
}

func TestExecutor_ResolveExpression(t *testing.T) {
	t.Parallel()
	e := NewZQLExecutor(NewTransactionEngine())

	// Nil expression
	val, err := e.resolveExpression(nil, map[string]any{})
	require.NoError(t, err)
	require.Nil(t, val)

	// VariableRef: path on non-object value
	bindings := map[string]any{
		"scalar": "hello",
		"obj": map[string]any{
			"nested": map[string]any{
				"target": 42,
			},
		},
	}
	_, err = e.resolveExpression(VariableRefExpr{VariableName: "scalar", Path: []string{"sub"}}, bindings)
	require.Error(t, err)
	require.Contains(t, err.Error(), ErrCodeZQLUnresolvedPath)

	// VariableRef: missing field on object
	_, err = e.resolveExpression(VariableRefExpr{VariableName: "obj", Path: []string{"missing"}}, bindings)
	require.Error(t, err)
	require.Contains(t, err.Error(), ErrCodeZQLUnresolvedPath)

	// VariableRef: valid nested path
	nestedVal, err := e.resolveExpression(VariableRefExpr{VariableName: "obj", Path: []string{"nested", "target"}}, bindings)
	require.NoError(t, err)
	require.Equal(t, 42, nestedVal)

	// ListLiteralExpr resolution & sub-item error
	listExpr := ListLiteralExpr{
		Items: []ZQLExpression{
			LiteralExpr{Value: 1},
			LiteralExpr{Value: 2},
		},
	}
	listVal, err := e.resolveExpression(listExpr, bindings)
	require.NoError(t, err)
	require.Equal(t, []any{1, 2}, listVal)

	badList := ListLiteralExpr{
		Items: []ZQLExpression{
			VariableRefExpr{VariableName: "not_found"},
		},
	}
	_, err = e.resolveExpression(badList, bindings)
	require.Error(t, err)

	// ObjectLiteral sub-field error
	badObj := ObjectLiteralExpr{
		Fields: map[string]ZQLExpression{
			"f": VariableRefExpr{VariableName: "not_found"},
		},
	}
	_, err = e.resolveExpression(badObj, bindings)
	require.Error(t, err)
}

func TestExecutor_ResolveUpsertExprAndMin(t *testing.T) {
	t.Parallel()
	e := NewZQLExecutor(NewTransactionEngine())

	// UpsertExpr as expression
	upsertExpr := UpsertExpr{
		Kind: "epic",
		ID:   LiteralExpr{Value: "EPC-10"},
		Payload: ObjectLiteralExpr{
			Fields: map[string]ZQLExpression{
				"name": LiteralExpr{Value: "Epic One"},
			},
		},
	}
	res, err := e.resolveExpression(upsertExpr, map[string]any{})
	require.NoError(t, err)
	m, ok := res.(map[string]any)
	require.True(t, ok)
	require.Equal(t, "EPC-10", m["id"])
	require.Equal(t, "epic", m["kind"])
	require.Equal(t, "Epic One", m["name"])

	// min helper test
	require.Equal(t, 3, min(3, 5))
	require.Equal(t, 2, min(7, 2))
}

func TestParser_LexerScanners(t *testing.T) {
	t.Parallel()

	// 1. peek(n) within and beyond length
	lexer := NewZQLLexer("abc")
	require.Equal(t, 'b', lexer.peek(1))
	require.Equal(t, 'c', lexer.peek(2))
	require.Equal(t, rune(0), lexer.peek(3))
	require.Equal(t, rune(0), lexer.peek(10))

	// 2. Comments: single line // and --, multi-line /* ... */
	commentInput := `
		// single line slash comment
		-- single line dash comment
		/* multi line
		   comment */
		42
	`
	lexComments := NewZQLLexer(commentInput)
	tok, err := lexComments.NextToken()
	require.NoError(t, err)
	require.Equal(t, ZQLTokenNumber, tok.Type)
	require.Equal(t, "42", tok.Value)

	// 3. Scan number variants: positive int, negative int, positive float, negative float
	numLexer := NewZQLLexer("100 -25 3.14 -0.05")
	expectedNums := []string{"100", "-25", "3.14", "-0.05"}
	for _, exp := range expectedNums {
		tok, err := numLexer.NextToken()
		require.NoError(t, err)
		require.Equal(t, ZQLTokenNumber, tok.Type)
		require.Equal(t, exp, tok.Value)
	}

	// 4. String escape characters & unclosed string
	strLexer := NewZQLLexer(`"hello\n\"world\""`)
	tok, err = strLexer.NextToken()
	require.NoError(t, err)
	require.Equal(t, "hello\n\"world\"", tok.Value)

	badStrLexer := NewZQLLexer(`"unclosed string`)
	_, err = badStrLexer.NextToken()
	require.Error(t, err)
	require.Contains(t, err.Error(), ErrCodeZQLSyntaxError)

	// 5. Lone $ error & unexpected character error
	badVarLexer := NewZQLLexer(`$ `)
	_, err = badVarLexer.NextToken()
	require.Error(t, err)

	badCharLexer := NewZQLLexer(`@`)
	_, err = badCharLexer.NextToken()
	require.Error(t, err)
}

func TestParser_ParseStatements(t *testing.T) {
	t.Parallel()

	// 1. Rollback statement variants
	prog1, err := ParseZQL("ROLLBACK TRANSACTION;")
	require.NoError(t, err)
	require.Equal(t, StmtRollbackTransaction, prog1.Statements[0].NodeType)

	prog2, err := ParseZQL("ROLLBACK;")
	require.NoError(t, err)
	require.Equal(t, StmtRollbackTransaction, prog2.Statements[0].NodeType)

	_, err = ParseZQL("ROLLBACK")
	require.Error(t, err)

	// 2. Delete statement variants
	del1, err := ParseZQL(`DELETE priority_plan "PRI-1" CASCADE;`)
	require.NoError(t, err)
	require.Equal(t, StmtDelete, del1.Statements[0].NodeType)
	require.Equal(t, "CASCADE", del1.Statements[0].CascadeMode)
	require.Equal(t, "priority_plan", del1.Statements[0].DeleteKind)

	del2, err := ParseZQL(`DELETE priority_plan "PRI-2" RESTRICT;`)
	require.NoError(t, err)
	require.Equal(t, "RESTRICT", del2.Statements[0].CascadeMode)

	del3, err := ParseZQL(`DELETE priority_plan "PRI-3";`)
	require.NoError(t, err)
	require.Equal(t, "RESTRICT", del3.Statements[0].CascadeMode)

	_, err = ParseZQL(`DELETE priority_plan "PRI-4"`)
	require.Error(t, err)
}

func TestParser_ParseLiteralsAndLists(t *testing.T) {
	t.Parallel()

	// 1. List literals: empty, populated, booleans, null, numbers
	script := `
		LET $arr = [1, 2.5, true, false, null, "text"];
	`
	prog, err := ParseZQL(script)
	require.NoError(t, err)
	require.Len(t, prog.Statements, 1)
	letStmt := prog.Statements[0]
	listExpr, ok := letStmt.Expression.(ListLiteralExpr)
	require.True(t, ok)
	require.Len(t, listExpr.Items, 6)

	// 2. Invalid list syntax (missing closing bracket)
	_, err = ParseZQL(`LET $bad = [1, 2;`)
	require.Error(t, err)

	// 3. Object literal syntax error (missing colon)
	_, err = ParseZQL(`LET $bad_obj = { key "missing_colon" };`)
	require.Error(t, err)

	// 4. Object literal syntax error (missing comma or closing brace)
	_, err = ParseZQL(`LET $bad_obj2 = { key: "val" other: "val" };`)
	require.Error(t, err)

	// 5. Invalid variable ref syntax
	_, err = ParseZQL(`LET $bad_ref = $var.;`)
	require.Error(t, err)
}

func TestPreflight_HelperFunctions(t *testing.T) {
	t.Parallel()

	// 1. parseEnumStrings
	res1 := parseEnumStrings([]string{"a", "b"})
	require.Equal(t, []string{"a", "b"}, res1)

	res2 := parseEnumStrings([]any{"x", 123, true, nil})
	require.Equal(t, []string{"x", "123", "true"}, res2)

	res3 := parseEnumStrings("not a slice")
	require.Nil(t, res3)

	// 2. parseNumericValue
	n1, ok1 := parseNumericValue(int(10))
	require.True(t, ok1)
	require.Equal(t, float64(10), n1)

	n2, ok2 := parseNumericValue(int32(20))
	require.True(t, ok2)
	require.Equal(t, float64(20), n2)

	n3, ok3 := parseNumericValue(int64(30))
	require.True(t, ok3)
	require.Equal(t, float64(30), n3)

	n4, ok4 := parseNumericValue(float32(40.5))
	require.True(t, ok4)
	require.Equal(t, float64(float32(40.5)), n4)

	n5, ok5 := parseNumericValue(float64(50.5))
	require.True(t, ok5)
	require.Equal(t, 50.5, n5)

	_, okBad := parseNumericValue("string")
	require.False(t, okBad)

	// 3. Chained loader setters
	v := NewDefaultPreflightValidator()
	require.Equal(t, v, v.WithSpecLoader(nil))
	require.Equal(t, v, v.WithLifecycleLoader(nil))
}

func TestPreflight_ValidationEdgeBranches(t *testing.T) {
	t.Parallel()
	v := NewDefaultPreflightValidator()
	ctx := context.Background()

	// 1. Nil mutation
	rNil, err := v.Validate(ctx, nil)
	require.NoError(t, err)
	require.False(t, rNil.Valid)
	require.Equal(t, "rejected_failclosed", rNil.Disposition)

	// 2. Empty action & invalid action enum
	mEmptyAction := &Mutation{TargetKind: "backlog_item"}
	r1, err1 := v.Validate(ctx, mEmptyAction)
	require.NoError(t, err1)
	require.False(t, r1.Valid)

	mInvalidAction := &Mutation{Action: "invalid_action", TargetKind: "backlog_item"}
	r2, err2 := v.Validate(ctx, mInvalidAction)
	require.NoError(t, err2)
	require.False(t, r2.Valid)

	// 3. Empty target kind
	mEmptyKind := &Mutation{Action: ActionCreateNode}
	r3, err3 := v.Validate(ctx, mEmptyKind)
	require.NoError(t, err3)
	require.False(t, r3.Valid)

	// 4. Invalid safety class enum
	mInvalidSafety := &Mutation{
		Action:      ActionCreateNode,
		TargetKind:  "backlog_item",
		SafetyClass: "dangerous",
	}
	r4, err4 := v.Validate(ctx, mInvalidSafety)
	require.NoError(t, err4)
	require.False(t, r4.Valid)

	// 5. Invalid priority tier enum and non-string type
	mInvalidTier := &Mutation{
		Action:     ActionCreateNode,
		TargetKind: "backlog_item",
		Fields:     map[string]any{"priority_tier": "P99", "title": "Test"},
	}
	r5, err5 := v.Validate(ctx, mInvalidTier)
	require.NoError(t, err5)
	require.False(t, r5.Valid)

	mNonStringTier := &Mutation{
		Action:     ActionCreateNode,
		TargetKind: "backlog_item",
		Fields:     map[string]any{"priority_tier": 123, "title": "Test"},
	}
	r6, err6 := v.Validate(ctx, mNonStringTier)
	require.NoError(t, err6)
	require.False(t, r6.Valid)

	// 6. Non-string status & non-string title & missing title on create
	mNonStringStatus := &Mutation{
		Action:     ActionCreateNode,
		TargetKind: "backlog_item",
		Fields:     map[string]any{"status": 999, "title": "Test"},
	}
	r7, err7 := v.Validate(ctx, mNonStringStatus)
	require.NoError(t, err7)
	require.False(t, r7.Valid)

	mMissingTitle := &Mutation{
		Action:     ActionCreateNode,
		TargetKind: "backlog_item",
		Fields:     map[string]any{},
	}
	r8, err8 := v.Validate(ctx, mMissingTitle)
	require.NoError(t, err8)
	require.False(t, r8.Valid)

	mNonStringTitle := &Mutation{
		Action:     ActionCreateNode,
		TargetKind: "backlog_item",
		Fields:     map[string]any{"title": 12345},
	}
	r9, err9 := v.Validate(ctx, mNonStringTitle)
	require.NoError(t, err9)
	require.False(t, r9.Valid)
}

func TestPreflight_ArrayRefsAndBatch(t *testing.T) {
	t.Parallel()
	v := NewDefaultPreflightValidator()
	ctx := context.Background()

	// 1. Array refs check non-slice violation
	mBadRefs := &Mutation{
		Action:     ActionCreateNode,
		TargetKind: "backlog_item",
		Fields: map[string]any{
			"title":         "Valid Title",
			"criteria_refs": "not-a-slice",
		},
	}
	receipt, err := v.Validate(ctx, mBadRefs)
	require.NoError(t, err)
	require.False(t, receipt.Valid)

	// 2. ValidateBatch with mixed outcomes
	muts := []Mutation{
		{
			Action:     ActionCreateNode,
			TargetKind: "backlog_item",
			Fields:     map[string]any{"title": "Valid 1"},
		},
		{
			Action:     ActionCreateNode,
			TargetKind: "backlog_item",
			Fields:     map[string]any{}, // Missing title
		},
	}
	batch, err := v.ValidateBatch(ctx, muts)
	require.NoError(t, err)
	require.Equal(t, 2, batch.Total)
	require.Equal(t, 1, batch.ValidCount)
	require.Equal(t, 1, batch.InvalidCount)
	require.False(t, batch.AllValid)
}

func TestTransaction_StateTransitionsAndAccessors(t *testing.T) {
	t.Parallel()

	tx := NewTransaction("TX-999", IsolationStagedSnapshot)
	require.Equal(t, "TX-999", tx.ID())
	require.Equal(t, IsolationAllOrNothing, tx.Mode())
	require.Equal(t, StateInitial, tx.State())
	require.Empty(t, tx.Receipts())
	require.Empty(t, tx.StagedMutations())

	// Begin
	require.NoError(t, tx.Begin())
	require.Equal(t, StateBegin, tx.State())

	// Cannot Begin again
	require.Error(t, tx.Begin())

	// Stage mutation
	mut := Mutation{Action: ActionCreateNode, TargetKind: "goal", TargetID: "G-1"}
	require.NoError(t, tx.Stage(mut))
	require.Len(t, tx.StagedMutations(), 1)
	require.Len(t, tx.Receipts(), 1)

	// PreCheck
	require.NoError(t, tx.PreCheck(context.Background(), func(ctx context.Context, idx int, m *Mutation) error {
		return nil
	}))
	require.Equal(t, StatePreCheck, tx.State())

	// Cannot Stage in Initial state
	txUnstarted := NewTransaction("TX-UNSTARTED", IsolationAllOrNothing)
	require.Error(t, txUnstarted.Stage(mut))
}

func TestTransaction_PreCheckFailures(t *testing.T) {
	t.Parallel()

	// AllOrNothing mode PreCheck failure
	txAll := NewTransaction("TX-ALL", IsolationAllOrNothing)
	require.NoError(t, txAll.Begin())
	require.NoError(t, txAll.Stage(Mutation{Action: ActionCreateNode, TargetKind: "task", TargetID: "T-1"}))
	errAll := txAll.PreCheck(context.Background(), func(ctx context.Context, idx int, m *Mutation) error {
		return errors.New("check failed")
	})
	require.Error(t, errAll)

	// PartialCommit mode PreCheck quarantine
	txPart := NewTransaction("TX-PART", IsolationPartialCommit)
	require.NoError(t, txPart.Begin())
	require.NoError(t, txPart.Stage(Mutation{Action: ActionCreateNode, TargetKind: "task", TargetID: "T-2"}))
	errPart := txPart.PreCheck(context.Background(), func(ctx context.Context, idx int, m *Mutation) error {
		return errors.New("check failed")
	})
	require.NoError(t, errPart)
	require.Equal(t, ReceiptStatusQuarantined, txPart.Receipts()[0].Status)
}

func TestTransaction_RollbackAndCommitVariants(t *testing.T) {
	t.Parallel()

	// 1. Rollback handlers executed in reverse LIFO order
	var order []int
	tx := NewTransaction("TX-RB", IsolationStagedSnapshot)
	require.NoError(t, tx.Begin())
	tx.RegisterRollback(func(ctx context.Context) error {
		order = append(order, 1)
		return nil
	})
	tx.RegisterRollback(func(ctx context.Context) error {
		order = append(order, 2)
		return nil
	})
	require.NoError(t, tx.Rollback(context.Background()))
	require.Equal(t, []int{2, 1}, order)
	require.Equal(t, StateRollback, tx.State())

	// Idempotent rollback
	require.NoError(t, tx.Rollback(context.Background()))

	// Rollback error accumulation
	txErr := NewTransaction("TX-RBERR", IsolationStagedSnapshot)
	require.NoError(t, txErr.Begin())
	txErr.RegisterRollback(func(ctx context.Context) error {
		return errors.New("undo error")
	})
	require.Error(t, txErr.Rollback(context.Background()))

	// Commit requires PreCheck
	txNoPre := NewTransaction("TX-NOPRE", IsolationStagedSnapshot)
	require.NoError(t, txNoPre.Begin())
	errCommit := txNoPre.Commit(context.Background(), nil)
	require.Error(t, errCommit)

	// DryRun isolation commit marks receipts validated
	txDry := NewTransaction("TX-DRY", IsolationDryRun)
	require.NoError(t, txDry.Begin())
	require.NoError(t, txDry.Stage(Mutation{Action: ActionCreateNode, TargetKind: "task", TargetID: "T-DRY"}))
	require.NoError(t, txDry.PreCheck(context.Background(), func(ctx context.Context, idx int, m *Mutation) error {
		return nil
	}))
	require.NoError(t, txDry.Commit(context.Background(), nil))
	require.Equal(t, ReceiptStatusDryRunValidated, txDry.Receipts()[0].Status)

	// Cannot rollback committed transaction
	require.Error(t, txDry.Rollback(context.Background()))
}

func TestTransaction_CommitFailureUnwinds(t *testing.T) {
	t.Parallel()

	// AllOrNothing unwinds applied rollbacks upon applier error
	var unwound []int
	tx := NewTransaction("TX-UNWIND", IsolationAllOrNothing)
	require.NoError(t, tx.Begin())
	require.NoError(t, tx.Stage(Mutation{Action: ActionCreateNode, TargetKind: "task", TargetID: "T-1"}))
	require.NoError(t, tx.Stage(Mutation{Action: ActionCreateNode, TargetKind: "task", TargetID: "T-2"}))
	require.NoError(t, tx.PreCheck(context.Background(), func(ctx context.Context, idx int, m *Mutation) error {
		return nil
	}))

	err := tx.Commit(context.Background(), func(ctx context.Context, idx int, mut *Mutation) (func(context.Context) error, error) {
		if idx == 0 {
			return func(ctx context.Context) error {
				unwound = append(unwound, 0)
				return nil
			}, nil
		}
		return nil, errors.New("apply error on item 1")
	})
	require.Error(t, err)
	require.Equal(t, []int{0}, unwound)
	require.Equal(t, StateRollback, tx.State())

	// PartialCommit quarantines error item
	txPart := NewTransaction("TX-PART-COMMIT", IsolationPartialCommit)
	require.NoError(t, txPart.Begin())
	require.NoError(t, txPart.Stage(Mutation{Action: ActionCreateNode, TargetKind: "task", TargetID: "T-OK"}))
	require.NoError(t, txPart.Stage(Mutation{Action: ActionCreateNode, TargetKind: "task", TargetID: "T-FAIL"}))
	require.NoError(t, txPart.PreCheck(context.Background(), func(ctx context.Context, idx int, m *Mutation) error {
		return nil
	}))

	err = txPart.Commit(context.Background(), func(ctx context.Context, idx int, mut *Mutation) (func(context.Context) error, error) {
		if idx == 0 {
			return nil, nil
		}
		return nil, errors.New("error on item 1")
	})
	require.NoError(t, err)
	require.Equal(t, StateCommit, txPart.State())
	require.Equal(t, ReceiptStatusCommitted, txPart.Receipts()[0].Status)
	require.Equal(t, ReceiptStatusQuarantined, txPart.Receipts()[1].Status)
}

func TestTransactionEngine_FullExecutionFlow(t *testing.T) {
	t.Parallel()
	eng := NewTransactionEngine()

	// Seed, Get, Count
	eng.Seed("NODE-1", map[string]any{"key": "initial"})
	val, ok := eng.Get(context.Background(), "NODE-1")
	require.True(t, ok)
	require.Equal(t, "initial", val["key"])
	require.Equal(t, 1, eng.Count(context.Background()))

	// Deep copy verify
	val["key"] = "tampered"
	fresh, okFresh := eng.Get(context.Background(), "NODE-1")
	require.True(t, okFresh)
	require.Equal(t, "initial", fresh["key"])

	// Update node with break glass
	bgCtx := pkgctx.WithLifecycleBreakGlass(context.Background(), "repair protocol")
	txUp := NewTransaction("TX-UP", IsolationStagedSnapshot)
	require.NoError(t, txUp.Begin())
	require.NoError(t, txUp.Stage(Mutation{
		Action:     ActionUpdateNode,
		TargetKind: "task",
		TargetID:   "NODE-1",
		Fields:     map[string]any{"key": "updated"},
	}))
	require.NoError(t, eng.ExecuteTransaction(bgCtx, txUp))

	updated, okUpdated := eng.Get(context.Background(), "NODE-1")
	require.True(t, okUpdated)
	require.Equal(t, "updated", updated["key"])

	// Edge add/remove simulation
	txEdge := NewTransaction("TX-EDGE", IsolationStagedSnapshot)
	require.NoError(t, txEdge.Begin())
	require.NoError(t, txEdge.Stage(Mutation{
		Action:     ActionAddEdge,
		TargetKind: "task",
		TargetID:   "NODE-1",
	}))
	require.NoError(t, eng.ExecuteTransaction(context.Background(), txEdge))
}

func TestTransactionEngine_ErrorBranches(t *testing.T) {
	t.Parallel()
	eng := NewTransactionEngine()
	ctx := context.Background()

	// 1. Fail trigger on create node
	txFail := NewTransaction("TX-FAIL", IsolationStagedSnapshot)
	require.NoError(t, txFail.Begin())
	require.NoError(t, txFail.Stage(Mutation{
		Action:     ActionCreateNode,
		TargetKind: "task",
		TargetID:   "NODE-FAIL",
		Fields:     map[string]any{"fail_trigger": true},
	}))
	require.Error(t, eng.ExecuteTransaction(ctx, txFail))

	// 2. Duplicate create node error
	eng.Seed("NODE-DUP", map[string]any{"exists": true})
	txDup := NewTransaction("TX-DUP", IsolationStagedSnapshot)
	require.NoError(t, txDup.Begin())
	require.NoError(t, txDup.Stage(Mutation{
		Action:     ActionCreateNode,
		TargetKind: "task",
		TargetID:   "NODE-DUP",
		Fields:     map[string]any{"title": "Dup"},
	}))
	require.Error(t, eng.ExecuteTransaction(ctx, txDup))

	// 3. Update non-existent node error
	txMissing := NewTransaction("TX-MISS", IsolationStagedSnapshot)
	require.NoError(t, txMissing.Begin())
	require.NoError(t, txMissing.Stage(Mutation{
		Action:     ActionUpdateNode,
		TargetKind: "task",
		TargetID:   "DOES-NOT-EXIST",
		Fields:     map[string]any{"title": "Missing"},
	}))
	require.Error(t, eng.ExecuteTransaction(ctx, txMissing))

	// 4. Empty TargetID error
	txNoID := NewTransaction("TX-NOID", IsolationStagedSnapshot)
	require.NoError(t, txNoID.Begin())
	require.NoError(t, txNoID.Stage(Mutation{
		Action:     ActionCreateNode,
		TargetKind: "task",
		TargetID:   "",
	}))
	require.Error(t, eng.ExecuteTransaction(ctx, txNoID))

	// 5. Unsupported action error
	txBadAct := NewTransaction("TX-BADACT", IsolationStagedSnapshot)
	require.NoError(t, txBadAct.Begin())
	require.NoError(t, txBadAct.Stage(Mutation{
		Action:     "unknown_action",
		TargetKind: "task",
		TargetID:   "NODE-X",
	}))
	require.Error(t, eng.ExecuteTransaction(ctx, txBadAct))
}

func TestMutation_ConcurrentGoroutineSafety(t *testing.T) {
	t.Parallel()
	eng := NewTransactionEngine()
	ctx := context.Background()

	const workers = 8
	const iterations = 15

	var wg sync.WaitGroup
	errCh := make(chan error, workers*iterations)

	for i := 0; i < workers; i++ {
		workerID := i
		goroutinelabels.NewGoroutine(
			fmt.Sprintf("mutation_worker_%d", workerID),
			"concurrent transaction stress testing",
		).WithWaitGroup(&wg).StartSimple(func() {
			for j := 0; j < iterations; j++ {
				id := fmt.Sprintf("CONCUR-%d-%d", workerID, j)
				tx := NewTransaction(fmt.Sprintf("TX-%s", id), IsolationStagedSnapshot)
				if err := tx.Begin(); err != nil {
					errCh <- err
					return
				}
				if err := tx.Stage(Mutation{
					Action:     ActionCreateNode,
					TargetKind: "backlog_item",
					TargetID:   id,
					Fields: map[string]any{
						"title": fmt.Sprintf("Item %s", id),
					},
				}); err != nil {
					errCh <- err
					return
				}
				if err := eng.ExecuteTransaction(ctx, tx); err != nil {
					errCh <- err
					return
				}
				// Verify committed state readable immediately
				if _, ok := eng.Get(ctx, id); !ok {
					errCh <- fmt.Errorf("node %s missing after commit", id)
					return
				}
			}
		})
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		require.NoError(t, err)
	}

	require.Equal(t, workers*iterations, eng.Count(ctx))
}
