package community

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestInterceptorPipeline_Ordering(t *testing.T) {
	pipe := NewInterceptorPipeline()

	var order []string

	pipe.Use(func(ctx context.Context, req any, next ProviderHandler) (any, error) {
		order = append(order, "mw1-pre")
		res, err := next(ctx, req)
		order = append(order, "mw1-post")
		return res, err
	})

	pipe.Use(func(ctx context.Context, req any, next ProviderHandler) (any, error) {
		order = append(order, "mw2-pre")
		res, err := next(ctx, req)
		order = append(order, "mw2-post")
		return res, err
	})

	handler := func(ctx context.Context, req any) (any, error) {
		order = append(order, "handler")
		return "ok", nil
	}

	res, err := pipe.Execute(context.Background(), "input", handler)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if res != "ok" {
		t.Fatalf("expected 'ok', got %v", res)
	}

	expected := []string{"mw1-pre", "mw2-pre", "handler", "mw2-post", "mw1-post"}
	if len(order) != len(expected) {
		t.Fatalf("expected length %d, got %d: %v", len(expected), len(order), order)
	}
	for i := range expected {
		if order[i] != expected[i] {
			t.Fatalf("expected %s at %d, got %s", expected[i], i, order[i])
		}
	}
}

func TestInterceptorPipeline_ShortCircuit(t *testing.T) {
	pipe := NewInterceptorPipeline()

	pipe.Use(func(ctx context.Context, req any, next ProviderHandler) (any, error) {
		return nil, errors.New("auth rejected")
	})

	handlerExecuted := false
	handler := func(ctx context.Context, req any) (any, error) {
		handlerExecuted = true
		return "data", nil
	}

	_, err := pipe.Execute(context.Background(), "input", handler)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if handlerExecuted {
		t.Fatal("handler should not have executed")
	}
}

func TestInterceptorPipeline_Logging(t *testing.T) {
	pipe := NewInterceptorPipeline()

	var logged []string
	pipe.Use(NewLoggingInterceptor(func(msg string) {
		logged = append(logged, msg)
	}))

	handler := func(ctx context.Context, req any) (any, error) {
		return "result", nil
	}

	_, err := pipe.Execute(context.Background(), "input", handler)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(logged) != 1 || !strings.Contains(logged[0], "[interceptor] success") {
		t.Fatalf("unexpected logs: %v", logged)
	}
}

func TestInterceptorPipeline_Timeout(t *testing.T) {
	pipe := NewInterceptorPipeline()
	pipe.Use(NewTimeoutInterceptor(20 * time.Millisecond))

	handler := func(ctx context.Context, req any) (any, error) {
		select {
		case <-time.After(100 * time.Millisecond):
			return "done", nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	_, err := pipe.Execute(context.Background(), "input", handler)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if !errors.Is(err, ErrExecutionTimeout) {
		t.Fatalf("expected ErrExecutionTimeout, got %v", err)
	}
}
