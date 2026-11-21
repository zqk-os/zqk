package community

import (
	"errors"
	"reflect"
	"sync"
	"testing"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

func newTestIntentCompiler(t *testing.T) *IntentCompiler {
	t.Helper()
	compiler := NewIntentCompiler()
	if compiler == nil {
		t.Fatal("expected non-nil IntentCompiler")
	}
	return compiler
}

func assertIntentCompileError(t *testing.T, compiler *IntentCompiler, input string, wantErr error) {
	t.Helper()
	plan, err := compiler.Compile(input)
	if err == nil {
		t.Fatalf("expected error %v for input %q, got plan %+v", wantErr, input, plan)
	}
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected error wrapping %v for input %q, got %v", wantErr, input, err)
	}
}

func TestIntentAST_StaticFloorInvariant(t *testing.T) {
	compiler := newTestIntentCompiler(t)

	expr := `GOAL "Ship Release" -> STEP lint THEN STEP build; STEP test DEPENDS_ON build`
	tokens, err := compiler.Tokenize(expr)
	if err != nil {
		t.Fatalf("Tokenize failed: %v", err)
	}
	if len(tokens) < 10 {
		t.Fatalf("expected at least 10 tokens, got %d", len(tokens))
	}
	if tokens[0].Kind != TokenGoal {
		t.Fatalf("expected first token %s, got %s", TokenGoal, tokens[0].Kind)
	}
	if tokens[len(tokens)-1].Kind != TokenEOF {
		t.Fatalf("expected last token %s, got %s", TokenEOF, tokens[len(tokens)-1].Kind)
	}

	root, err := compiler.Parse(tokens)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if root.Kind != NodeKindRoot {
		t.Fatalf("expected root node kind %s, got %s", NodeKindRoot, root.Kind)
	}

	plan, err := compiler.Synthesize(root)
	if err != nil {
		t.Fatalf("Synthesize failed: %v", err)
	}
	if plan.Goal != "Ship Release" {
		t.Fatalf("expected goal %q, got %q", "Ship Release", plan.Goal)
	}

	wantOrder := []string{"lint", "build", "test"}
	if !reflect.DeepEqual(plan.OrderedStepIDs, wantOrder) {
		t.Fatalf("expected ordered steps %v, got %v", wantOrder, plan.OrderedStepIDs)
	}
	if len(plan.Digest) != 64 {
		t.Fatalf("expected 64-char sha256 digest, got %q (len=%d)", plan.Digest, len(plan.Digest))
	}
}

func TestIntentAST_OperationalDynamicProof(t *testing.T) {
	compiler := newTestIntentCompiler(t)

	expr := `
		GOAL "Parallel Pipeline" -> STAGE ci {
			PARALLEL {
				STEP unit_test;
				STEP vet;
			};
			STEP package DEPENDS_ON (unit_test, vet);
			STEP deploy DEPENDS_ON package;
		}
	`

	baseline, err := compiler.Compile(expr)
	if err != nil {
		t.Fatalf("baseline Compile failed: %v", err)
	}
	if len(baseline.Stages) != 3 {
		t.Fatalf("expected 3 execution stages, got %d", len(baseline.Stages))
	}
	if !reflect.DeepEqual(baseline.Stages[0].StepIDs, []string{"unit_test", "vet"}) {
		t.Fatalf("expected stage 0 [unit_test vet], got %v", baseline.Stages[0].StepIDs)
	}
	if !reflect.DeepEqual(baseline.Stages[1].StepIDs, []string{"package"}) {
		t.Fatalf("expected stage 1 [package], got %v", baseline.Stages[1].StepIDs)
	}
	if !reflect.DeepEqual(baseline.Stages[2].StepIDs, []string{"deploy"}) {
		t.Fatalf("expected stage 2 [deploy], got %v", baseline.Stages[2].StepIDs)
	}

	const workers = 8
	var mu sync.Mutex
	digests := make([]string, 0, workers)
	errs := make([]error, 0, workers)

	wg := goroutinelabels.StartTestGoroutines(t, workers, "intent_ast_worker", "concurrent intent compilation", func(_ int) {
		plan, compileErr := compiler.Compile(expr)
		mu.Lock()
		defer mu.Unlock()
		if compileErr != nil {
			errs = append(errs, compileErr)
			return
		}
		digests = append(digests, plan.Digest)
	})
	wg.Wait()

	if len(errs) > 0 {
		t.Fatalf("concurrent Compile errors: %v", errs)
	}
	if len(digests) != workers {
		t.Fatalf("expected %d digests, got %d", workers, len(digests))
	}
	for _, d := range digests {
		if d != baseline.Digest {
			t.Fatalf("non-deterministic digest: got %q, want %q", d, baseline.Digest)
		}
	}
	if got := compiler.CompileCount(); got != workers+1 {
		t.Fatalf("expected compile count %d, got %d", workers+1, got)
	}
}

func TestIntentAST_NegativeAdversarialBoundary(t *testing.T) {
	compiler := newTestIntentCompiler(t)

	cases := []struct {
		name    string
		input   string
		wantErr error
	}{
		{
			name:    "blank input",
			input:   "   \t\n  ",
			wantErr: ErrMalformedIntentSyntax,
		},
		{
			name:    "unterminated string",
			input:   `GOAL "unclosed -> STEP a`,
			wantErr: ErrMalformedIntentSyntax,
		},
		{
			name:    "unbalanced stage brace",
			input:   `STAGE build { STEP compile`,
			wantErr: ErrMalformedIntentSyntax,
		},
		{
			name:    "unbalanced dependency paren",
			input:   `STEP a; STEP b DEPENDS_ON (a`,
			wantErr: ErrMalformedIntentSyntax,
		},
		{
			name:    "unexpected symbol",
			input:   `STEP a @ b`,
			wantErr: ErrMalformedIntentSyntax,
		},
		{
			name:    "duplicate step declaration",
			input:   `STEP a; STEP a`,
			wantErr: ErrDuplicateIntentNode,
		},
		{
			name:    "duplicate stage declaration",
			input:   `STAGE s1 { STEP a }; STAGE s1 { STEP b }`,
			wantErr: ErrDuplicateIntentNode,
		},
		{
			name:    "dangling dependency target",
			input:   `STEP a DEPENDS_ON nonexistent`,
			wantErr: ErrUnknownDependency,
		},
		{
			name:    "self dependency",
			input:   `STEP loop DEPENDS_ON loop`,
			wantErr: ErrIntentCycleDetected,
		},
		{
			name:    "two-node cycle",
			input:   `STEP a DEPENDS_ON b; STEP b DEPENDS_ON a`,
			wantErr: ErrIntentCycleDetected,
		},
		{
			name:    "three-node cycle",
			input:   `STEP a DEPENDS_ON c; STEP b DEPENDS_ON a; STEP c DEPENDS_ON b`,
			wantErr: ErrIntentCycleDetected,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertIntentCompileError(t, compiler, tc.input, tc.wantErr)
		})
	}
}
