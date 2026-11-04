// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package when

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

type dummyRecord struct {
	ID    int
	Title string
}

func TestFluentConditionalBranching_ShortCircuitAndFallback(t *testing.T) {
	t.Parallel()

	var firstExecuted, secondExecuted, fallbackExecuted bool
	var secondCondEvaluated bool

	When(func() bool { return true }).
		Then(func() { firstExecuted = true }).
		OrElseWhen(func() bool {
			secondCondEvaluated = true
			return true
		}).
		Then(func() { secondExecuted = true }).
		OrElse(func() { fallbackExecuted = true }).
		Run()

	if !firstExecuted {
		t.Fatal("expected first branch to execute")
	}
	if secondCondEvaluated || secondExecuted {
		t.Fatal("expected second condition and branch to be short-circuited")
	}
	if fallbackExecuted {
		t.Fatal("expected fallback not to execute")
	}

	// Test fallback path
	firstExecuted = false
	secondExecuted = false
	fallbackExecuted = false

	When(func() bool { return false }).
		ThenDo(func() { firstExecuted = true }).
		OrElseWhen(func() bool { return false }).
		Then(func() { secondExecuted = true }).
		OrElse(func() { fallbackExecuted = true }).
		Run()

	if firstExecuted || secondExecuted {
		t.Fatal("expected primary branches not to execute on false conditions")
	}
	if !fallbackExecuted {
		t.Fatal("expected fallback branch to execute")
	}

	// Test no matching branch without OrElse
	var anyExecuted bool
	When(func() bool { return false }).
		Then(func() { anyExecuted = true }).
		OrElseWhen(func() bool { return false }).
		Then(func() { anyExecuted = true }).
		Run()

	if anyExecuted {
		t.Fatal("expected no branch to execute when conditions fail without OrElse")
	}
}

func TestFlowSignals_AndExecute(t *testing.T) {
	t.Parallel()

	sig := When(func() bool { return true }).
		Then(func() {}).
		AndReturn().
		Execute()
	if sig != FlowReturn {
		t.Fatalf("expected FlowReturn, got %v", sig)
	}

	sig = When(func() bool { return true }).
		Then(func() {}).
		AndContinue().
		Execute()
	if sig != FlowContinue {
		t.Fatalf("expected FlowContinue, got %v", sig)
	}

	sig = When(func() bool { return true }).
		Then(func() {}).
		AndBreak().
		Execute()
	if sig != FlowBreak {
		t.Fatalf("expected FlowBreak, got %v", sig)
	}

	sig = When(func() bool { return false }).
		Then(func() {}).
		Execute()
	if sig != FlowNone {
		t.Fatalf("expected FlowNone on non-matching execute, got %v", sig)
	}

	// Empty chain boundary checks
	emptyChain := &Chain{}
	if emptyChain.AndReturn() != emptyChain {
		t.Fatal("expected unchanged empty chain reference")
	}
	if emptyChain.AndContinue() != emptyChain {
		t.Fatal("expected unchanged empty chain reference")
	}
	if emptyChain.AndBreak() != emptyChain {
		t.Fatal("expected unchanged empty chain reference")
	}
	if emptyChain.Execute() != FlowNone {
		t.Fatalf("expected FlowNone for empty chain execute")
	}
	emptyChain.Run() // must not panic
}

