// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package functional

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/zqk-os/zqk/pkg/coordination"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

type cycle173MockCoordinator struct {
	mu     sync.Mutex
	events []*coordination.EventContext
}

func (m *cycle173MockCoordinator) Emit(ctx context.Context, eventCtx *coordination.EventContext) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, eventCtx)
	return nil
}

func (m *cycle173MockCoordinator) Subscribe(subscriber coordination.OperationalEventSubscriber) string {
	return "mock-sub-1"
}

func (m *cycle173MockCoordinator) Unsubscribe(subscriberID string) {}

func (m *cycle173MockCoordinator) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.events)
}

func (m *cycle173MockCoordinator) lastEvent() *coordination.EventContext {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.events) == 0 {
		return nil
	}
	return m.events[len(m.events)-1]
}

func TestResult_ConstructorsAndInspection(t *testing.T) {
	t.Parallel()

	okRes := Ok("payload")
	if !okRes.IsOk() {
		t.Fatal("expected okRes.IsOk() to be true")
	}
	if okRes.IsErr() {
		t.Fatal("expected okRes.IsErr() to be false")
	}
	if okRes.Err() != nil {
		t.Fatalf("expected nil error, got: %v", okRes.Err())
	}
	val, err := okRes.Value()
	if err != nil || val != "payload" {
		t.Fatalf("unexpected Value() result: val=%s, err=%v", val, err)
	}

	errSample := errors.New("synthetic failure")
	errRes := Err[string](errSample)
	if errRes.IsOk() {
		t.Fatal("expected errRes.IsOk() to be false")
	}
	if !errRes.IsErr() {
		t.Fatal("expected errRes.IsErr() to be true")
	}
	if !errors.Is(errRes.Err(), errSample) {
		t.Fatalf("expected error %v, got %v", errSample, errRes.Err())
	}
	valErr, rawErr := errRes.Value()
	if !errors.Is(rawErr, errSample) || valErr != "" {
		t.Fatalf("unexpected errRes Value(): val=%s, err=%v", valErr, rawErr)
	}

	fromOk := From("direct", nil)
	if !fromOk.IsOk() || fromOk.Unwrap() != "direct" {
		t.Fatal("expected fromOk to be Ok")
	}
	fromErr := From("ignored", errSample)
	if !fromErr.IsErr() || !errors.Is(fromErr.Err(), errSample) {
		t.Fatal("expected fromErr to preserve error")
	}
}

func TestResult_UnwrapVariants(t *testing.T) {
	t.Parallel()

	okRes := Ok(42)
	if okRes.Unwrap() != 42 {
		t.Fatalf("expected 42, got %d", okRes.Unwrap())
	}
	if okRes.UnwrapOr(99) != 42 {
		t.Fatalf("expected 42, got %d", okRes.UnwrapOr(99))
	}
	if okRes.UnwrapOrElse(func(err error) int { return 99 }) != 42 {
		t.Fatalf("expected 42, got %d", okRes.UnwrapOrElse(func(err error) int { return 99 }))
	}

	errSample := errors.New("boom")
	errRes := Err[int](errSample)
	if errRes.UnwrapOr(100) != 100 {
		t.Fatalf("expected fallback 100, got %d", errRes.UnwrapOr(100))
	}
	var capturedErr error
	fallbackComputed := errRes.UnwrapOrElse(func(err error) int {
		capturedErr = err
		return 200
	})
	if fallbackComputed != 200 || !errors.Is(capturedErr, errSample) {
		t.Fatalf("expected 200 and errSample, got %d, %v", fallbackComputed, capturedErr)
	}

	assertPanics(t, "Unwrap on Err", func() {
		errRes.Unwrap()
	})
}

func assertPanics(t *testing.T, desc string, fn func()) {
	t.Helper()
	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("expected %s to panic, but it did not", desc)
		}
	}()
	fn()
}

