package scheduler

import (
	"fmt"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
)

func TestNotificationContext_ShouldNotify(t *testing.T) {
	t.Parallel()
	logger := logging.GetLoggerFromProfile("test")
	nc := NewNotificationContext(logger, &NotificationContextOptions{
		Enabled:     true,
		MinPriority: PriorityMedium,
	})
	t.Cleanup(func() { nc.Stop() })

	// High priority should notify
	notif := CreateJobNotification("SCH-001", "run_wrapper", "test", "completed", PriorityHigh, 0, nil, nil)
	if !nc.ShouldNotify(notif) {
		t.Error("Expected high priority notification to be shown")
	}

	// Low priority should not notify
	notif = CreateJobNotification("SCH-002", "run_wrapper", "test", "completed", PriorityLow, 0, nil, nil)
	if nc.ShouldNotify(notif) {
		t.Error("Expected low priority notification to be filtered out")
	}
}

func TestNotificationContext_CategoryFilter(t *testing.T) {
	t.Parallel()
	logger := logging.GetLoggerFromProfile("test")
	nc := NewNotificationContext(logger, &NotificationContextOptions{
		Enabled:    true,
		Categories: []string{"infrastructure", "deployment"},
	})
	t.Cleanup(func() { nc.Stop() })

	// Allowed category should notify
	notif := CreateJobNotification("SCH-001", "run_wrapper", "infrastructure", "completed", PriorityMedium, 0, nil, nil)
	if !nc.ShouldNotify(notif) {
		t.Error("Expected infrastructure category notification to be shown")
	}

	// Disallowed category should not notify
	notif = CreateJobNotification("SCH-002", "run_wrapper", "testing", "completed", PriorityMedium, 0, nil, nil)
	if nc.ShouldNotify(notif) {
		t.Error("Expected testing category notification to be filtered out")
	}
}

func TestNotificationContext_Suppression(t *testing.T) {
	t.Parallel()
	logger := logging.GetLoggerFromProfile("test")
	nc := NewNotificationContext(logger, &NotificationContextOptions{
		ShowTerminal: false,
		ShowDesktop:  false,
		MinPriority:  PriorityLow, // Allow all priorities
		Enabled:      true,
	})
	t.Cleanup(func() { nc.Stop() })

	// Suppress a job for 1 hour
	nc.Suppress("SCH-001", 1*time.Hour)

	notif := CreateJobNotification("SCH-001", "run_wrapper", "test", "completed", PriorityHigh, 0, nil, nil)
	if nc.ShouldNotify(notif) {
		t.Error("Expected suppressed job notification to be filtered out")
	}

	// Other jobs should still notify
	notif = CreateJobNotification("SCH-002", "run_wrapper", "test", "completed", PriorityHigh, 0, nil, nil)
	if !nc.ShouldNotify(notif) {
		t.Error("Expected non-suppressed job notification to be shown")
	}
}

func TestNotificationContext_History(t *testing.T) {
	t.Parallel()
	logger := logging.GetLoggerFromProfile("test")
	nc := NewNotificationContext(logger, &NotificationContextOptions{
		MaxHistorySize: 5,
		ShowTerminal:   false,
		ShowDesktop:    false,
		MinPriority:    PriorityLow, // Allow all priorities
		Enabled:        true,
	})

	// Add notifications (they will be added to history if they pass ShouldNotify)
	for i := 0; i < 10; i++ {
		notif := CreateJobNotification(
			fmt.Sprintf("SCH-%03d", i),
			"run_wrapper",
			"test",
			"completed",
			PriorityMedium,
			time.Duration(i)*time.Second,
			nil,
			nil,
		)
		nc.Notify(notif)
	}

	// Wait a bit for delivery loop to process
	time.Sleep(200 * time.Millisecond)

	// Should only keep last 5
	history := nc.GetHistory(0) // 0 = all
	if len(history) != 5 {
		t.Errorf("Expected history size 5, got %d", len(history))
	}

	// Most recent should be last
	if len(history) > 0 && history[len(history)-1].Duration != 9*time.Second {
		t.Errorf("Expected most recent notification to have duration 9s, got %v", history[len(history)-1].Duration)
	}
}

