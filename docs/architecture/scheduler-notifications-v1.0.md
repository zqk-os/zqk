# Scheduler Notification System v1.0

**Last Verified:** 2026-08-31


**Version**: 1.0.0  
**Created**: 2025-01-02  
**Status**: Active  
**Purpose**: User-facing notification system for scheduler job completion and outcomes

## Overview

The scheduler notification system provides attention-grabbing, user-facing notifications when jobs complete, fail, or update status. Notifications are managed through a `NotificationContext` that allows fine-grained control over what, when, and how notifications are displayed.

## Key Features

- **Attention-Grabbing Display**: Color-coded terminal notifications with icons and priority indicators
- **Desktop Notifications**: Platform-native notifications for high-priority events (macOS, Linux, Windows)
- **Notification Context**: Centralized management of preferences, history, and delivery
- **Priority-Based Filtering**: Show only notifications above a priority threshold
- **Category Filtering**: Filter notifications by job category
- **Suppression**: Temporarily suppress notifications for specific jobs
- **Acknowledgment Tracking**: Track unacknowledged high-priority notifications
- **History**: Maintain configurable notification history

## Quick Start

### Basic Usage

```go
import "github.com/lanceman/zqk/pkg/scheduler"

logger := logging.GetLoggerFromProfile("system")

// Create notification context with defaults
nc := scheduler.NewNotificationContext(logger, nil)

// Create and send a notification
notif := scheduler.CreateJobNotification(
    "SCH-001",
    "run_wrapper",
    "infrastructure",
    "completed",
    scheduler.PriorityMedium,
    30*time.Second,
    nil,
    map[string]interface{}{
        "command": "terraform apply",
    },
)

nc.Notify(notif)
// Notification automatically displayed to terminal
```

### Custom Preferences

```go
opts := &scheduler.NotificationContextOptions{
    Enabled:      true,
    MinPriority:  scheduler.PriorityHigh,        // Only show high and critical
    ShowDesktop:  true,                          // Enable desktop notifications
    ShowTerminal: true,                          // Enable terminal notifications
    Categories:   []string{"deployment", "infrastructure"}, // Filter by category
    MaxHistorySize: 50,                           // Keep last 50 notifications
}

nc := scheduler.NewNotificationContext(logger, opts)
```

## Fluent Usage Examples

### Example 1: Basic Notification Context

```go
// Create with sensible defaults
nc := scheduler.NewNotificationContext(logger, nil)

// All notifications will be displayed
notif := scheduler.CreateJobNotification(
    "SCH-001", "run_wrapper", "deployment", "completed",
    scheduler.PriorityMedium, 45*time.Second, nil, nil,
)
nc.Notify(notif)
```

### Example 2: High-Priority Only

```go
// Only show high and critical priority notifications
nc := scheduler.NewNotificationContext(logger, &scheduler.NotificationContextOptions{
    MinPriority: scheduler.PriorityHigh,
})

// Medium priority - won't be shown
notif1 := scheduler.CreateJobNotification(
    "SCH-002", "run_wrapper", "testing", "completed",
    scheduler.PriorityMedium, 10*time.Second, nil, nil,
)
nc.Notify(notif1) // Filtered out

// High priority - will be shown
notif2 := scheduler.CreateJobNotification(
    "SCH-003", "run_wrapper", "deployment", "failed",
    scheduler.PriorityHigh, 0, fmt.Errorf("deployment failed"), nil,
)
nc.Notify(notif2) // Displayed
```

### Example 3: Category Filtering

```go
// Only show notifications from specific categories
nc := scheduler.NewNotificationContext(logger, &scheduler.NotificationContextOptions{
    Categories: []string{"infrastructure", "deployment"},
})

// Infrastructure - shown
notif1 := scheduler.CreateJobNotification(
    "SCH-004", "run_wrapper", "infrastructure", "completed",
    scheduler.PriorityMedium, 0, nil, nil,
)
nc.Notify(notif1) // Displayed

// Testing - filtered out
notif2 := scheduler.CreateJobNotification(
    "SCH-005", "run_wrapper", "testing", "completed",
    scheduler.PriorityMedium, 0, nil, nil,
)
nc.Notify(notif2) // Filtered out
```

### Example 4: Job Suppression

