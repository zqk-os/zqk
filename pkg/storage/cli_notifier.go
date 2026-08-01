package storage

import (
	"context"
	"sync"
	"time"

	"github.com/lanceman/zqk/pkg/concurrency"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/storage/locknames"
	// ProgressEventEmitter is an interface for emitting progress events through coordinator
	// This allows coordinator integration without creating import cycles
)

type ProgressEventEmitter interface {
	EmitProgress(ctx context.Context, operationID, operationType string, progress, total int, message string, fields map[string]any, emitAudit bool) error
	EmitStatusChange(ctx context.Context, operationID, operationType, oldStatus, newStatus, message string, fields map[string]any) error
	EmitError(ctx context.Context, operationID, operationType string, err error, message string, fields map[string]any) error
	EmitCompletion(ctx context.Context, operationID, operationType string, duration time.Duration, message string, fields map[string]any) error
}

// CLINotifier implements OperationNotifier for CLI output
// Requires a ProgressEventEmitter for unified observability through coordinator
type CLINotifier struct {
	verbose        bool
	quiet          bool
	mu             sync.RWMutex
	logger         *logging.EventLogger
	progressLogger logging.ProgressLogger
	progress       map[string]*ProgressTracker
	eventEmitter   ProgressEventEmitter // Required event emitter for coordinator integration
	profile        string               // CLI profile for coordinator events
}

// ProgressTracker tracks progress for an operation
type ProgressTracker struct {
	OperationID string
	ObjectID    string
	Type        OperationType
	Progress    int
	Message     string
	LastUpdate  time.Time
}

// NewCLINotifier creates a new CLI notifier
// eventEmitter is required - all progress events are emitted through it via coordinator
func NewCLINotifier(verbose, quiet bool, eventEmitter ProgressEventEmitter) *CLINotifier {
	if eventEmitter == nil {
		panic(ConstMiscClinotifierRequiresAProgresseventemitter)
	}
	// Use system context for CLI notifier initialization
	ctx := pkgctx.NewSystemContext()
	eventLogger := logging.NewEventLogger(ctx)

	// Get the underlying logger to create a progress logger
	// Determine formatter based on context/profile
	var progressFormatter logging.ProgressFormatter
	var profile string
	loggingCtx := pkgctx.GetLoggingContext(ctx)
	if loggingCtx != nil {
		profile = string(loggingCtx.Profile)
		switch loggingCtx.Profile {
		case pkgctx.ProfileMCP, pkgctx.ProfileAIAgent:
			// Use JSON formatter for MCP/AI agent profiles (structured output)
			progressFormatter = logging.NewJSONProgressFormatter()
		case pkgctx.ProfileDebug:
			// Use compact formatter for debug profile
			progressFormatter = logging.NewCompactProgressFormatter()
		default:
			// Use text formatter for human profile (default)
			progressFormatter = logging.NewTextProgressFormatter()
		}
	} else {
		// Default to text formatter if no context
		progressFormatter = logging.NewTextProgressFormatter()
		profile = string(pkgctx.ProfileHuman)
	}

	// Get logger from context to create progress logger
	// Use ctx from NewCLINotifier for formatter context propagation
	baseLogger := logging.GetLoggerFromLoggingContext(ctx, loggingCtx)
	if baseLogger == nil {
		// Fallback to default logger
		baseLogger = logging.GetLoggerFromLoggingContext(ctx,
			pkgctx.NewLoggingContext(pkgctx.ProfileHuman))
	}

	progressLogger := logging.NewProgressLogger(baseLogger, progressFormatter)

	return &CLINotifier{
		verbose:        verbose,
		quiet:          quiet,
		logger:         eventLogger,
		progressLogger: progressLogger,
		progress:       make(map[string]*ProgressTracker),
		eventEmitter:   eventEmitter,
		profile:        profile,
	}
}

// NotifyProgress sends a progress update
// Also emits progress event through coordinator if available
func (n *CLINotifier) NotifyProgress(op *Operation, progress int, message string) error {
	if n.quiet {
		return nil
	}

	var tracker *ProgressTracker
	_ = concurrency.RunInLockOrLog(&n.mu, locknames.LockNameCliNotifierProgress, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		var exists bool
		tracker, exists = n.progress[op.ID]
		if !exists {
			tracker = &ProgressTracker{
				OperationID: op.ID,
				ObjectID:    op.ObjectID,
				Type:        op.Type,
			}
			n.progress[op.ID] = tracker
		}
		tracker.Progress = progress
		tracker.Message = message
		tracker.LastUpdate = time.Now()
		return nil
	})

	// Use progress logger instead of direct fmt.Fprintf
	// This ensures compliance with POLICY-CODE-007 (All Output Through Logging Framework)
	n.progressLogger.Progress(op.ID, progress, message,
		logging.String("object_id", op.ObjectID),
		logging.String(ConstMiscOperationType, string(op.Type)))

	// Emit progress event through event emitter (required)
	// Use system context for background event emission
	ctx := pkgctx.NewSystemContext()
	// Determine if this is a milestone (25%, 50%, 75%, 100%)
	emitAudit := progress == 25 || progress == 50 || progress == 75 || progress == 100

	fields := map[string]any{
		"object_id":            op.ObjectID,
		ConstMiscOperationType: string(op.Type),
	}
	var _err_82447083 = n.eventEmitter.EmitProgress(ctx, op.ID, string(op.Type), progress, 100, message, fields, emitAudit)
	if //nolint:errcheck // Best effort
	_err_82447083 != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.

			// NotifyStatus sends a status change notification
			// Also emits status change event through coordinator if available
			ProfileSystem))).Error(ErrMsgSwallowedError,

			_err_82447083).Log()
	}

	return nil
}

