package cli

import (
	"testing"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
)

func TestCommandExecutionTracker(t *testing.T) {
	t.Parallel()
	tracker := NewCommandExecutionTracker()

	// Test initial state
	if tracker.StartTime.IsZero() {
		t.Error("StartTime should be set")
	}

	// Test SetCommand
	tracker.SetCommand("zqk object create", []string{"backlog_item"})
	if tracker.Command != "zqk object create" {
		t.Errorf("Expected command 'zqk object create', got '%s'", tracker.Command)
	}

	// Test SetContext
	tracker.SetContext("PLAN-208", "WS-007", "MIL-036")
	if tracker.PriorityPlan != "PLAN-208" {
		t.Errorf("Expected PriorityPlan 'PLAN-208', got '%s'", tracker.PriorityPlan)
	}

	// Test SetActor
	tracker.SetActor("account:system", []string{"admin"})
	if tracker.ActorID != "account:system" {
		t.Errorf("Expected ActorID 'account:system', got '%s'", tracker.ActorID)
	}

	// Test RecordObjectCreated
	tracker.RecordObjectCreated("ITEM-001")
	if len(tracker.ObjectsCreated) != 1 || tracker.ObjectsCreated[0] != "ITEM-001" {
		t.Errorf("Expected ObjectsCreated to contain 'ITEM-001', got %v", tracker.ObjectsCreated)
	}

	// Test RecordObjectUpdated
	tracker.RecordObjectUpdated("ITEM-001")
	if len(tracker.ObjectsUpdated) != 1 || tracker.ObjectsUpdated[0] != "ITEM-001" {
		t.Errorf("Expected ObjectsUpdated to contain 'ITEM-001', got %v", tracker.ObjectsUpdated)
	}

	// Test RecordObjectDeleted
	tracker.RecordObjectDeleted("ITEM-001")
	if len(tracker.ObjectsDeleted) != 1 || tracker.ObjectsDeleted[0] != "ITEM-001" {
		t.Errorf("Expected ObjectsDeleted to contain 'ITEM-001', got %v", tracker.ObjectsDeleted)
	}

	// Test SetOutcome
	tracker.SetOutcome(true, 0, nil, false)
	if !tracker.Success {
		t.Error("Expected Success to be true")
	}
	if tracker.EndTime.IsZero() {
		t.Error("EndTime should be set after SetOutcome")
	}

	// Test ToCommandMetric
	metric := tracker.ToCommandMetric()
	if metric.Command != "zqk object create" {
		t.Errorf("Expected metric.Command 'zqk object create', got '%s'", metric.Command)
	}
	if metric.PriorityPlan != "PLAN-208" {
		t.Errorf("Expected metric.PriorityPlan 'PLAN-208', got '%s'", metric.PriorityPlan)
	}
	if len(metric.ObjectsCreated) != 1 {
		t.Errorf("Expected 1 object created, got %d", len(metric.ObjectsCreated))
	}
}

func TestCommandExecutionTracker_Context(t *testing.T) {
	t.Parallel()
	tracker := NewCommandExecutionTracker()
	ctx := pkgctx.NewSystemContext()

	// Test WithTracker
	ctxWithTracker := WithTracker(ctx, tracker)

	// Test GetTrackerFromContext
	retrievedTracker := GetTrackerFromContext(ctxWithTracker)
	if retrievedTracker == nil {
		t.Error("Expected to retrieve tracker from context")
	}
	if retrievedTracker != tracker {
		t.Error("Retrieved tracker should be the same instance")
	}

	// Test GetTrackerFromContext with nil context
	retrievedTracker = GetTrackerFromContext(ctx)
	if retrievedTracker != nil {
		t.Error("Expected nil tracker from context without tracker")
	}
}

func TestCommandExecutionTracker_Concurrent(t *testing.T) {
	t.Parallel()
	tracker := NewCommandExecutionTracker()

	// Test concurrent access
	done := make(chan bool)

	// Concurrent writes
	goroutinelabels.StartTestGoroutine("test_record_created", "recording object created events in test", func() {
		for i := 0; i < 100; i++ {
			tracker.RecordObjectCreated("ITEM-001")
		}
		done <- true
	})

	goroutinelabels.StartTestGoroutine("test_record_updated", "recording object updated events in test", func() {
		for i := 0; i < 100; i++ {
			tracker.RecordObjectUpdated("ITEM-001")
		}
		done <- true
	})

	goroutinelabels.StartTestGoroutine("test_record_deleted", "recording object deleted events in test", func() {
		for i := 0; i < 100; i++ {
			tracker.RecordObjectDeleted("ITEM-001")
		}
		done <- true
	})

	// Wait for all goroutines
	<-done
	<-done
	<-done

	// Verify all operations were recorded
	if len(tracker.ObjectsCreated) != 100 {
		t.Errorf("Expected 100 created objects, got %d", len(tracker.ObjectsCreated))
	}
	if len(tracker.ObjectsUpdated) != 100 {
		t.Errorf("Expected 100 updated objects, got %d", len(tracker.ObjectsUpdated))
	}
	if len(tracker.ObjectsDeleted) != 100 {
		t.Errorf("Expected 100 deleted objects, got %d", len(tracker.ObjectsDeleted))
	}
}

func TestCommandExecutionTracker_Timing(t *testing.T) {
	t.Parallel()
	tracker := NewCommandExecutionTracker()

	startTime := tracker.StartTime
	time.Sleep(10 * time.Millisecond)

	tracker.SetOutcome(true, 0, nil, false)

	if tracker.EndTime.Before(startTime) {
		t.Error("EndTime should be after StartTime")
	}

	metric := tracker.ToCommandMetric()
	if metric.Duration <= 0 {
		t.Error("Duration should be positive")
	}
	if metric.StartTime != startTime {
		t.Error("StartTime should match")
	}
	if metric.EndTime != tracker.EndTime {
		t.Error("EndTime should match")
	}
}
