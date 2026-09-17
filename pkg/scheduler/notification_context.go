package scheduler

import (
	"context"
	"sync"
	"time"

	"github.com/lanceman/zqk/pkg/concurrency"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/logging"
)

// Context key type for avoiding collisions
type notificationContextKey struct{}

// NotificationContext manages notification preferences, history, and delivery
type NotificationContext struct {
	mu sync.RWMutex

	// Preferences
	enabled        bool
	minPriority    NotificationPriority
	showDesktop    bool
	showTerminal   bool
	categories     map[string]bool      // Category filter (empty = all allowed)
	suppressedJobs map[string]time.Time // Job IDs to suppress (with expiry)

	// History and tracking
	history        []*JobNotification
	maxHistorySize int
	unacknowledged map[string]*JobNotification // Job ID -> notification

	// Delivery channels
	terminalChannel chan *JobNotification
	desktopChannel  chan *JobNotification
	eventChannel    chan *JobNotification // For event-based integrations

	// Context for graceful shutdown
	ctx    context.Context
	cancel context.CancelFunc

	// Callbacks
	onNotification func(*JobNotification) // Custom handler

	logger logging.Logger
}

// NotificationContextOptions configures a notification context
type NotificationContextOptions struct {
	Enabled        bool
	MinPriority    NotificationPriority
	ShowDesktop    bool
	ShowTerminal   bool
	Categories     []string // Empty = all categories allowed
	MaxHistorySize int
	OnNotification func(*JobNotification)
}

// NewNotificationContext creates a new notification context with options
func NewNotificationContext(logger logging.Logger, opts *NotificationContextOptions) NotificationContextInterface {
	if opts == nil {
		opts = &NotificationContextOptions{
			Enabled:        true,
			MinPriority:    PriorityLow,
			ShowDesktop:    true,
			ShowTerminal:   true,
			MaxHistorySize: 100,
		}
	}

	// Build category filter
	categories := make(map[string]bool)
	for _, cat := range opts.Categories {
		categories[cat] = true
	}

	nc := &NotificationContext{
		enabled:         opts.Enabled,
		minPriority:     opts.MinPriority,
		showDesktop:     opts.ShowDesktop,
		showTerminal:    opts.ShowTerminal,
		categories:      categories,
		suppressedJobs:  make(map[string]time.Time),
		history:         make([]*JobNotification, 0),
		maxHistorySize:  opts.MaxHistorySize,
		unacknowledged:  make(map[string]*JobNotification),
		terminalChannel: make(chan *JobNotification, 10),
		desktopChannel:  make(chan *JobNotification, 10),
		eventChannel:    make(chan *JobNotification, 10),
		onNotification:  opts.OnNotification,
		logger:          logger,
	}
	nc.ctx, nc.cancel = context.WithCancel(context.Background()) // Background: request-or-shutdown derived

	// Start delivery goroutines
	goroutinelabels.NewGoroutine("notification_context_delivery_loop", "delivering job notifications").
		StartWithContext(nc.ctx, func(ctx context.Context) error {
			nc.deliveryLoop()
			return nil
		})

	return nc
}

// ShouldNotify determines if a notification should be displayed
func (nc *NotificationContext) ShouldNotify(notif *JobNotification) bool {
	var enabled bool
	var minPriority NotificationPriority
	var categories map[string]bool
	var suppressedJobs map[string]time.Time
	_ = concurrency.RunInRLockWithLogger(
		&nc.mu, LockNameNotificationContextShouldNotify, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			enabled = nc.enabled
			minPriority = nc.minPriority
			categories = nc.categories
			suppressedJobs = nc.suppressedJobs
			return nil
		},
	)

	// Check if notifications are enabled
	if !enabled {
		return false
	}

	// Check priority threshold (higher priority values come first)
	// Priority order: critical > high > medium > low
	priorityOrder := map[NotificationPriority]int{
		PriorityCritical: 4,
		PriorityHigh:     3,
		PriorityMedium:   2,
		PriorityLow:      1,
	}
	notifPriority := priorityOrder[notif.Priority]
	minPriorityVal := priorityOrder[minPriority]
	if notifPriority == 0 {
		// Unknown priority - default to low
		notifPriority = 1
	}
	if minPriorityVal == 0 {
		// No minimum set - allow all
		minPriorityVal = 1
	}
	if notifPriority < minPriorityVal {
		return false
	}

	// Check category filter (if any categories specified, must match)
	if len(categories) > 0 {
		if !categories[notif.Category] {
			return false
		}
	}

	// Check if job is suppressed
	if expiry, suppressed := suppressedJobs[notif.JobID]; suppressed {
		if time.Now().Before(expiry) {
			return false
		}
		// Expired suppression - remove it (upgrade to write lock)
		_ = concurrency.RunInLockWithLogger(
			&nc.mu, LockNameNotificationContextRemoveSuppression, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
			func() error {
				delete(nc.suppressedJobs, notif.JobID)
				return nil
			},
		)
	}

	return true
}