func TestResult_MonadicTransforms(t *testing.T) {
	t.Parallel()

	okRes := Ok(10)
	mapped := Map(okRes, func(v int) string {
		return fmt.Sprintf("val-%d", v)
	})
	if !mapped.IsOk() || mapped.Unwrap() != "val-10" {
		t.Fatalf("unexpected mapped result: %v", mapped)
	}

	errSample := errors.New("initial failure")
	errRes := Err[int](errSample)
	mappedErr := Map(errRes, func(v int) string {
		return fmt.Sprintf("val-%d", v)
	})
	if !mappedErr.IsErr() || !errors.Is(mappedErr.Err(), errSample) {
		t.Fatalf("expected error propagated, got: %v", mappedErr)
	}

	mappedErrResult := MapErr(errRes, func(err error) error {
		return fmt.Errorf("wrapped: %w", err)
	})
	if !mappedErrResult.IsErr() || !errors.Is(mappedErrResult.Err(), errSample) {
		t.Fatalf("expected wrapped error, got: %v", mappedErrResult.Err())
	}

	unmodifiedOk := MapErr(okRes, func(err error) error {
		return errors.New("should not run")
	})
	if !unmodifiedOk.IsOk() || unmodifiedOk.Unwrap() != 10 {
		t.Fatal("expected MapErr on Ok to leave result untouched")
	}

	flatMapOk := FlatMap(okRes, func(v int) Result[int] {
		return Ok(v * 3)
	})
	if !flatMapOk.IsOk() || flatMapOk.Unwrap() != 30 {
		t.Fatalf("expected 30, got %v", flatMapOk)
	}

	flatMapErr := FlatMap(errRes, func(v int) Result[int] {
		return Ok(v * 3)
	})
	if !flatMapErr.IsErr() || !errors.Is(flatMapErr.Err(), errSample) {
		t.Fatal("expected flatMapErr to propagate error")
	}
}

func TestResult_OrElseAndAlternatives(t *testing.T) {
	t.Parallel()

	okRes := Ok("alpha")
	errSample := errors.New("err-alpha")
	errRes := Err[string](errSample)

	if OrElse(okRes, Ok("beta")).Unwrap() != "alpha" {
		t.Fatal("expected OrElse on Ok to return okRes")
	}
	if OrElse(errRes, Ok("beta")).Unwrap() != "beta" {
		t.Fatal("expected OrElse on Err to return alternative")
	}

	orElseGetOk := OrElseGet(okRes, func(err error) Result[string] {
		return Ok("computed")
	})
	if orElseGetOk.Unwrap() != "alpha" {
		t.Fatal("expected OrElseGet on Ok to return okRes")
	}

	var captured error
	orElseGetErr := OrElseGet(errRes, func(err error) Result[string] {
		captured = err
		return Ok("recovered")
	})
	if orElseGetErr.Unwrap() != "recovered" || !errors.Is(captured, errSample) {
		t.Fatalf("expected recovered and captured error, got %v", orElseGetErr)
	}
}

