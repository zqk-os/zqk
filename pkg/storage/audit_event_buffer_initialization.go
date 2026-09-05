package storage

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"

	"github.com/lanceman/zqk/pkg/concurrency"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/storage/locknames"
)

var (
	globalBuffer      *AuditEventBuffer
	bufferOnce        sync.Once
	bufferMu          sync.RWMutex
	bufferInitialized bool

	globalAuditBufferTracker   []*AuditEventBuffer
	globalAuditBufferTrackerMu sync.Mutex
)

// GetGlobalAuditEventBuffer returns the global audit event buffer instance
// It loads configuration from .zqk/config.yaml if projectRoot is available.
// Reads of globalBuffer are guarded by bufferMu so they synchronize with
// InitializeGlobalBufferWithConfig (which may replace the buffer).
func GetGlobalAuditEventBuffer() *AuditEventBuffer {
	bufferOnce.Do(func() {
		globalBuffer = NewAuditEventBuffer("", nil, DefaultAggregationRules())
		// Register with shutdown coordinator so InitiateShutdown() propagates to buffer
		coordinator := GetGlobalShutdownCoordinator()
		if coordinator != nil {
			coordinator.RegisterQueue(globalBuffer)
		}
	})
	var buf *AuditEventBuffer
	if err := concurrency.RunInRLockWithLogger(&bufferMu, locknames.LockNameAuditBufferGetGlobal, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		buf = globalBuffer
		return nil
	}); err != nil {
		logging.FluentEvent(logging.GetLogger()).Error(fmt.Sprintf(ErrMsgLockGetGlobal, err), nil).Log()
	}
	return buf
}

// FlushGlobalAuditBufferForProjectRoot flushes the global audit buffer if it is configured
// for the given project root. Used by tests to deterministically flush pending audit writes
// before temp dir cleanup (avoids "directory not empty" without sleep).
func FlushGlobalAuditBufferForProjectRoot(projectRoot string) {
	buf := GetGlobalAuditEventBuffer()
	if buf == nil || buf.GetProjectRoot() != projectRoot {
		return
	}
	if err := buf.Flush(); err != nil {
		logging.FluentEvent(logging.GetLogger()).Error(fmt.Sprintf(ErrMsgFlushFailRoot, projectRoot, err), nil).Log()
	}
}

// TearDownGlobalAuditBufferForTestProjectRoot flushes the global audit buffer when it belongs to
// projectRoot, then replaces it with a buffer rooted at resetRoot via InitializeGlobalBufferWithConfig.
// That cancels the previous buffer's periodic flush goroutine so async writes cannot leave
// docs/process/audit/... under a temp dir after RemoveAll. Callers should create resetRoot with
// os.MkdirTemp outside projectRoot and remove it after this returns. If the global buffer does not
// match projectRoot, this is a no-op (another test may own the buffer).
func TearDownGlobalAuditBufferForTestProjectRoot(projectRoot, resetRoot string, secCtx *pkgctx.SecurityContext) error {
	buf := GetGlobalAuditEventBuffer()
	if buf == nil || buf.GetProjectRoot() != projectRoot {
		return nil
	}
	if err := buf.Flush(); err != nil {
		logging.FluentEvent(logging.GetLogger()).Error(fmt.Sprintf(ErrMsgFlushFailRoot, projectRoot, err), nil).Log()
	}
	if err := fileutil.MkdirAll(resetRoot, paths.DirPerm755); err != nil {
		return err
	}
	return InitializeGlobalBufferWithConfig(resetRoot, secCtx)
}

