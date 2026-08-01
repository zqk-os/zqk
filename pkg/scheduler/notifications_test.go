package scheduler

import (
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
)

func TestNotificationDisplay_TerminalNotification(t *testing.T) {
	t.Parallel()
	logger := logging.GetLoggerFromProfile("test")
	display := NewNotificationDisplay(logger)

	notif := CreateJobNotification(
		"SCH-001",
		"run_wrapper",
		"infrastructure",
		"completed",
		PriorityMedium,
		30*time.Second,
		nil,
		map[string]any{
			objects.FieldKeyCommand: "terraform apply",
		},
	)

	// Should not panic
	display.Display(notif)
}

func TestNotificationDisplay_DesktopNotification(t *testing.T) {
	t.Parallel()
	logger := logging.GetLoggerFromProfile("test")
	display := NewNotificationDisplay(logger)

	notif := CreateJobNotification(
		"SCH-002",
		"run_wrapper",
		"deployment",
		"failed",
		PriorityHigh,
		45*time.Second,
		&testError{message: "command failed"},
		map[string]any{
			objects.FieldKeyCommand: "kubectl apply",
			"error":                 "connection timeout",
		},
	)

	// Should not panic (desktop notification is best-effort)
	display.Display(notif)
}

func TestCreateJobNotification_Completed(t *testing.T) {
	t.Parallel()
	notif := CreateJobNotification(
		"SCH-003",
		"run_wrapper",
		"testing",
		"completed",
		PriorityMedium,
		10*time.Second,
		nil,
		nil,
	)

	if notif.JobID != "SCH-003" {
		t.Errorf("Expected JobID SCH-003, got %s", notif.JobID)
	}
	if notif.Event != "completed" {
		t.Errorf("Expected event 'completed', got %s", notif.Event)
	}
	if notif.Title != "Job Completed Successfully" {
		t.Errorf("Expected title 'Job Completed Successfully', got %s", notif.Title)
	}
	if notif.Duration != 10*time.Second {
		t.Errorf("Expected duration 10s, got %v", notif.Duration)
	}
}

func TestCreateJobNotification_Failed(t *testing.T) {
	t.Parallel()
	err := &testError{message: "test error"}
	notif := CreateJobNotification(
		"SCH-004",
		"run_wrapper",
		"testing",
		"failed",
		PriorityLow, // Should be upgraded to Medium
		time.Duration(0),
		err,
		nil,
	)

	if notif.Event != "failed" {
		t.Errorf("Expected event 'failed', got %s", notif.Event)
	}
	if notif.Priority != PriorityMedium {
		t.Errorf("Expected priority Medium (upgraded from Low), got %s", notif.Priority)
	}
	if notif.Error != err {
		t.Errorf("Expected error to be set")
	}
	if notif.Title != "Job Failed" {
		t.Errorf("Expected title 'Job Failed', got %s", notif.Title)
	}
}

func TestCreateJobNotification_StatusUpdate(t *testing.T) {
	t.Parallel()
	notif := CreateJobNotification(
		"SCH-005",
		"run_wrapper",
		"testing",
		"status_update",
		PriorityLow,
		time.Duration(0),
		nil,
		map[string]any{
			objects.FieldKeyStatus: "in_progress",
		},
	)

	if notif.Event != "status_update" {
		t.Errorf("Expected event 'status_update', got %s", notif.Event)
	}
	if notif.Message == emptyValue {
		t.Error("Expected message to be set for status update")
	}
}

type testError struct {
	message string
}

func (e *testError) Error() string {
	return e.message
}