func TestResult_MatchAndMatchErr(t *testing.T) {
	t.Parallel()

	okRes := Ok(100)
	errSample := errors.New("mismatch")
	errRes := Err[int](errSample)

	matchedVal := Match(okRes, func(v int) string {
		return fmt.Sprintf("success-%d", v)
	}, func(err error) string {
		return "failed"
	})
	if matchedVal != "success-100" {
		t.Fatalf("expected success-100, got %s", matchedVal)
	}

	matchedErrVal := Match(errRes, func(v int) string {
		return "unexpected"
	}, func(err error) string {
		return fmt.Sprintf("error-%v", err)
	})
	if matchedErrVal != "error-mismatch" {
		t.Fatalf("expected error-mismatch, got %s", matchedErrVal)
	}

	var methodSuccessCalled, methodFailureCalled bool
	okRes.Match(func(v int) {
		methodSuccessCalled = true
	}, func(err error) {
		methodFailureCalled = true
	})
	if !methodSuccessCalled || methodFailureCalled {
		t.Fatal("expected okRes.Match to invoke only onSuccess")
	}

	methodSuccessCalled = false
	methodFailureCalled = false
	errRes.Match(func(v int) {
		methodSuccessCalled = true
	}, func(err error) {
		methodFailureCalled = true
	})
	if methodSuccessCalled || !methodFailureCalled {
		t.Fatal("expected errRes.Match to invoke only onFailure")
	}

	var matchedErrSideEffect error
	chained := MatchErr(errRes, func(err error) {
		matchedErrSideEffect = err
	})
	if !chained.IsErr() || !errors.Is(matchedErrSideEffect, errSample) {
		t.Fatal("expected MatchErr package func to capture error")
	}

	var methodSideEffect error
	errRes.MatchErr(func(err error) {
		methodSideEffect = err
	})
	if !errors.Is(methodSideEffect, errSample) {
		t.Fatal("expected errRes.MatchErr method to capture error")
	}

	var okSideEffect error
	okRes.MatchErr(func(err error) {
		okSideEffect = err
	})
	if okSideEffect != nil {
		t.Fatal("expected okRes.MatchErr not to invoke callback")
	}

	// Boundary: nil callbacks should not panic
	okRes.Match(nil, nil)
	errRes.Match(nil, nil)
	nilMatchedOk := Match[int, string](okRes, nil, nil)
	if nilMatchedOk != "" {
		t.Fatal("expected empty string for nil Match callback on Ok")
	}
	nilMatchedErr := Match[int, string](errRes, nil, nil)
	if nilMatchedErr != "" {
		t.Fatal("expected empty string for nil Match callback on Err")
	}
	chainedNil := MatchErr(errRes, nil)
	if !chainedNil.IsErr() {
		t.Fatal("expected errRes preserved on nil MatchErr callback")
	}
	methodNil := errRes.MatchErr(nil)
	if !methodNil.IsErr() {
		t.Fatal("expected errRes preserved on nil method MatchErr callback")
	}
}

func TestApply_WithCoordinatorMetrics(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	coordinator := &cycle173MockCoordinator{}

	// Successful Apply with metrics emission
	okResult := Apply(ctx, 10, func(x int) (int, error) {
		return x * 2, nil
	}, WithCoordinator(coordinator), WithOperationType("metric_success_test"))

	if !okResult.IsOk() || okResult.Unwrap() != 20 {
		t.Fatalf("expected 20, got %v", okResult)
	}
	if coordinator.count() != 1 {
		t.Fatalf("expected 1 event, got %d", coordinator.count())
	}
	lastEvt := coordinator.lastEvent()
	if lastEvt == nil || lastEvt.OperationType != "metric_success_test" || lastEvt.Status != "complete" {
		t.Fatalf("unexpected event context: %+v", lastEvt)
	}

	// Failing Apply with metrics emission
	errExpected := errors.New("apply failure")
	errResult := Apply(ctx, 10, func(x int) (int, error) {
		return 0, errExpected
	}, WithCoordinator(coordinator), WithOperationType("metric_failure_test"))

	if !errResult.IsErr() || !errors.Is(errResult.Err(), errExpected) {
		t.Fatalf("expected errExpected, got %v", errResult)
	}
	if coordinator.count() != 2 {
		t.Fatalf("expected 2 events, got %d", coordinator.count())
	}
	lastEvt = coordinator.lastEvent()
	if lastEvt == nil || lastEvt.OperationType != "metric_failure_test" || lastEvt.Status != "error" {
		t.Fatalf("unexpected event context: %+v", lastEvt)
	}
}