// InitializeGlobalBufferWithConfig initializes the global buffer with configuration from project root.
// Avoids data races by creating a new buffer with the desired config and replacing the global
// reference, then cancelling the old buffer. Never mutates a buffer after its periodicFlush
// goroutine is started (no write to ctx/flushTicker while periodicFlush reads them).
func InitializeGlobalBufferWithConfig(projectRoot string, secCtx *pkgctx.SecurityContext) error {
	return concurrency.RunInLockWithLogger(&bufferMu, locknames.LockNameAuditBufferInitGlobal, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		if bufferInitialized && globalBuffer != nil && globalBuffer.projectRoot == projectRoot {
			return nil
		}

		config, err := LoadAggregationConfig(projectRoot)
		if err != nil {
			config = DefaultAggregationConfig()
		}

		windowSize := time.Hour
		if config.WindowSize != emptyValue {
			parsed, err := ParseWindowSize(config.WindowSize)
			if err == nil {
				windowSize = parsed
			}
		}

		// Preserve disabled state when replacing (e.g. tests call SetEnabled(false))
		enabled := config.Enabled
		oldBuffer := globalBuffer
		if oldBuffer != nil && !oldBuffer.IsEnabled() {
			enabled = false
		}
		newBuffer := NewAuditEventBufferWithConfig(projectRoot, secCtx, config.Rules, windowSize, config.Threshold, enabled)
		globalBuffer = newBuffer
		bufferInitialized = true
		if oldBuffer != nil {
			oldBuffer.cancel()
		}
		return nil
	})
}

// NewAuditEventBuffer creates a new audit event buffer with default window and threshold.
func NewAuditEventBuffer(projectRoot string, secCtx *pkgctx.SecurityContext, rules []AggregationRule) *AuditEventBuffer {
	return NewAuditEventBufferWithConfig(projectRoot, secCtx, rules, time.Hour, DefaultAggregationThreshold, true)
}

// NewAuditEventBufferWithConfig creates a new audit event buffer with the given window size,
// threshold, and enabled flag. The buffer's ctx and flushTicker are set at creation and must
// not be mutated afterward so periodicFlush can read them without a data race.
func NewAuditEventBufferWithConfig(projectRoot string, secCtx *pkgctx.SecurityContext, rules []AggregationRule, windowSize time.Duration, threshold int, enabled bool) *AuditEventBuffer {
	ctx, cancel := context.WithCancel(pkgctx.NewSystemContext())

	buffer := &AuditEventBuffer{
		buffer:          make(map[string]*AggregationGroup),
		windowSize:      windowSize,
		threshold:       threshold,
		stopChan:        make(chan struct{}),
		ctx:             ctx,
		cancel:          cancel,
		projectRoot:     projectRoot,
		secCtx:          secCtx,
		enabled:         enabled,
		preserveSamples: DefaultPreserveSamples,
		rules:           rules,
	}

	buffer.flushTicker = time.NewTicker(buffer.windowSize)
	goroutinelabels.NewGoroutine(OpNameAuditEventBufferPeriodicFlush, DescPeriodicFlush).
		WithCleanup(func() {
			if buffer.flushTicker != nil {
				buffer.flushTicker.Stop()
			}
		}).
		StartWithContext(buffer.ctx, func(ctx context.Context) error {
			buffer.periodicFlush()
			return nil
		})

	// Register with global tracker for test teardown
	_ = concurrency.RunInLock(&globalAuditBufferTrackerMu, func() error {
		globalAuditBufferTracker = append(globalAuditBufferTracker, buffer)
		return nil
	})

	return buffer
}

// ShutdownAllAuditBuffersForTesting forcefully shuts down all created AuditEventBuffers.
// Called by project_test_teardown.go to prevent goroutine leaks.
func ShutdownAllAuditBuffersForTesting() {
	var bufs []*AuditEventBuffer
	_ = concurrency.RunInLock(&globalAuditBufferTrackerMu, func() error {
		bufs = globalAuditBufferTracker
		globalAuditBufferTracker = nil
		return nil
	})
	for _, b := range bufs {
		_ = b.Shutdown()
	}
}

// SetEnabled enables or disables the buffer
func (b *AuditEventBuffer) SetEnabled(enabled bool) {
	if err := concurrency.RunInLockWithLogger(&b.mu, locknames.LockNameAuditBufferSetEnabled, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		b.enabled = enabled
		return nil
	}); err != nil {
		logging.FluentEvent(logging.GetLogger()).Error(fmt.Sprintf(ErrMsgLockSetEnabled, err), nil).Log()
	}
}

// IsEnabled returns whether the buffer is enabled
func (b *AuditEventBuffer) IsEnabled() bool {
	var enabled bool
	if err := concurrency.RunInRLockWithLogger(&b.mu, locknames.LockNameAuditBufferIsEnabled, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		enabled = b.enabled
		return nil
	}); err != nil {
		logging.FluentEvent(logging.GetLogger()).Error(fmt.Sprintf(ErrMsgLockIsEnabled, err), nil).Log()
	}
	return enabled
}

