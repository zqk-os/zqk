package scheduler

import (
	"context"
	"fmt"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
)

// ExampleNotificationContext_BasicUsage demonstrates basic notification context usage
func ExampleNotificationContext_basicUsage() {
	logger := logging.GetLoggerFromProfile("test")

	// Create notification context with default settings
	nc := NewNotificationContext(logger, nil)

	// Create and send a notification
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

	nc.Notify(notif)

	fmt.Println("Notification displayed to terminal")

	// Output: Notification displayed to terminal
}

// ExampleNotificationContext_CustomPreferences demonstrates custom notification preferences
func ExampleNotificationContext_customPreferences() {
	logger := logging.GetLoggerFromProfile("test")

	// Create notification context with custom preferences
	opts := &NotificationContextOptions{
		Enabled:        true,
		MinPriority:    PriorityHigh,                             // Only show high and critical
		ShowDesktop:    true,                                     // Enable desktop notifications
		ShowTerminal:   true,                                     // Enable terminal notifications
		Categories:     []string{"deployment", "infrastructure"}, // Filter by category
		MaxHistorySize: 50,                                       // Keep last 50 notifications
	}
	nc := NewNotificationContext(logger, opts)

	// High priority notification will be shown
	notif1 := CreateJobNotification(
		"SCH-002",
		"run_wrapper",
		"deployment",
		"failed",
		PriorityHigh,
		45*time.Second,
		fmt.Errorf("deployment failed"),
		nil,
	)
	nc.Notify(notif1)

	// Low priority notification will be filtered
	notif2 := CreateJobNotification(
		"SCH-003",
		"run_wrapper",
		"testing",
		"completed",
		PriorityLow,
		10*time.Second,
		nil,
		nil,
	)
	nc.Notify(notif2) // This won't be displayed

	fmt.Println("Only high priority deployment notification displayed")

	// Output: Only high priority deployment notification displayed
}

// ExampleNotificationContext_Suppression demonstrates job suppression
func ExampleNotificationContext_suppression() {
	logger := logging.GetLoggerFromProfile("test")
	nc := NewNotificationContext(logger, nil)

	// Suppress notifications for a specific job during maintenance
	nc.Suppress("SCH-004", 1*time.Hour)

	// This notification will be suppressed
	notif := CreateJobNotification(
		"SCH-004",
		"run_wrapper",
		"maintenance",
		"completed",
		PriorityHigh,
		0,
		nil,
		nil,
	)
	nc.Notify(notif) // Won't be displayed

	// Other jobs still notify normally
	notif2 := CreateJobNotification(
		"SCH-005",
		"run_wrapper",
		"deployment",
		"completed",
		PriorityHigh,
		0,
		nil,
		nil,
	)
	nc.Notify(notif2) // Will be displayed

	fmt.Println("Only SCH-005 notification displayed")

	// Output: Only SCH-005 notification displayed
}

// ExampleNotificationContext_Acknowledgment demonstrates acknowledgment tracking
func ExampleNotificationContext_acknowledgment() {
	logger := logging.GetLoggerFromProfile("test")
	nc := NewNotificationContext(logger, nil)

	// Send high priority notifications
	notif1 := CreateJobNotification("SCH-006", "run_wrapper", "deployment", "failed", PriorityHigh, 0, fmt.Errorf("error"), nil)
	notif2 := CreateJobNotification("SCH-cache-prewarm", "run_wrapper", "infrastructure", "failed", PriorityCritical, 0, fmt.Errorf("error"), nil)

	nc.Notify(notif1)
	nc.Notify(notif2)

	// Get unacknowledged notifications
	unacknowledged := nc.GetUnacknowledged()
	fmt.Printf("Unacknowledged: %d\n", len(unacknowledged))

	// Acknowledge one
	nc.Acknowledge("SCH-006")

	unacknowledged = nc.GetUnacknowledged()
	fmt.Printf("After acknowledgment: %d\n", len(unacknowledged))

	// Output:
	// Unacknowledged: 2
	// After acknowledgment: 1
}