func TestApplyOrElse_ChainsAndCombinators(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// ApplyOrElse: Ok path skips fallback
	okResult := ApplyOrElse(ctx, 5, func(x int) (int, error) {
		return x * 2, nil
	}, func(err error) (int, error) {
		return 999, nil
	}, WithoutMetrics())

	if !okResult.IsOk() || okResult.Unwrap() != 10 {
		t.Fatalf("expected 10, got %v", okResult)
	}

	// ApplyOrElse: Err path triggers fallback
	fallbackResult := ApplyOrElse(ctx, 5, func(x int) (int, error) {
		return 0, errors.New("primary failed")
	}, func(err error) (int, error) {
		return 42, nil
	}, WithoutMetrics())

	if !fallbackResult.IsOk() || fallbackResult.Unwrap() != 42 {
		t.Fatalf("expected fallback 42, got %v", fallbackResult)
	}

	// ApplyAndThen: second stage fails
	chainErr := ApplyAndThen(ctx, 5, func(x int) (int, error) {
		return x + 1, nil
	}, func(x int) (string, error) {
		return "", errors.New("second step failed")
	}, WithoutMetrics())

	if !chainErr.IsErr() {
		t.Fatal("expected second step failure in ApplyAndThen")
	}
}

func TestDoAndGet_TelemetryWrappers(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// Do: returns error
	doErr := Do(ctx, "target", func(target string) error {
		return errors.New("do failure")
	}, WithoutMetrics())
	if doErr == nil {
		t.Fatal("expected do failure")
	}

	// DoOrElse: success path returns nil without fallback
	doSuccess := DoOrElse(ctx, "target", func(target string) error {
		return nil
	}, func(err error) error {
		return errors.New("should not run")
	}, WithoutMetrics())
	if doSuccess != nil {
		t.Fatalf("expected nil error, got %v", doSuccess)
	}

	// DoAndThen: second function returns error
	doAndThenErr := DoAndThen(ctx, 5, func(x int) (int, error) {
		return x * 2, nil
	}, func(x int) error {
		return errors.New("second do failed")
	}, WithoutMetrics())
	if doAndThenErr == nil {
		t.Fatal("expected second do failure")
	}

	// Get & GetOrElse: success
	getOk := Get(ctx, func() (string, error) {
		return "hello", nil
	}, WithoutMetrics())
	if !getOk.IsOk() || getOk.Unwrap() != "hello" {
		t.Fatalf("expected hello, got %v", getOk)
	}

	getVal := GetOrElse(ctx, func() (string, error) {
		return "direct", nil
	}, "fallback", WithoutMetrics())
	if getVal != "direct" {
		t.Fatalf("expected direct, got %s", getVal)
	}
}

func TestMaps_ExtendedCombinators(t *testing.T) {
	t.Parallel()

	// FilterMap
	sampleMap := map[string]int{"a": 1, "b": 2, "c": 3, "d": 4}
	evens := FilterMap(sampleMap, func(k string, v int) bool {
		return v%2 == 0
	})
	if len(evens) != 2 || evens["b"] != 2 || evens["d"] != 4 {
		t.Fatalf("unexpected FilterMap evens: %v", evens)
	}
	if FilterMap(map[string]int(nil), func(k string, v int) bool { return true }) != nil {
		t.Fatal("expected FilterMap on nil map to return nil")
	}

	// TransformMap
	transformed := TransformMap(sampleMap, func(v int) string {
		return fmt.Sprintf("val-%d", v)
	})
	if len(transformed) != 4 || transformed["a"] != "val-1" || transformed["c"] != "val-3" {
		t.Fatalf("unexpected TransformMap result: %v", transformed)
	}
	if TransformMap(map[string]int(nil), func(v int) string { return "" }) != nil {
		t.Fatal("expected TransformMap on nil map to return nil")
	}

	// MergeMaps
	m1 := map[string]int{"a": 1, "b": 2}
	m2 := map[string]int{"b": 20, "c": 30}
	merged := MergeMaps(m1, m2)
	if len(merged) != 3 || merged["a"] != 1 || merged["b"] != 20 || merged["c"] != 30 {
		t.Fatalf("unexpected MergeMaps result: %v", merged)
	}
	if MergeMaps[string, int]() != nil {
		t.Fatal("expected MergeMaps with no args to return nil")
	}
	if MergeMaps[string, int](nil, nil) != nil {
		t.Fatal("expected MergeMaps with all nils to return nil")
	}

	// InvertMap
	invertInput := map[string]int{"alpha": 100, "beta": 200}
	inverted := InvertMap(invertInput)
	if len(inverted) != 2 || inverted[100] != "alpha" || inverted[200] != "beta" {
		t.Fatalf("unexpected InvertMap result: %v", inverted)
	}
	if InvertMap[string, int](nil) != nil {
		t.Fatal("expected InvertMap on nil map to return nil")
	}

	// Pick
	picked := Pick(sampleMap, "a", "c", "nonexistent")
	if len(picked) != 2 || picked["a"] != 1 || picked["c"] != 3 {
		t.Fatalf("unexpected Pick result: %v", picked)
	}
	if Pick(map[string]int(nil), "a") != nil {
		t.Fatal("expected Pick on nil map to return nil")
	}

	// Omit
	omitted := Omit(sampleMap, "b", "d", "missing")
	if len(omitted) != 2 || omitted["a"] != 1 || omitted["c"] != 3 {
		t.Fatalf("unexpected Omit result: %v", omitted)
	}
	if Omit(map[string]int(nil), "a") != nil {
		t.Fatal("expected Omit on nil map to return nil")
	}
}

