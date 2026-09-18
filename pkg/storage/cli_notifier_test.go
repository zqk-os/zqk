package storage

import (
	"context"
	"testing"
	"time"
)

type dummyEventEmitter struct{}

func (d *dummyEventEmitter) EmitProgress(ctx context.Context, operationID, operationType string, progress, total int, message string, fields map[string]any, emitAudit bool) error {
	return nil
}

func (d *dummyEventEmitter) EmitStatusChange(ctx context.Context, operationID, operationType, oldStatus, newStatus, message string, fields map[string]any) error {
	return nil
}

func (d *dummyEventEmitter) EmitError(ctx context.Context, operationID, operationType string, err error, message string, fields map[string]any) error {
	return nil
}

func (d *dummyEventEmitter) EmitCompletion(ctx context.Context, operationID, operationType string, duration time.Duration, message string, fields map[string]any) error {
	return nil
}

func TestNewCLINotifier_Errors(t *testing.T) {
	// 1. Nil event emitter should return error, not panic
	notifier, err := NewCLINotifier(false, false, nil)
	if err == nil {
		t.Fatal("expected error with nil eventEmitter, got nil")
	}
	if notifier != nil {
		t.Errorf("expected nil notifier on error, got %v", notifier)
	}

	// 2. Non-nil event emitter succeeds
	notifier, err = NewCLINotifier(true, false, &dummyEventEmitter{})
	if err != nil {
		t.Fatalf("unexpected error with valid eventEmitter: %v", err)
	}
	if notifier == nil {
		t.Fatal("expected non-nil CLINotifier")
	}
}
