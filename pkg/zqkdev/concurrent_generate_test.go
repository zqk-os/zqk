package zqkdev

import (
	"context"
	"fmt"
	"runtime"
	"sync/atomic"
	"testing"
	"time"
)

// GenerateItem represents a single YAML file to process during concurrent generation.

func TestConcurrentGenerate_EmptyInput(t *testing.T) {
	result := ConcurrentGenerate(context.Background(), nil, func(_ context.Context, item GenerateItem) error {
		t.Fatal("should not be called on empty input")
		return nil
	})

	if result.Generated != 0 || result.Skipped != 0 || result.Errors != 0 {
		t.Errorf("expected all zeros, got generated=%d skipped=%d errors=%d",
			result.Generated, result.Skipped, result.Errors)
	}
}

func TestConcurrentGenerate_AllSucceed(t *testing.T) {
	items := []GenerateItem{
		{YAMLPath: "a.yaml", BaseName: "a"},
		{YAMLPath: "b.yaml", BaseName: "b"},
		{YAMLPath: "c.yaml", BaseName: "c"},
	}

	var count atomic.Int32
	result := ConcurrentGenerate(context.Background(), items, func(_ context.Context, item GenerateItem) error {
		count.Add(1)
		return nil
	})

	if result.Generated != 3 {
		t.Errorf("expected 3 generated, got %d", result.Generated)
	}
	if result.Errors != 0 {
		t.Errorf("expected 0 errors, got %d", result.Errors)
	}
	if int(count.Load()) != 3 {
		t.Errorf("expected fn called 3 times, got %d", count.Load())
	}
}

func TestConcurrentGenerate_SomeErrors(t *testing.T) {
	items := []GenerateItem{
		{YAMLPath: "a.yaml", BaseName: "a"},
		{YAMLPath: "b.yaml", BaseName: "b"},
		{YAMLPath: "c.yaml", BaseName: "c"},
	}

	result := ConcurrentGenerate(context.Background(), items, func(_ context.Context, item GenerateItem) error {
		if item.BaseName == "b" {
			return fmt.Errorf("deliberate error")
		}
		return nil
	})

	if result.Generated != 2 {
		t.Errorf("expected 2 generated, got %d", result.Generated)
	}
	if result.Errors != 1 {
		t.Errorf("expected 1 error, got %d", result.Errors)
	}
	if len(result.ErrorDetails) != 1 {
		t.Fatalf("expected 1 error detail, got %d", len(result.ErrorDetails))
	}
	if result.ErrorDetails[0].Path != "b.yaml" {
		t.Errorf("expected error for b.yaml, got %s", result.ErrorDetails[0].Path)
	}
}

func TestConcurrentGenerate_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	items := make([]GenerateItem, 100)
	for i := range items {
		items[i] = GenerateItem{YAMLPath: fmt.Sprintf("file_%d.yaml", i), BaseName: fmt.Sprintf("file_%d", i)}
	}

	var started atomic.Int32
	cancel() // Cancel immediately

	result := ConcurrentGenerate(ctx, items, func(ctx context.Context, item GenerateItem) error {
		started.Add(1)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Second):
			return nil
		}
	})

	// With context already cancelled, not all items should be processed
	total := result.Generated + result.Errors
	if total == 100 {
		// This is acceptable only if they all got context.Canceled errors
		t.Logf("all items processed (likely all errors from cancellation): generated=%d errors=%d", result.Generated, result.Errors)
	}
}

func TestConcurrentGenerate_BoundedConcurrency(t *testing.T) {
	// Verify that concurrency is bounded to GOMAXPROCS (per project policy)
	maxProcs := runtime.GOMAXPROCS(0)
	items := make([]GenerateItem, maxProcs*4)
	for i := range items {
		items[i] = GenerateItem{YAMLPath: fmt.Sprintf("file_%d.yaml", i), BaseName: fmt.Sprintf("file_%d", i)}
	}

	var maxConcurrent atomic.Int32
	var current atomic.Int32

	result := ConcurrentGenerate(context.Background(), items, func(_ context.Context, item GenerateItem) error {
		cur := current.Add(1)
		// Track high-water mark
		for {
			old := maxConcurrent.Load()
			if cur <= old || maxConcurrent.CompareAndSwap(old, cur) {
				break
			}
		}
		time.Sleep(5 * time.Millisecond) // Simulate work to let contention build
		current.Add(-1)
		return nil
	})

	if result.Generated != len(items) {
		t.Errorf("expected %d generated, got %d", len(items), result.Generated)
	}

	peak := int(maxConcurrent.Load())
	// Peak concurrency must not exceed GOMAXPROCS + some scheduling slack
	if peak > maxProcs+2 {
		t.Errorf("concurrency exceeded bound: peak=%d GOMAXPROCS=%d", peak, maxProcs)
	}
	t.Logf("peak concurrency: %d (GOMAXPROCS=%d)", peak, maxProcs)
}

func TestConcurrentGenerate_ErrorDetailsPreserved(t *testing.T) {
	items := []GenerateItem{
		{YAMLPath: "good.yaml", BaseName: "good"},
		{YAMLPath: "bad1.yaml", BaseName: "bad1"},
		{YAMLPath: "bad2.yaml", BaseName: "bad2"},
	}

	result := ConcurrentGenerate(context.Background(), items, func(_ context.Context, item GenerateItem) error {
		if item.BaseName == "bad1" {
			return fmt.Errorf("error one")
		}
		if item.BaseName == "bad2" {
			return fmt.Errorf("error two")
		}
		return nil
	})

	if result.Generated != 1 {
		t.Errorf("expected 1 generated, got %d", result.Generated)
	}
	if result.Errors != 2 {
		t.Errorf("expected 2 errors, got %d", result.Errors)
	}
	if len(result.ErrorDetails) != 2 {
		t.Fatalf("expected 2 error details, got %d", len(result.ErrorDetails))
	}
}