// Notify sends a notification through the context (respects preferences)
func (nc *NotificationContext) Notify(notif *JobNotification) {
	if !nc.ShouldNotify(notif) {
		return
	}

	_ = concurrency.RunInLockWithLogger(
		&nc.mu, LockNameNotificationContextNotify, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			// Add to history
			nc.history = append(nc.history, notif)
			if len(nc.history) > nc.maxHistorySize {
				// Remove oldest
				nc.history = nc.history[1:]
			}

			// Add to unacknowledged if high priority (use priority order for comparison)
			priorityOrder := map[NotificationPriority]int{
				PriorityCritical: 4,
				PriorityHigh:     3,
				PriorityMedium:   2,
				PriorityLow:      1,
			}
			notifPriority := priorityOrder[notif.Priority]
			highPriority := priorityOrder[PriorityHigh]
			if notifPriority == 0 {
				// Unknown priority - default to low
				notifPriority = 1
			}
			if notifPriority >= highPriority {
				nc.unacknowledged[notif.JobID] = notif
			}
			return nil
		},
	)

	// Calculate priority for channel routing (outside lock)
	priorityOrder := map[NotificationPriority]int{
		PriorityCritical: 4,
		PriorityHigh:     3,
		PriorityMedium:   2,
		PriorityLow:      1,
	}
	notifPriority := priorityOrder[notif.Priority]
	highPriority := priorityOrder[PriorityHigh]
	if notifPriority == 0 {
		// Unknown priority - default to low
		notifPriority = 1
	}

	// Call custom handler if set
	if nc.onNotification != nil {
		nc.onNotification(notif)
	}

	// Send to delivery channels
	var showTerminal, showDesktop bool
	_ = concurrency.RunInRLockWithLogger(
		&nc.mu, LockNameNotificationContextGetChannels, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			showTerminal = nc.showTerminal
			showDesktop = nc.showDesktop
			return nil
		},
	)

	if showTerminal {
		select {
		case nc.terminalChannel <- notif:
		default:
			// Channel full - drop notification (non-blocking)
		}
	}

	// Check if desktop notification should be shown (high+ priority)
	if showDesktop && notifPriority >= highPriority {
		select {
		case nc.desktopChannel <- notif:
		default:
			// Channel full - drop notification (non-blocking)
		}
	}

	// Always send to event channel (for integrations)
	select {
	case nc.eventChannel <- notif:
	default:
		// Channel full - drop notification (non-blocking)
	}
}

// Acknowledge marks a notification as acknowledged
func (nc *NotificationContext) Acknowledge(jobID string) {
	_ = concurrency.RunInLockWithLogger(
		&nc.mu, LockNameNotificationContextAcknowledge, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			delete(nc.unacknowledged, jobID)
			return nil
		},
	)
}

// Suppress suppresses notifications for a job ID for a duration
func (nc *NotificationContext) Suppress(jobID string, duration time.Duration) {
	_ = concurrency.RunInLockWithLogger(
		&nc.mu, LockNameNotificationContextSuppress, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			nc.suppressedJobs[jobID] = time.Now().Add(duration)
			return nil
		},
	)
}