// SetFileStorage sets the fileStorage instance for CAS routing.
// Uses TryLock so that when called reentrantly from the flush path (e.g. CreateAuditEventWithBuilder
// during flushGroupLocked), we skip the set to avoid deadlock; the buffer is already configured.
func (b *AuditEventBuffer) SetFileStorage(fileStorage *FileObjectStorage) {
	if !b.mu.TryLock() {
		return
	}
	defer b.mu.Unlock()
	b.fileStorage = fileStorage
}

// SetTSDBProvider sets the TSDBProvider instance for time-series persistence.
// Uses TryLock so that when called reentrantly from the flush path, we skip the set to avoid deadlock.
func (b *AuditEventBuffer) SetTSDBProvider(tsdb TSDBProvider) {
	if !b.mu.TryLock() {
		return
	}
	defer b.mu.Unlock()
	b.tsdb = tsdb
}

// SetFlushErrorCallback sets the callback function to be called when flush operations complete
// This allows callers to be notified of flush successes and failures
func (b *AuditEventBuffer) SetFlushErrorCallback(callback FlushErrorCallback) {
	if err := concurrency.RunInLockWithLogger(&b.mu, locknames.LockNameAuditBufferSetFlushCallback, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		b.flushErrorCallback = callback
		return nil
	}); err != nil {
		logging.FluentEvent(logging.GetLogger()).Error(fmt.Sprintf(ErrMsgLockSetFlushErr, err), nil).Log()
	}
}

// SetFlushProgressChannel sets the channel for receiving flush progress/error events
// This allows callers to monitor flush operations via event stream
func (b *AuditEventBuffer) SetFlushProgressChannel(ch chan FlushProgress) {
	if err := concurrency.RunInLockWithLogger(&b.mu, locknames.LockNameAuditBufferSetFlushChannel, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		b.flushProgressChan = ch
		return nil
	}); err != nil {
		logging.FluentEvent(logging.GetLogger()).Error(fmt.Sprintf(ErrMsgLockSetFlushCh, err), nil).Log()
	}
}

// SetProjectRoot sets the project root for audit event storage.
// Uses TryLock so that when called reentrantly from the flush path (e.g. CreateAuditEventWithBuilder
// during flushGroupLocked -> WriteSystemObjectAndRegisterHash -> createCacheAuditEvent), we skip
// the set to avoid deadlock; the buffer is already configured for this project root.
func (b *AuditEventBuffer) SetProjectRoot(projectRoot string) {
	if !b.mu.TryLock() {
		return
	}
	defer b.mu.Unlock()
	b.projectRoot = projectRoot
}

// GetProjectRoot returns the project root for audit event storage (for tests that need to flush before cleanup).
func (b *AuditEventBuffer) GetProjectRoot() string {
	var root string
	if err := concurrency.RunInRLockWithLogger(&b.mu, locknames.LockNameAuditBufferGetProjectRoot, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		root = b.projectRoot
		return nil
	}); err != nil {
		logging.FluentEvent(logging.GetLogger()).Error(fmt.Sprintf(ErrMsgLockGetProjRoot, err), nil).Log()
	}
	return root
}

// SetSecurityContext sets the security context for audit event creation.
// Uses TryLock so that when called reentrantly from the flush path, we skip the set to avoid deadlock.
func (b *AuditEventBuffer) SetSecurityContext(secCtx *pkgctx.SecurityContext) {
	if !b.mu.TryLock() {
		return
	}
	defer b.mu.Unlock()
	b.secCtx = secCtx
}

// getContext returns a context for operations (background context for long-running flush operations).
// Callers MUST call the returned cancel func to avoid context leaks.
func (b *AuditEventBuffer) getContext() (context.Context, context.CancelFunc) {
	// For background flush operations, use system context with timeout
	// This prevents indefinite hangs while still allowing operations to complete
	ctx, cancel := context.WithTimeout(pkgctx.NewSystemContext(), 30*time.Second)
	return ctx, cancel
}