func TestGenericChainT_Comprehensive(t *testing.T) {
	t.Parallel()

	// Direct match
	intVal := Result[int]().
		When(func() bool { return true }).
		Then(func() int { return 42 }).
		Run()
	if intVal != 42 {
		t.Fatalf("expected 42, got %d", intVal)
	}

	// OrElseWhen match
	var secondCondCalled bool
	strVal := Result[string]().
		When(func() bool { return false }).
		Then(func() string { return "first" }).
		OrElseWhen(func() bool { return true }).
		Then(func() string { return "second" }).
		OrElseWhen(func() bool {
			secondCondCalled = true
			return true
		}).
		Then(func() string { return "third" }).
		OrElse(func() string { return "fallback" }).
		Run()

	if strVal != "second" {
		t.Fatalf("expected 'second', got %q", strVal)
	}
	if secondCondCalled {
		t.Fatal("expected subsequent condition to be short-circuited")
	}

	// OrElse fallback match
	fallbackVal := Result[float64]().
		When(func() bool { return false }).
		Then(func() float64 { return 1.5 }).
		OrElseWhen(func() bool { return false }).
		Then(func() float64 { return 2.5 }).
		OrElse(func() float64 { return 99.9 }).
		Run()
	if fallbackVal != 99.9 {
		t.Fatalf("expected 99.9, got %f", fallbackVal)
	}

	// Zero value when no condition matches and no OrElse provided
	zeroStruct := Result[dummyRecord]().
		When(func() bool { return false }).
		Then(func() dummyRecord { return dummyRecord{ID: 1, Title: "one"} }).
		Run()
	if zeroStruct.ID != 0 || zeroStruct.Title != "" {
		t.Fatalf("expected zero struct, got %+v", zeroStruct)
	}

	zeroSlice := Result[[]int]().
		When(func() bool { return false }).
		Then(func() []int { return []int{1, 2, 3} }).
		Run()
	if zeroSlice != nil {
		t.Fatalf("expected nil slice, got %v", zeroSlice)
	}
}

func TestTryResult_ErrorPreservationAndRecovery(t *testing.T) {
	t.Parallel()

	// Success pipeline
	resSuccess := Try(10, nil).
		Then(func(v int) int { return v * 2 }).
		Then(func(v int) int { return v + 5 }).
		UnlessErr(func(err error) error {
			return fmt.Errorf("unexpected error wrapper: %w", err)
		}).
		OrElse(func() int { return -1 })

	if resSuccess.Err() != nil {
		t.Fatalf("expected nil error on success pipeline, got %v", resSuccess.Err())
	}
	if resSuccess.Val() != 25 {
		t.Fatalf("expected 25, got %d", resSuccess.Val())
	}
	v, err := resSuccess.Get()
	if err != nil || v != 25 {
		t.Fatalf("expected (25, nil), got (%d, %v)", v, err)
	}

	// Error pipeline with UnlessErr and OrElse
	originalErr := errors.New("initial failure")
	resErr := Try(100, originalErr).
		Then(func(v int) int { return v * 2 }). // must be skipped
		UnlessErr(func(err error) error {
			return fmt.Errorf("annotated: %w", err)
		}).
		OrElse(func() int { return 500 }) // sets fallback value, preserves error

	if resErr.Val() != 500 {
		t.Fatalf("expected fallback value 500, got %d", resErr.Val())
	}
	if resErr.Err() == nil || resErr.Err().Error() != "annotated: initial failure" {
		t.Fatalf("expected annotated error, got %v", resErr.Err())
	}

	// Clear error to intentionally recover
	recovered := resErr.ClearErr()
	if recovered.Err() != nil {
		t.Fatalf("expected error cleared, got %v", recovered.Err())
	}
	recVal, recErr := recovered.Get()
	if recErr != nil || recVal != 500 {
		t.Fatalf("expected (500, nil) after ClearErr, got (%d, %v)", recVal, recErr)
	}
}

