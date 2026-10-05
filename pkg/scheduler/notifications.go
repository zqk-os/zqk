package scheduler

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/execwrap"

	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/when"
	"github.com/zqk-os/zqk/pkg/zqktime"
)

var stdoutIsTerminal = isTerminal(os.Stdout)

// NotificationPriority represents the priority/urgency of a notification
type NotificationPriority string

const (
	PriorityLow      NotificationPriority = "low"
	PriorityMedium   NotificationPriority = "medium"
	PriorityHigh     NotificationPriority = "high"
	PriorityCritical NotificationPriority = "critical"
)

// Level returns numeric priority ranking (1=low, 2=medium, 3=high, 4=critical).
func (p NotificationPriority) Level() int {
	switch p {
	case PriorityCritical:
		return 4
	case PriorityHigh:
		return 3
	case PriorityMedium:
		return 2
	case PriorityLow:
		return 1
	default:
		return 1
	}
}

// IsAtLeast returns true if p has equal or higher priority than min.
func (p NotificationPriority) IsAtLeast(min NotificationPriority) bool {
	return p.Level() >= min.Level()
}

const (
	notificationEventCompleted    = "completed"
	notificationEventFailed       = "failed"
	notificationEventStatusUpdate = "status_update"
)

// JobNotification represents a user-facing notification about a job
type JobNotification struct {
	JobID     string
	JobType   string
	Category  string
	Event     string // "completed", "failed", "status_update"
	Priority  NotificationPriority
	Title     string
	Message   string
	Duration  time.Duration
	Error     error
	Timestamp time.Time
	Metadata  map[string]any
}

// NotificationDisplay displays a notification to the user in an attention-grabbing way
type NotificationDisplay struct {
	logger logging.Logger
}

// NewNotificationDisplay creates a new notification display
// NewNotificationDisplay creates a new notification display
func NewNotificationDisplay(logger logging.Logger) NotificationDisplayInterface {
	return &NotificationDisplay{
		logger: logger,
	}
}

// Display shows a notification to the user with attention-grabbing formatting
func (nd *NotificationDisplay) Display(notif *JobNotification) {
	// Always log to structured logger
	nd.logNotification(notif)

	// Display to terminal with attention-grabbing formatting
	nd.displayTerminalNotification(notif)

	// Try desktop notification (if available and appropriate)
	if notif.Priority.IsAtLeast(PriorityHigh) {
		nd.displayDesktopNotification(notif)
	}
}

// logNotification logs the notification to the structured logger
// DisplayTerminalNotification displays a notification in the terminal
func (nd *NotificationDisplay) DisplayTerminalNotification(notif *JobNotification) {
	nd.displayTerminalNotification(notif)
}

// DisplayDesktopNotification displays a notification on the desktop
func (nd *NotificationDisplay) DisplayDesktopNotification(notif *JobNotification) {
	nd.displayDesktopNotification(notif)
}

func (nd *NotificationDisplay) logNotification(notif *JobNotification) {
	fields := []logging.Field{
		logging.String(KeyJobID, notif.JobID),
		logging.String(KeyJobType, notif.JobType),
		logging.String(KeyCategory, notif.Category),
		logging.String("event", notif.Event),
		logging.String("priority", string(notif.Priority)),
	}
	// Log if this is a test failure notification (always include in log)
	if isTestFailure, ok := notif.Metadata["is_test_failure"].(bool); ok && isTestFailure {
		fields = append(fields, logging.Bool("is_test_failure", true))
		fields = append(fields, logging.String("notification_message", notif.Message))
		fields = append(fields, logging.String("priority_before_downgrade", "high"))
		fields = append(fields, logging.String("priority_after_downgrade", string(notif.Priority)))
	}

	if notif.Duration > 0 {
		fields = append(fields, logging.String(KeyDuration, notif.Duration.String()))
	}

	when.When(func() bool { return notif.Error != nil }).Then(func() {
		logging.Fluent(nd.logger).Error(notif.Title, notif.Error).WithFields(fields...).Log()
	}).OrElse(func() {
		logging.Fluent(nd.logger).Info(notif.Title).WithFields(fields...).Log()
	}).Run()
}