```go
nc := scheduler.NewNotificationContext(logger, nil)

// Suppress notifications for a job during maintenance
nc.Suppress("SCH-006", 1*time.Hour)

// This notification will be suppressed
notif := scheduler.CreateJobNotification(
    "SCH-006", "run_wrapper", "maintenance", "completed",
    scheduler.PriorityHigh, 0, nil, nil,
)
nc.Notify(notif) // Suppressed - won't be displayed

// Other jobs still notify normally
notif2 := scheduler.CreateJobNotification(
    "SCH-007", "run_wrapper", "deployment", "completed",
    scheduler.PriorityHigh, 0, nil, nil,
)
nc.Notify(notif2) // Displayed normally
```

### Example 5: Acknowledgment Tracking

```go
nc := scheduler.NewNotificationContext(logger, nil)

// Send high priority notifications
notif1 := scheduler.CreateJobNotification(
    "SCH-008", "run_wrapper", "deployment", "failed",
    scheduler.PriorityHigh, 0, fmt.Errorf("error"), nil,
)
notif2 := scheduler.CreateJobNotification(
    "SCH-009", "run_wrapper", "infrastructure", "failed",
    scheduler.PriorityCritical, 0, fmt.Errorf("error"), nil,
)

nc.Notify(notif1)
nc.Notify(notif2)

// Get unacknowledged notifications
unacknowledged := nc.GetUnacknowledged()
fmt.Printf("Unacknowledged: %d\n", len(unacknowledged)) // 2

// Acknowledge one
nc.Acknowledge("SCH-008")

unacknowledged = nc.GetUnacknowledged()
fmt.Printf("After acknowledgment: %d\n", len(unacknowledged)) // 1
```

### Example 6: Notification History

```go
nc := scheduler.NewNotificationContext(logger, &scheduler.NotificationContextOptions{
    MaxHistorySize: 10,
})

// Send multiple notifications
for i := 0; i < 20; i++ {
    notif := scheduler.CreateJobNotification(
        fmt.Sprintf("SCH-%03d", i),
        "run_wrapper",
        "test",
        "completed",
        scheduler.PriorityMedium,
        time.Duration(i)*time.Second,
        nil,
        nil,
    )
    nc.Notify(notif)
}

// Get recent history (last 5)
history := nc.GetHistory(5)
fmt.Printf("Recent notifications: %d\n", len(history)) // 5
fmt.Printf("Most recent: %s\n", history[len(history)-1].JobID) // SCH-019
```

### Example 7: Dynamic Configuration

```go
nc := scheduler.NewNotificationContext(logger, &scheduler.NotificationContextOptions{
    MinPriority: scheduler.PriorityLow,
})

// Change minimum priority dynamically
nc.SetMinPriority(scheduler.PriorityHigh)

// Change enabled state
nc.SetEnabled(false) // Disable all notifications
nc.SetEnabled(true)  // Re-enable

// Change category filter
nc.SetCategories([]string{"deployment"})
```

### Example 8: Context Integration

```go
import "context"

nc := scheduler.NewNotificationContext(logger, nil)

// Add notification context to Go context
ctx := scheduler.WithNotificationContext(context.Background(), nc)

// Extract and use in a function
func processJob(ctx context.Context) {
    nc := scheduler.NotificationContextFromContext(ctx)
    if nc != nil {
        notif := scheduler.CreateJobNotification(
            "SCH-010", "run_wrapper", "test", "completed",
            scheduler.PriorityMedium, 0, nil, nil,
        )
        nc.Notify(notif)
    }
}

processJob(ctx)
```

### Example 9: Scheduler Integration

```go
// Create scheduler with notification context
storage := // ... storage provider
specLoader := // ... spec loader
lifecycleLoader := // ... lifecycle loader

scheduler := scheduler.NewScheduler(storage, specLoader, lifecycleLoader)

// Create custom notification context
opts := &scheduler.NotificationContextOptions{
    Enabled:     true,
    MinPriority: scheduler.PriorityMedium,
    Categories:  []string{"deployment", "infrastructure"},
}
nc := scheduler.NewNotificationContext(logger, opts)

// Set on scheduler
scheduler.SetNotificationContext(nc)

// All job handlers will use this context automatically
// Notifications respect the preferences
```

### Example 10: Custom Notification Handler