// GetUnacknowledged returns all unacknowledged notifications
func (nc *NotificationContext) GetUnacknowledged() []*JobNotification {
	var result []*JobNotification
	_ = concurrency.RunInRLockWithLogger(
		&nc.mu, LockNameNotificationContextGetUnacknowledged, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			result = make([]*JobNotification, 0, len(nc.unacknowledged))
			for _, notif := range nc.unacknowledged {
				result = append(result, notif)
			}
			return nil
		},
	)
	return result
}

// GetHistory returns notification history (most recent first)
func (nc *NotificationContext) GetHistory(limit int) []*JobNotification {
	var result []*JobNotification
	_ = concurrency.RunInRLockWithLogger(
		&nc.mu, LockNameNotificationContextGetHistory, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			historyLimit := limit
			if historyLimit <= 0 || historyLimit > len(nc.history) {
				historyLimit = len(nc.history)
			}

			// Return most recent first
			result = make([]*JobNotification, historyLimit)
			start := len(nc.history) - historyLimit
			for i := 0; i < historyLimit; i++ {
				result[i] = nc.history[start+i]
			}
			return nil
		},
	)
	return result
}

// SetMinPriority updates the minimum priority threshold
func (nc *NotificationContext) SetMinPriority(priority NotificationPriority) {
	_ = concurrency.RunInLockWithLogger(
		&nc.mu, LockNameNotificationContextSetMinPriority, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			nc.minPriority = priority
			return nil
		},
	)
}

// SetEnabled enables or disables notifications
func (nc *NotificationContext) SetEnabled(enabled bool) {
	_ = concurrency.RunInLockWithLogger(
		&nc.mu, LockNameNotificationContextSetEnabled, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			nc.enabled = enabled
			return nil
		},
	)
}

// SetCategories updates the category filter
func (nc *NotificationContext) SetCategories(categories []string) {
	_ = concurrency.RunInLockWithLogger(
		&nc.mu, LockNameNotificationContextSetCategories, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			nc.categories = make(map[string]bool)
			for _, cat := range categories {
				nc.categories[cat] = true
			}
			return nil
		},
	)
}

// TerminalChannel returns the channel for terminal notifications
func (nc *NotificationContext) TerminalChannel() <-chan *JobNotification {
	return nc.terminalChannel
}

// DesktopChannel returns the channel for desktop notifications
func (nc *NotificationContext) DesktopChannel() <-chan *JobNotification {
	return nc.desktopChannel
}

// EventChannel returns the channel for event-based notifications
func (nc *NotificationContext) EventChannel() <-chan *JobNotification {
	return nc.eventChannel
}

// deliveryLoop handles notification delivery
func (nc *NotificationContext) deliveryLoop() {
	display := NewNotificationDisplay(nc.logger)
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-nc.ctx.Done():
			// Context cancelled - exit gracefully
			return
		case notif := <-nc.terminalChannel:
			display.DisplayTerminalNotification(notif)
		case notif := <-nc.desktopChannel:
			display.DisplayDesktopNotification(notif)
		case <-ticker.C:
			// Periodic cleanup of expired suppressions
			nc.cleanupSuppressions()
		}
	}
}

// Stop stops the notification context and its delivery goroutine
func (nc *NotificationContext) Stop() {
	if nc.cancel != nil {
		nc.cancel()
	}
}

// cleanupSuppressions removes expired suppressions
func (nc *NotificationContext) cleanupSuppressions() {
	_ = concurrency.RunInLockWithLogger(
		&nc.mu, LockNameNotificationContextCleanupSuppressions, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			now := time.Now()
			for jobID, expiry := range nc.suppressedJobs {
				if now.After(expiry) {
					delete(nc.suppressedJobs, jobID)
				}
			}
			return nil
		},
	)
}

// FromContext extracts NotificationContext from a Go context
func NotificationContextFromContext(ctx context.Context) NotificationContextInterface {
	if nc, ok := ctx.Value(notificationContextKey{}).(NotificationContextInterface); ok {
		return nc
	}
	return nil
}

// WithNotificationContext adds NotificationContext to a Go context
func WithNotificationContext(ctx context.Context, nc NotificationContextInterface) context.Context {
	return context.WithValue(ctx, notificationContextKey{}, nc)
}