func TestNotificationContext_Unacknowledged(t *testing.T) {
	t.Parallel()
	logger := logging.GetLoggerFromProfile("test")
	nc := NewNotificationContext(logger, &NotificationContextOptions{
		ShowTerminal: false,
		ShowDesktop:  false,
		MinPriority:  PriorityLow, // Allow all priorities
		Enabled:      true,
	})
	t.Cleanup(func() { nc.Stop() })

	// Low priority should not be tracked (only high+ are tracked)
	// Use a unique job ID to avoid conflicts
	notif := CreateJobNotification("SCH-UNACK-001", "run_wrapper", "test", "completed", PriorityLow, 0, nil, nil)
	nc.Notify(notif)
	// No sleep needed - unacknowledged is updated synchronously
	unacknowledged := nc.GetUnacknowledged()
	if len(unacknowledged) != 0 {
		t.Errorf("Expected low priority notification not to be tracked, got %d unacknowledged", len(unacknowledged))
	}

	// High priority should be tracked
	notif = CreateJobNotification("SCH-UNACK-002", "run_wrapper", "test", "failed", PriorityHigh, 0, nil, nil)
	// Verify it should notify
	if !nc.ShouldNotify(notif) {
		t.Fatal("High priority notification should pass ShouldNotify check")
	}
	nc.Notify(notif)
	// No sleep needed - unacknowledged is updated synchronously
	unacknowledged = nc.GetUnacknowledged()
	if len(unacknowledged) != 1 {
		t.Errorf("Expected 1 unacknowledged notification, got %d (priority: %s, should notify: %v)",
			len(unacknowledged), notif.Priority, nc.ShouldNotify(notif))
	}
	if len(unacknowledged) > 0 && unacknowledged[0].JobID != "SCH-UNACK-002" {
		t.Errorf("Expected unacknowledged job SCH-UNACK-002, got %s", unacknowledged[0].JobID)
	}

	// Acknowledge
	nc.Acknowledge("SCH-UNACK-002")
	unacknowledged = nc.GetUnacknowledged()
	if len(unacknowledged) != 0 {
		t.Errorf("Expected no unacknowledged notifications after acknowledgment, got %d", len(unacknowledged))
	}
}

func TestNotificationContext_Disabled(t *testing.T) {
	t.Parallel()
	logger := logging.GetLoggerFromProfile("test")
	nc := NewNotificationContext(logger, &NotificationContextOptions{
		Enabled: false,
	})

	notif := CreateJobNotification("SCH-001", "run_wrapper", "test", "completed", PriorityHigh, 0, nil, nil)
	if nc.ShouldNotify(notif) {
		t.Error("Expected notification to be filtered when disabled")
	}
}

func TestNotificationContext_ContextIntegration(t *testing.T) {
	t.Parallel()
	logger := logging.GetLoggerFromProfile("test")
	nc := NewNotificationContext(logger, nil)
	t.Cleanup(func() { nc.Stop() })

	// Add to context
	ctx := WithNotificationContext(pkgctx.NewSystemContext(), nc)

	// Extract from context
	extracted := NotificationContextFromContext(ctx)
	if extracted != nc {
		t.Error("Expected extracted context to match original")
	}

	// Test with nil context
	extracted = NotificationContextFromContext(pkgctx.NewSystemContext())
	if extracted != nil {
		t.Error("Expected nil when context doesn't have notification context")
	}
}

func TestNotificationContext_Channels(t *testing.T) {
	t.Parallel()
	logger := logging.GetLoggerFromProfile("test")
	nc := NewNotificationContext(logger, &NotificationContextOptions{
		ShowTerminal: true,
		ShowDesktop:  true,
	})
	t.Cleanup(func() { nc.Stop() })

	// Verify channels exist
	if nc.TerminalChannel() == nil {
		t.Error("Expected terminal channel to exist")
	}
	if nc.DesktopChannel() == nil {
		t.Error("Expected desktop channel to exist")
	}
	if nc.EventChannel() == nil {
		t.Error("Expected event channel to exist")
	}
}

func TestNotificationContext_SetMinPriority(t *testing.T) {
	t.Parallel()
	logger := logging.GetLoggerFromProfile("test")
	nc := NewNotificationContext(logger, &NotificationContextOptions{
		MinPriority: PriorityLow,
	})
	t.Cleanup(func() { nc.Stop() })

	// Change to high
	nc.SetMinPriority(PriorityHigh)

	notif := CreateJobNotification("SCH-001", "run_wrapper", "test", "completed", PriorityMedium, 0, nil, nil)
	if nc.ShouldNotify(notif) {
		t.Error("Expected medium priority to be filtered after setting min to high")
	}
}

func TestNotificationContext_SetEnabled(t *testing.T) {
	t.Parallel()
	logger := logging.GetLoggerFromProfile("test")
	nc := NewNotificationContext(logger, &NotificationContextOptions{
		Enabled: true,
	})

	notif := CreateJobNotification("SCH-001", "run_wrapper", "test", "completed", PriorityHigh, 0, nil, nil)
	if !nc.ShouldNotify(notif) {
		t.Error("Expected notification when enabled")
	}

	// Disable
	nc.SetEnabled(false)
	if nc.ShouldNotify(notif) {
		t.Error("Expected notification to be filtered when disabled")
	}
}

func TestNotificationContext_SetCategories(t *testing.T) {
	t.Parallel()
	logger := logging.GetLoggerFromProfile("test")
	nc := NewNotificationContext(logger, nil)
	t.Cleanup(func() { nc.Stop() })

	// Set categories
	nc.SetCategories([]string{"infrastructure"})

	notif := CreateJobNotification("SCH-001", "run_wrapper", "infrastructure", "completed", PriorityMedium, 0, nil, nil)
	if !nc.ShouldNotify(notif) {
		t.Error("Expected infrastructure category to be allowed")
	}

	notif = CreateJobNotification("SCH-002", "run_wrapper", "testing", "completed", PriorityMedium, 0, nil, nil)
	if nc.ShouldNotify(notif) {
		t.Error("Expected testing category to be filtered")
	}
}