func TestIfTrue_BranchingAndErrorHandling(t *testing.T) {
	t.Parallel()

	// IfTrue(true) with success
	err := IfTrue(true).
		Then(func() error { return nil }).
		UnlessErr(func(e error) error { return fmt.Errorf("wrapped: %w", e) }).
		OrElse(func() error { return errors.New("should not run") }).
		Err()
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	// IfTrue(true) with Then error
	actionErr := errors.New("action failed")
	err = IfTrue(true).
		Then(func() error { return actionErr }).
		UnlessErr(func(e error) error { return fmt.Errorf("annotated: %w", e) }).
		OrElse(func() error { return errors.New("should not run") }).
		Err()
	if err == nil || err.Error() != "annotated: action failed" {
		t.Fatalf("expected annotated error, got %v", err)
	}

	// IfTrue(false) with OrElse success
	err = IfTrue(false).
		Then(func() error { return errors.New("should not run") }).
		OrElse(func() error { return nil }).
		Err()
	if err != nil {
		t.Fatalf("expected nil error after OrElse, got %v", err)
	}

	// IfTrue(false) with OrElse error and subsequent UnlessErr
	err = IfTrue(false).
		Then(func() error { return errors.New("should not run") }).
		OrElse(func() error { return errors.New("fallback error") }).
		UnlessErr(func(e error) error { return fmt.Errorf("caught fallback: %w", e) }).
		Err()
	if err == nil || err.Error() != "caught fallback: fallback error" {
		t.Fatalf("expected caught fallback error, got %v", err)
	}

	// Skip Then when error already set
	chain := &ErrChain{err: errors.New("pre-existing error")}
	chain.Then(func() error { return errors.New("new error") })
	if chain.Err().Error() != "pre-existing error" {
		t.Fatalf("expected pre-existing error preserved, got %v", chain.Err())
	}
	chain.OrElse(func() error { return errors.New("else error") })
	if chain.Err().Error() != "pre-existing error" {
		t.Fatalf("expected pre-existing error preserved on OrElse, got %v", chain.Err())
	}
}

func TestPredicates_NilValuesAndEmptyTypes(t *testing.T) {
	t.Parallel()

	// IsNilValue tests
	if !IsNilValue(nil) {
		t.Fatal("expected untyped nil to be nil")
	}

	var ch chan int
	if !IsNilValue(ch) {
		t.Fatal("expected nil channel to be nil value")
	}
	nonNilCh := make(chan int, 1)
	if IsNilValue(nonNilCh) {
		t.Fatal("expected open channel to be non-nil")
	}

	var fn func()
	if !IsNilValue(fn) {
		t.Fatal("expected nil func to be nil value")
	}
	nonNilFn := func() {}
	if IsNilValue(nonNilFn) {
		t.Fatal("expected defined func to be non-nil")
	}

	var iface any = (*int)(nil)
	if !IsNilValue(iface) {
		t.Fatal("expected typed nil pointer in interface to be nil value")
	}
	var nonNilIface any = 42
	if IsNilValue(nonNilIface) {
		t.Fatal("expected integer in interface to be non-nil value")
	}

	var m map[string]int
	if !IsNilValue(m) {
		t.Fatal("expected nil map to be nil value")
	}
	nonNilM := make(map[string]int)
	if IsNilValue(nonNilM) {
		t.Fatal("expected initialized map to be non-nil value")
	}

	var ptr *string
	if !IsNilValue(ptr) {
		t.Fatal("expected nil pointer to be nil value")
	}
	str := "hello"
	if IsNilValue(&str) {
		t.Fatal("expected initialized pointer to be non-nil value")
	}

	var sl []byte
	if !IsNilValue(sl) {
		t.Fatal("expected nil slice to be nil value")
	}
	nonNilSl := []byte{1, 2}
	if IsNilValue(nonNilSl) {
		t.Fatal("expected initialized slice to be non-nil value")
	}

	// Default scalar kind check for IsNilValue
	if IsNilValue(123) || IsNilValue("test") || IsNilValue(true) || IsNilValue(dummyRecord{}) {
		t.Fatal("expected primitive scalars and structs to return false for IsNilValue")
	}

	// IsNilOrEmpty array checks
	zeroLenArray := [0]int{}
	if !IsNilOrEmpty(zeroLenArray) {
		t.Fatal("expected 0-length array to be nil or empty")
	}
	filledArray := [2]int{1, 2}
	if IsNilOrEmpty(filledArray) {
		t.Fatal("expected populated array not to be nil or empty")
	}

	// Non-handled kinds in IsNilOrEmpty return false
	if IsNilOrEmpty(42) || IsNilOrEmpty(true) || IsNilOrEmpty(dummyRecord{ID: 1}) {
		t.Fatal("expected unhandled types in IsNilOrEmpty to return false")
	}
}