// displayTerminalNotification displays a prominent terminal notification
func (nd *NotificationDisplay) displayTerminalNotification(notif *JobNotification) {
	// Check if we're in a TTY (interactive terminal)
	if !stdoutIsTerminal {
		// Not a TTY - just log, don't display fancy formatting
		return
	}

	// Build attention-grabbing message
	var icon string
	var colorCode string
	var resetCode = "\033[0m"

	switch notif.Event {
	case notificationEventCompleted:
		icon = "✅"
		colorCode = "\033[32m" // Green
	case notificationEventFailed:
		icon = "❌"
		colorCode = "\033[31m" // Red
	case notificationEventStatusUpdate:
		icon = "ℹ️"
		colorCode = "\033[36m" // Cyan
	default:
		icon = "🔔"
		colorCode = "\033[33m" // Yellow
	}

	// Add priority indicator
	var priorityIndicator string
	switch notif.Priority {
	case PriorityCritical:
		priorityIndicator = "🚨 CRITICAL"
		colorCode = "\033[31;1m" // Bright red
	case PriorityHigh:
		priorityIndicator = "⚠️  HIGH"
		colorCode = "\033[33;1m" // Bright yellow
	case PriorityMedium:
		priorityIndicator = "📢 MEDIUM"
	case PriorityLow:
		priorityIndicator = "ℹ️  LOW"
	}

	// Build message
	var lines []string
	lines = append(lines,
		"",
		fmt.Sprintf("%s%s═══════════════════════════════════════════════════════════%s", colorCode, strings.Repeat("═", 10), resetCode),
		fmt.Sprintf("%s%s %s %s%s", colorCode, icon, priorityIndicator, notif.Title, resetCode),
		fmt.Sprintf("%s%s═══════════════════════════════════════════════════════════%s", colorCode, strings.Repeat("═", 10), resetCode),
		fmt.Sprintf("  Job ID:    %s", notif.JobID),
		fmt.Sprintf("  Job Type:  %s", notif.JobType))
	if notif.Category != emptyValue {
		lines = append(lines, fmt.Sprintf("  Category:  %s", notif.Category))
	}
	if notif.Duration > 0 {
		lines = append(lines, fmt.Sprintf("  Duration:  %s", formatDuration(notif.Duration)))
	}
	if notif.Message != emptyValue {
		lines = append(lines, fmt.Sprintf("  Message:   %s", notif.Message))
	}
	if notif.Error != nil {
		lines = append(lines, fmt.Sprintf("  Error:     %s", notif.Error.Error()))
	}
	lines = append(lines,
		fmt.Sprintf("  Time:      %s", zqktime.FormatLayoutUTC(notif.Timestamp, zqktime.LayoutDateTimeSpace)),
		fmt.Sprintf("%s%s═══════════════════════════════════════════════════════════%s", colorCode, strings.Repeat("═", 10), resetCode),
		"")

	// Write formatted notification through logger to ensure proper routing
	// In MCP mode, this will go to stderr via the logging framework
	// In normal mode, it will go to stdout (if logger is configured for stdout)
	formattedMsg := strings.Join(lines, "\n")
	SLog(nd.logger).Info(formattedMsg).NotifyType("terminal_display").Log()
}