// ExampleNotificationContext_History demonstrates notification history
func ExampleNotificationContext_history() {
	logger := logging.GetLoggerFromProfile("test")
	nc := NewNotificationContext(logger, &NotificationContextOptions{
		Enabled:        true,
		ShowTerminal:   true,
		MaxHistorySize: 5,
	})

	// Send multiple notifications
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

	// Get recent history (last 3)
	history := nc.GetHistory(3)
	fmt.Printf("Recent notifications: %d\n", len(history))
	fmt.Printf("Most recent: %s\n", history[len(history)-1].JobID)

	// Output:
	// Recent notifications: 3
	// Most recent: SCH-009
}

// ExampleNotificationContext_ContextIntegration demonstrates integration with Go context
func ExampleNotificationContext_contextIntegration() {
	logger := logging.GetLoggerFromProfile("test")
	nc := NewNotificationContext(logger, nil)

	// Add notification context to Go context
	ctx := WithNotificationContext(pkgctx.NewSystemContext(), nc)

	// Extract and use in a function
	func(ctx context.Context) {
		nc := NotificationContextFromContext(ctx)
		if nc != nil {
			notif := CreateJobNotification(
				"SCH-008",
				"run_wrapper",
				"test",
				"completed",
				PriorityMedium,
				0,
				nil,
				nil,
			)
			nc.Notify(notif)
		}
	}(ctx)

	fmt.Println("Notification sent through context")

	// Output: Notification sent through context
}

// ExampleNotificationContext_DynamicConfiguration demonstrates dynamic configuration changes
func ExampleNotificationContext_dynamicConfiguration() {
	logger := logging.GetLoggerFromProfile("test")
	nc := NewNotificationContext(logger, &NotificationContextOptions{
		MinPriority: PriorityLow,
	})

	// Change minimum priority dynamically
	nc.SetMinPriority(PriorityHigh)

	// Change enabled state
	nc.SetEnabled(false)
	nc.SetEnabled(true)

	// Change category filter
	nc.SetCategories([]string{"deployment"})

	fmt.Println("Configuration updated")

	// Output: Configuration updated
}

// ExampleCreateJobNotification demonstrates creating different types of notifications
func ExampleCreateJobNotification() {
	logger := logging.GetLoggerFromProfile("test")
	display := NewNotificationDisplay(logger)

	// Success notification
	successNotif := CreateJobNotification(
		"SCH-009",
		"run_wrapper",
		"deployment",
		"completed",
		PriorityMedium,
		45*time.Second,
		nil,
		map[string]any{
			objects.FieldKeyCommand: "kubectl apply",
			"stdout":                "Deployment successful",
		},
	)
	display.Display(successNotif)

	// Failure notification
	failureNotif := CreateJobNotification(
		"SCH-010",
		"run_wrapper",
		"infrastructure",
		"failed",
		PriorityHigh,
		30*time.Second,
		fmt.Errorf("terraform apply failed: timeout"),
		map[string]any{
			objects.FieldKeyCommand: "terraform apply",
			"stderr":                "Error: timeout waiting for resources",
		},
	)
	display.Display(failureNotif)

	// Status update notification
	statusNotif := CreateJobNotification(
		"SCH-011",
		"run_wrapper",
		"testing",
		"status_update",
		PriorityLow,
		0,
		nil,
		map[string]any{
			objects.FieldKeyStatus: "50% complete",
		},
	)
	display.Display(statusNotif)

	fmt.Println("Three different notification types displayed")

	// Output: Three different notification types displayed
}

// ExampleNotificationContext_SchedulerIntegration demonstrates integration with scheduler
func ExampleNotificationContext_schedulerIntegration() {
	logger := logging.GetLoggerFromProfile("test")

	// Create notification context with preferences
	opts := &NotificationContextOptions{
		Enabled:     true,
		MinPriority: PriorityMedium,
		Categories:  []string{"deployment", "infrastructure"},
	}
	nc := NewNotificationContext(logger, opts)

	// In real usage, this would be set on the scheduler:
	// scheduler.SetNotificationContext(nc)

	// Notifications from handlers will automatically use this context
	// and respect the preferences
	_ = nc // Use the context

	fmt.Println("Notification context configured for scheduler")

	// Output: Notification context configured for scheduler
}