func TestRunePredicates_AllBranches(t *testing.T) {
	t.Parallel()

	// Newline runes
	if !IsNewLine('\n') || !IsNewLine('\r') || IsNewLine(' ') || IsNewLine('a') {
		t.Fatal("IsNewLine failed")
	}
	// Punctuation runes
	if !IsComma(',') || IsComma(';') {
		t.Fatal("IsComma failed")
	}
	if !IsColon(':') || IsColon(';') {
		t.Fatal("IsColon failed")
	}
	if !IsDash('-') || IsDash('_') {
		t.Fatal("IsDash failed")
	}
	if !IsHyphen('-') || IsHyphen('_') {
		t.Fatal("IsHyphen failed")
	}
	if !IsTilde('~') || IsTilde('^') {
		t.Fatal("IsTilde failed")
	}
	if !IsFwdSlash('/') || IsFwdSlash('\\') {
		t.Fatal("IsFwdSlash failed")
	}
	if !IsBackSlash('\\') || IsBackSlash('/') {
		t.Fatal("IsBackSlash failed")
	}
	if !IsLeftCurly('{') || IsLeftCurly('(') {
		t.Fatal("IsLeftCurly failed")
	}
	if !IsRightCurly('}') || IsRightCurly(')') {
		t.Fatal("IsRightCurly failed")
	}
	if !IsDecimal('.') || IsDecimal(',') {
		t.Fatal("IsDecimal failed")
	}
	// Character and Number
	if !IsChar('A') || !IsChar('z') || IsChar('9') || IsChar('!') || IsChar('\n') {
		t.Fatal("IsChar failed")
	}
	if !IsNum('0') || !IsNum('9') || IsNum('a') || IsNum(' ') || IsNum('?') {
		t.Fatal("IsNum failed")
	}
}

func TestPointers_Helpers(t *testing.T) {
	t.Parallel()

	// Ptr creation
	val := "hello"
	p := Ptr(val)
	if p == nil || *p != "hello" {
		t.Fatalf("expected pointer to 'hello', got %v", p)
	}

	// PtrOr
	var nilInt *int
	ten := 10
	if PtrOr(nilInt, 99) != 99 {
		t.Fatalf("expected default 99 for nil pointer")
	}
	if PtrOr(&ten, 99) != 10 {
		t.Fatalf("expected dereferenced value 10")
	}

	// PtrOrElse
	var nilStr *string
	var fnInvoked bool
	strDefaultFn := func() string {
		fnInvoked = true
		return "fallback"
	}

	// Non-nil pointer skips fn
	res := PtrOrElse(&val, strDefaultFn)
	if res != "hello" || fnInvoked {
		t.Fatalf("expected 'hello' and fn not invoked, got %q (invoked=%v)", res, fnInvoked)
	}

	// Nil pointer invokes fn
	res = PtrOrElse(nilStr, strDefaultFn)
	if res != "fallback" || !fnInvoked {
		t.Fatalf("expected 'fallback' and fn invoked, got %q (invoked=%v)", res, fnInvoked)
	}

	// Nil pointer with nil fn returns zero value
	zeroRes := PtrOrElse[string](nil, nil)
	if zeroRes != "" {
		t.Fatalf("expected zero string for nil pointer and nil fn, got %q", zeroRes)
	}
}

func TestConvenienceChainBuilders_TrueAndFalseBranches(t *testing.T) {
	t.Parallel()

	// WhenNilOrEmpty
	var executedNilOrEmpty bool
	WhenNilOrEmpty("").Then(func() { executedNilOrEmpty = true }).Run()
	if !executedNilOrEmpty {
		t.Fatal("expected WhenNilOrEmpty to trigger for empty string")
	}
	executedNilOrEmpty = false
	WhenNilOrEmpty("non-empty").Then(func() { executedNilOrEmpty = true }).Run()
	if executedNilOrEmpty {
		t.Fatal("expected WhenNilOrEmpty not to trigger for non-empty string")
	}

	// WhenNotNilOrEmpty
	var executedNotNilOrEmpty bool
	WhenNotNilOrEmpty("content").Then(func() { executedNotNilOrEmpty = true }).Run()
	if !executedNotNilOrEmpty {
		t.Fatal("expected WhenNotNilOrEmpty to trigger for non-empty string")
	}
	executedNotNilOrEmpty = false
	WhenNotNilOrEmpty("").Then(func() { executedNotNilOrEmpty = true }).Run()
	if executedNotNilOrEmpty {
		t.Fatal("expected WhenNotNilOrEmpty not to trigger for empty string")
	}

	// WhenBlank
	var executedBlank bool
	WhenBlank("  \t \n ").Then(func() { executedBlank = true }).Run()
	if !executedBlank {
		t.Fatal("expected WhenBlank to trigger for whitespace string")
	}
	executedBlank = false
	WhenBlank("not blank").Then(func() { executedBlank = true }).Run()
	if executedBlank {
		t.Fatal("expected WhenBlank not to trigger for non-blank string")
	}

	// WhenNotBlank
	var executedNotBlank bool
	WhenNotBlank("content").Then(func() { executedNotBlank = true }).Run()
	if !executedNotBlank {
		t.Fatal("expected WhenNotBlank to trigger for non-blank string")
	}
	executedNotBlank = false
	WhenNotBlank("   ").Then(func() { executedNotBlank = true }).Run()
	if executedNotBlank {
		t.Fatal("expected WhenNotBlank not to trigger for blank string")
	}
}