// displayDesktopNotification displays a desktop notification (if available)
func (nd *NotificationDisplay) displayDesktopNotification(notif *JobNotification) {
	// Only show desktop notifications for high-priority events
	if !notif.Priority.IsAtLeast(PriorityHigh) {
		return
	}

	var cmd *exec.Cmd
	title := notif.Title
	body := notif.Message
	if body == emptyValue {
		body = fmt.Sprintf("Job %s (%s)", notif.JobID, notif.JobType)
	}

	switch runtime.GOOS {
	case "darwin": // macOS
		// Use osascript to show notification with strict escaping (L:F-SEC-02)
		cleanBody := strings.ReplaceAll(strings.ReplaceAll(body, `\`, `\\`), `"`, `\"`)
		cleanTitle := strings.ReplaceAll(strings.ReplaceAll(title, `\`, `\\`), `"`, `\"`)
		script := fmt.Sprintf(`display notification "%s" with title "%s"`, cleanBody, cleanTitle)
		cmd = execwrap.Command("osascript", "-e", script)
	case "linux":
		// Try notify-send (most common on Linux)
		urgency := "normal"
		if notif.Priority == PriorityCritical {
			urgency = "critical"
		}
		cmd = execwrap.Command("notify-send", "-u", urgency, title, body)
	case "windows":
		// Use PowerShell for Windows 10+ notifications with XML escaping (L:F-SEC-02)
		xmlEscape := func(s string) string {
			s = strings.ReplaceAll(s, "&", "&amp;")
			s = strings.ReplaceAll(s, "<", "&lt;")
			s = strings.ReplaceAll(s, ">", "&gt;")
			s = strings.ReplaceAll(s, `"`, "&quot;")
			s = strings.ReplaceAll(s, "'", "&apos;")
			return s
		}
		cleanTitle := xmlEscape(title)
		cleanBody := xmlEscape(body)
		script := fmt.Sprintf(`[Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime] > $null; [Windows.Data.Xml.Dom.XmlDocument, Windows.Data.Xml.Dom.XmlDocument, ContentType = WindowsRuntime] > $null; $xml = [Windows.Data.Xml.Dom.XmlDocument]::new(); $xml.LoadXml('<toast><visual><binding template="ToastText02"><text id="1">%s</text><text id="2">%s</text></binding></visual></toast>'); $toast = [Windows.UI.Notifications.ToastNotification]::new($xml); [Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier("ZQK").Show($toast);`, cleanTitle, cleanBody)
		cmd = execwrap.Command("powershell", "-Command", script)
	default:
		// Unsupported platform
		return
	}

	// Execute notification command (best effort - don't fail if it doesn't work)
	if err := cmd.Run(); err != nil {
		// Silently ignore - desktop notifications are optional
		if nd != nil && nd.logger != nil {
			SchedulerNotificationsLog(nd.logger).Debug(LogEventSchedulerNotificationsDesktopDisplayFailed).
				WithFields(append([]logging.Field{logging.String("os", runtime.GOOS)}, logErrField(err)...)...).
				Log()
		}
	}
}

// formatDuration formats a duration with appropriate precision
// - Durations < 1 second: show milliseconds (e.g., "269ms", "15ms")
// - Durations >= 1 second: show rounded seconds (e.g., "5s", "1m30s")
func formatDuration(d time.Duration) string {
	if d < time.Second {
		// Show milliseconds for sub-second durations
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	// Round to seconds for longer durations
	return d.Round(time.Second).String()
}

// isTerminal checks if the file descriptor is a terminal
func isTerminal(f *fileutil.File) bool {
	fileInfo, err := f.Stat()
	if err != nil {
		return false
	}
	return (fileInfo.Mode() & fileutil.ModeCharDevice) != 0
}

// CreateJobNotification creates a notification from job execution details
func CreateJobNotification(jobID, jobType, category, event string, priority NotificationPriority, duration time.Duration, err error, metadata map[string]any) *JobNotification {
	// Extract job title and description from metadata if available
	jobTitle, _ := metadata[objects.FieldKeyJobTitle].(string)
	jobDescription, _ := metadata["job_description"].(string)

	notif := &JobNotification{
		JobID:     jobID,
		JobType:   jobType,
		Category:  category,
		Event:     event,
		Priority:  priority,
		Duration:  duration,
		Error:     err,
		Timestamp: time.Now(),
		Metadata:  metadata,
	}

	// Build title and message based on event type
	switch event {
	case notificationEventCompleted:
		when.When(func() bool { return jobTitle != emptyValue }).Then(func() {
			notif.Title = fmt.Sprintf("%s - Completed", jobTitle)
		}).OrElse(func() {
			notif.Title = "Job Completed Successfully"
		}).Run()
		if jobTitle != emptyValue {
			when.When(func() bool { return duration > 0 }).Then(func() {
				notif.Message = fmt.Sprintf("%s (%s) completed in %s", jobTitle, jobID, formatDuration(duration))
			}).OrElse(func() {
				notif.Message = fmt.Sprintf("%s (%s) completed", jobTitle, jobID)
			}).Run()
			if jobDescription != emptyValue {
				// Truncate description to first line or 100 chars for notification
				desc := jobDescription
				if idx := strings.Index(desc, "\n"); idx > 0 {
					desc = desc[:idx]
				}
				if len(desc) > 100 {
					desc = desc[:100] + "..."
				}
				notif.Message += fmt.Sprintf("\n  %s", desc)
			}
		} else {
			when.When(func() bool { return duration > 0 }).Then(func() {
				notif.Message = fmt.Sprintf("Job %s completed in %s", jobID, formatDuration(duration))
			}).OrElse(func() {
				notif.Message = fmt.Sprintf("Job %s completed", jobID)
			}).Run()
		}
	case notificationEventFailed:
		// Check if this is a test failure (tests failed) vs job execution failure (job couldn't run)
		isTestFailure, _ := metadata["is_test_failure"].(bool)
		var testFailures []string
		var testSummary map[string]any
		if tf, ok := metadata[KeyTestFailures].([]string); ok {
			testFailures = tf
		} else if tfAny, ok := metadata[KeyTestFailures].([]any); ok {
			// Handle case where test failures come as []any from JSON unmarshaling
			testFailures = make([]string, 0, len(tfAny))
			for _, v := range tfAny {
				if s, ok := v.(string); ok {
					testFailures = append(testFailures, s)
				}
			}
		}
		if ts, ok := metadata[KeyTestSummary].(map[string]any); ok {
			testSummary = ts
		}

		if isTestFailure {
			// Test failures - job executed successfully but tests failed
			// Clear the error since this is not a job execution failure
			notif.Error = nil

			when.When(func() bool { return jobTitle != emptyValue }).Then(func() {
				notif.Title = fmt.Sprintf("%s - Tests Failed", jobTitle)
			}).OrElse(func() {
				notif.Title = "Test Bundle - Tests Failed"
			}).Run()

			// Build message with test failure details - make it clear the job ran successfully
			var message strings.Builder
			if len(testFailures) > 0 {
				// We have parsed test failures - show details
				when.When(func() bool { return jobTitle != emptyValue }).Then(func() {
					fmt.Fprintf(&message, "%s (%s) ran successfully, but %d test(s) failed", jobTitle, jobID, len(testFailures))
				}).OrElse(func() {
					fmt.Fprintf(&message, "Test bundle %s ran successfully, but %d test(s) failed", jobID, len(testFailures))
				}).Run()

				// Add test summary if available
				if testSummary != nil {
					if total, ok := testSummary["total_tests"].(int); ok && total > 0 {
						passed, _ := testSummary["passed_tests"].(int)
						failed, _ := testSummary["failed_tests"].(int)
						skipped, _ := testSummary["skipped_tests"].(int)
						fmt.Fprintf(&message, "\n  Test results: %d passed, %d failed, %d skipped (of %d total)", passed, failed, skipped, total)
					}
				}

				// List failed tests (limit to first 5 to avoid overwhelming notification)
				message.WriteString("\n  Failed tests:")
				maxFailures := 5
				if len(testFailures) < maxFailures {
					maxFailures = len(testFailures)
				}
				for i := 0; i < maxFailures; i++ {
					fmt.Fprintf(&message, "\n    - %s", testFailures[i])
				}
				if len(testFailures) > maxFailures {
					fmt.Fprintf(&message, "\n    ... and %d more", len(testFailures)-maxFailures)
				}
			} else {
				// Parsing failed or no failures parsed - use generic message
				when.When(func() bool { return jobTitle != emptyValue }).Then(func() {
					fmt.Fprintf(&message, "%s (%s) ran successfully, but some test(s) failed", jobTitle, jobID)
				}).OrElse(func() {
					fmt.Fprintf(&message, "Test bundle %s ran successfully, but some test(s) failed", jobID)
				}).Run()
				message.WriteString("\n  (Unable to parse test output for detailed failure information)")
			}

			notif.Message = message.String()
			// Test failures are lower priority than job execution failures
			if notif.Priority > PriorityMedium {
				notif.Priority = PriorityMedium
			}
		} else {
			// Job execution failure - job couldn't run or timed out
			when.When(func() bool { return jobTitle != emptyValue }).Then(func() {
				notif.Title = fmt.Sprintf("%s - Failed", jobTitle)
			}).OrElse(func() {
				notif.Title = "Job Failed"
			}).Run()
			if jobTitle != emptyValue {
				when.When(func() bool { return err != nil }).Then(func() {
					notif.Message = fmt.Sprintf("%s (%s) failed: %v", jobTitle, jobID, err)
				}).OrElse(func() {
					notif.Message = fmt.Sprintf("%s (%s) failed", jobTitle, jobID)
				}).Run()
				if jobDescription != emptyValue {
					// Truncate description to first line or 100 chars for notification
					desc := jobDescription
					if idx := strings.Index(desc, "\n"); idx > 0 {
						desc = desc[:idx]
					}
					if len(desc) > 100 {
						desc = desc[:100] + "..."
					}
					notif.Message += fmt.Sprintf("\n  %s", desc)
				}
			} else {
				when.When(func() bool { return err != nil }).Then(func() {
					notif.Message = fmt.Sprintf("Job %s failed: %v", jobID, err)
				}).OrElse(func() {
					notif.Message = fmt.Sprintf("Job %s failed", jobID)
				}).Run()
			}
		}
		// Failures are always at least medium priority (use priority order for comparison)
		priorityOrder := map[NotificationPriority]int{
			PriorityCritical: 4,
			PriorityHigh:     3,
			PriorityMedium:   2,
			PriorityLow:      1,
		}
		priorityValue := priorityOrder[priority]
		mediumValue := priorityOrder[PriorityMedium]
		if priorityValue == 0 {
			priorityValue = 1 // Unknown priority defaults to low
		}
		if priorityValue < mediumValue {
			notif.Priority = PriorityMedium
		}
	case notificationEventStatusUpdate:
		notif.Title = "Job Status Update"
		status, statusOk := metadata[objects.FieldKeyStatus].(string)
		when.When(func() bool { return statusOk }).Then(func() {
			notif.Message = fmt.Sprintf("Job %s status: %s", jobID, status)
		}).OrElse(func() {
			notif.Message = fmt.Sprintf("Job %s status updated", jobID)
		}).Run()
	default:
		notif.Title = "Job Notification"
		notif.Message = fmt.Sprintf("Job %s: %s", jobID, event)
	}

	return notif
}