func (n *CLINotifier) NotifyStatus(op *Operation, oldStatus, newStatus OperationStatus) error {
	if n.quiet {
		return nil
	}

	// Use progress logger instead of direct fmt.Fprintf
	// This ensures compliance with POLICY-CODE-007 (All Output Through Logging Framework)
	n.progressLogger.Status(op.ID, string(oldStatus), string(newStatus),
		logging.String("object_id", op.ObjectID),
		logging.String(ConstMiscOperationType, string(op.Type)))

	// Emit status change event through event emitter (required)
	// Use system context for background event emission
	ctx := pkgctx.NewSystemContext()
	fields := map[string]any{
		"object_id":            op.ObjectID,
		ConstMiscOperationType: string(op.Type),
	}
	var _err_82448676 = n.eventEmitter.EmitStatusChange(ctx, op.ID, string(op.Type), string(oldStatus), string(newStatus), "", fields)
	if //nolint:errcheck // Best effort
	_err_82448676 != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.

			// NotifyError sends an error notification
			// Also emits error event through coordinator if available
			ProfileSystem))).Error(ErrMsgSwallowedError,

			_err_82448676).Log()
	}

	return nil
}

func (n *CLINotifier) NotifyError(op *Operation, err error) error {
	if n.quiet {
		return nil
	}

	// Use event logger for errors (errors should be logged, not just progress)
	// This ensures compliance with POLICY-CODE-007 (All Output Through Logging Framework)
	StorageLog(n.logger.Logger()).Error(LogEventStorageCLINotifierOperationFailedErr, err).
		String("operation_id", op.ID).
		ObjectID(op.ObjectID).
		String(ConstMiscOperationType, string(op.Type)).
		Log()

	// Emit error event through event emitter (required)
	// Use system context for background event emission
	ctx := pkgctx.NewSystemContext()
	fields := map[string]any{
		"object_id":            op.ObjectID,
		ConstMiscOperationType: string(op.Type),
	}
	var _err_82449599 = n.eventEmitter.EmitError(ctx, op.ID, string(op.Type), err, ConstMiscOperationFailed, fields)
	if //nolint:errcheck // Best effort
	_err_82449599 != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.

			// NotifyCompletion sends a completion notification
			// Also emits completion event through coordinator if available
			ProfileSystem))).Error(ErrMsgSwallowedError,

			_err_82449599).Log()
	}

	return nil
}

func (n *CLINotifier) NotifyCompletion(op *Operation) error {
	if n.quiet {
		return nil
	}

	var tracker *ProgressTracker
	var exists bool
	var duration time.Duration
	_ = concurrency.RunInLockOrLog(&n.mu, locknames.LockNameCliNotifierComplete, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		tracker, exists = n.progress[op.ID]
		if exists {
			duration = time.Since(tracker.LastUpdate)
			delete(n.progress, op.ID)
		}
		return nil
	})

	// Use progress logger for completion (100% progress)
	// This ensures compliance with POLICY-CODE-007 (All Output Through Logging Framework)
	n.progressLogger.Progress(op.ID, 100, "Completed",
		logging.String("object_id", op.ObjectID),
		logging.String(ConstMiscOperationType, string(op.Type)))

	// Emit completion event through event emitter (required)
	// Use system context for background event emission
	ctx := pkgctx.NewSystemContext()
	fields := map[string]any{
		"object_id":            op.ObjectID,
		ConstMiscOperationType: string(op.Type),
	}
	var _err_82450554 = n.eventEmitter.EmitCompletion(ctx, op.ID, string(op.Type), duration, ConstMiscOperationCompleted, fields)
	if //nolint:errcheck // Best effort
	_err_82450554 != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.

			// GetProgress returns current progress for all operations
			ProfileSystem))).Error(ErrMsgSwallowedError,

			_err_82450554).Log()
	}

	return nil
}

func (n *CLINotifier) GetProgress() map[string]*ProgressTracker {
	var result map[string]*ProgressTracker
	_ = concurrency.RunInRLockOrLog(&n.mu, locknames.LockNameCliNotifierGetProgress, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		result = make(map[string]*ProgressTracker)
		for k, v := range n.progress {
			result[k] = v
		}
		return nil
	})
	return result
}