func TestPredicateCombinators_FunctionalComposition(t *testing.T) {
	t.Parallel()

	// Predicate closure combinators
	and := func(preds ...func() bool) func() bool {
		return func() bool {
			for _, p := range preds {
				if !p() {
					return false
				}
			}
			return true
		}
	}

	or := func(preds ...func() bool) func() bool {
		return func() bool {
			for _, p := range preds {
				if p() {
					return true
				}
			}
			return false
		}
	}

	not := func(pred func() bool) func() bool {
		return func() bool {
			return !pred()
		}
	}

	condTrue := func() bool { return true }
	condFalse := func() bool { return false }

	var ran string
	When(and(condTrue, not(condFalse))).
		Then(func() { ran = "and_success" }).
		Run()
	if ran != "and_success" {
		t.Fatalf("expected and_success, got %q", ran)
	}

	ran = ""
	When(or(condFalse, condFalse)).
		Then(func() { ran = "or_unexpected" }).
		OrElse(func() { ran = "or_fallback" }).
		Run()
	if ran != "or_fallback" {
		t.Fatalf("expected or_fallback, got %q", ran)
	}
}

func TestWhen_ConcurrencyWithGoroutineLabels(t *testing.T) {
	t.Parallel()

	const workers = 8
	const iterations = 50

	var totalSuccess atomic.Int64
	var wg sync.WaitGroup
	wg.Add(workers)

	for i := 0; i < workers; i++ {
		workerID := i
		builder := goroutinelabels.NewGoroutine("test_when_concurrency", "concurrent evaluation of when chains and try blocks")
		builder.StartSimple(func() {
			defer wg.Done()

			for j := 0; j < iterations; j++ {
				// Evaluate fluent Chain
				var branch int
				When(func() bool { return (workerID+j)%2 == 0 }).
					Then(func() { branch = 1 }).
					OrElse(func() { branch = 2 }).
					Run()

				if branch != 1 && branch != 2 {
					t.Errorf("worker %d iteration %d: unexpected branch %d", workerID, j, branch)
					return
				}

				// Evaluate generic ChainT
				res := Result[int]().
					When(func() bool { return j%2 == 0 }).
					Then(func() int { return j * 10 }).
					OrElse(func() int { return -j }).
					Run()

				if (j%2 == 0 && res != j*10) || (j%2 != 0 && res != -j) {
					t.Errorf("worker %d iteration %d: unexpected generic result %d", workerID, j, res)
					return
				}

				// Evaluate TryResult
				val, err := Try("worker", nil).
					Then(func(s string) string { return fmt.Sprintf("%s-%d", s, workerID) }).
					Get()
				if err != nil || val != fmt.Sprintf("worker-%d", workerID) {
					t.Errorf("worker %d iteration %d: unexpected TryResult (%s, %v)", workerID, j, val, err)
					return
				}

				totalSuccess.Add(1)
			}
		})
	}

	wg.Wait()

	if total := totalSuccess.Load(); total != workers*iterations {
		t.Fatalf("expected %d total successful operations, got %d", workers*iterations, total)
	}
}