func TestWhen_FunctionalConditionalChaining(t *testing.T) {
	t.Parallel()

	var evaluatedFirst, evaluatedSecond, evaluatedFallback bool

	When(func() bool { return false }).
		Then(func() { evaluatedFirst = true }).
		OrElseWhen(func() bool { return true }).
		Then(func() { evaluatedSecond = true }).
		OrElse(func() { evaluatedFallback = true }).
		Run()

	if evaluatedFirst || !evaluatedSecond || evaluatedFallback {
		t.Fatalf("unexpected conditional execution: first=%v, second=%v, fallback=%v",
			evaluatedFirst, evaluatedSecond, evaluatedFallback)
	}

	evaluatedFallback = false
	When(func() bool { return false }).
		Then(func() { evaluatedFirst = true }).
		OrElse(func() { evaluatedFallback = true }).
		Run()

	if !evaluatedFallback {
		t.Fatal("expected fallback to execute when condition is false")
	}
}

func TestFunctional_ConcurrentOperations(t *testing.T) {
	t.Parallel()

	const iterations = 50
	var wg sync.WaitGroup
	var completedCount atomic.Int64

	sharedMap := map[string]int{"x": 10, "y": 20, "z": 30}
	okResult := Ok(100)

	for i := 0; i < iterations; i++ {
		wg.Add(1)
		workerID := i
		goroutinelabels.NewGoroutine(
			fmt.Sprintf("functional_concurrency_worker_%d", workerID),
			"concurrent evaluation of functional primitives",
		).StartSimple(func() {
			defer wg.Done()

			// Concurrently inspect Result
			if !okResult.IsOk() || okResult.Unwrap() != 100 {
				return
			}

			// Concurrently transform Result
			mapped := Map(okResult, func(v int) int {
				return v + workerID
			})
			if !mapped.IsOk() || mapped.Unwrap() != 100+workerID {
				return
			}

			// Concurrently flatmap Result
			chained := FlatMap(mapped, func(v int) Result[string] {
				return Ok(fmt.Sprintf("worker-%d-val-%d", workerID, v))
			})
			if !chained.IsOk() {
				return
			}

			// Concurrently transform and filter maps
			filtered := FilterMap(sharedMap, func(k string, v int) bool {
				return v > 15
			})
			if len(filtered) != 2 {
				return
			}

			picked := Pick(sharedMap, "x", "z")
			if len(picked) != 2 {
				return
			}

			completedCount.Add(1)
		})
	}

	wg.Wait()

	if completedCount.Load() != int64(iterations) {
		t.Fatalf("expected all %d concurrent workers to complete successfully, got %d",
			iterations, completedCount.Load())
	}
}