```go
nc := scheduler.NewNotificationContext(logger, &scheduler.NotificationContextOptions{
    OnNotification: func(notif *scheduler.JobNotification) {
        // Custom handling - e.g., send to external system
        fmt.Printf("Custom handler: %s - %s\n", notif.JobID, notif.Title)
        
        // Could send to Slack, email, etc.
        // sendToSlack(notif)
    },
})

// All notifications will also trigger custom handler
notif := scheduler.CreateJobNotification(
    "SCH-011", "run_wrapper", "deployment", "completed",
    scheduler.PriorityMedium, 0, nil, nil,
)
nc.Notify(notif)
// Custom handler called + normal display
```

## Notification Display

### Terminal Notifications

Terminal notifications are automatically displayed with:
- **Color coding**: Green (success), Red (failure), Cyan (status), Yellow (general)
- **Icons**: ✅ (success), ❌ (failure), ℹ️ (status), 🔔 (general)
- **Priority indicators**: 🚨 CRITICAL, ⚠️ HIGH, 📢 MEDIUM, ℹ️ LOW
- **Bordered formatting** for visibility

Example output:
```
═══════════════════════════════════════════════════════════
✅ ⚠️  HIGH Job Failed
═══════════════════════════════════════════════════════════
  Job ID:    SCH-005
  Job Type:  run_wrapper
  Category:  infrastructure
  Duration:  45s
  Message:   Job SCH-005 failed: command timed out
  Error:     command timed out after 30 seconds
  Time:      2025-01-02 14:30:15
═══════════════════════════════════════════════════════════
```

### Desktop Notifications

Desktop notifications are shown for high-priority events (high/critical):
- **macOS**: Uses `osascript` for native notifications
- **Linux**: Uses `notify-send`
- **Windows**: Uses PowerShell toast notifications

## Priority Levels

- **PriorityCritical**: 🚨 Critical issues requiring immediate attention
- **PriorityHigh**: ⚠️ High-priority issues (failures, timeouts)
- **PriorityMedium**: 📢 Medium-priority (successful completions, normal status)
- **PriorityLow**: ℹ️ Low-priority (status updates, informational)

## Event Types

- **"completed"**: Job completed successfully
- **"failed"**: Job failed (priority automatically upgraded to at least Medium)
- **"status_update"**: Incremental status update

## API Reference

### NotificationContext

```go
type NotificationContext struct {
    // Preferences, history, channels (internal)
}

// Methods:
func (nc *NotificationContext) Notify(notif *JobNotification)
func (nc *NotificationContext) ShouldNotify(notif *JobNotification) bool
func (nc *NotificationContext) Acknowledge(jobID string)
func (nc *NotificationContext) Suppress(jobID string, duration time.Duration)
func (nc *NotificationContext) GetUnacknowledged() []*JobNotification
func (nc *NotificationContext) GetHistory(limit int) []*JobNotification
func (nc *NotificationContext) SetMinPriority(priority NotificationPriority)
func (nc *NotificationContext) SetEnabled(enabled bool)
func (nc *NotificationContext) SetCategories(categories []string)
func (nc *NotificationContext) TerminalChannel() <-chan *JobNotification
func (nc *NotificationContext) DesktopChannel() <-chan *JobNotification
func (nc *NotificationContext) EventChannel() <-chan *JobNotification
```

### CreateJobNotification

```go
func CreateJobNotification(
    jobID, jobType, category, event string,
    priority NotificationPriority,
    duration time.Duration,
    err error,
    metadata map[string]interface{},
) *JobNotification
```

## Integration Points

1. **Run Wrapper Handler**: Automatically displays notifications on completion/failure
2. **Callback Listener Handler**: Displays notifications for external callbacks
3. **Scheduler**: Default notification context created automatically
4. **Custom Handlers**: Can be added via `OnNotification` callback

## Best Practices

1. **Set Appropriate Priorities**: Use PriorityHigh for failures, PriorityMedium for successes
2. **Filter by Category**: Use category filtering to reduce noise
3. **Suppress During Maintenance**: Suppress notifications for jobs under maintenance
4. **Track Unacknowledged**: Regularly check and acknowledge high-priority notifications
5. **Use History for Debugging**: Review notification history to understand job patterns

## Related Documentation

- [Scheduler Architecture](../scheduler/scheduler-architecture-v1.0.md)
- [Run Wrapper Jobs](../scheduler/run-wrapper-jobs-v1.0.md)
- [Callback Listener](../scheduler/callback-listener-v1.0.md)
- [POL-WORKFLOW-004](../../policies/POL-WORKFLOW-004.yaml): Terminal Command Run-Wrapper Requirement

---

*Last Updated: 2025-01-02*

