// Extracted from audit_events_helper.go (BLI-CEF-STORAGE-DECOMPOSE-001).
package storage

import (
	"context"
	"fmt"
	"strings"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage/audit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// isFailClosedAuditEvent reports whether an audit event must be persisted fail-closed.
// Medium+/critical events and any target_id job-lifecycle events must fail or quarantine
// originating operations rather than silently swallowing persist errors.
func isFailClosedAuditEvent(options *AuditEventOptions) bool {
	if options == nil {
		return false
	}
	sev := strings.ToLower(options.Severity)
	if sev == "medium" || sev == "high" || sev == "critical" {
		return true
	}
	if options.TargetID != "" {
		if options.TargetKind == objects.KindSchedulerJob ||
			strings.HasPrefix(options.EventType, "scheduler_job_") ||
			options.EventType == audit.EventTypeSchedulerJobStarted ||
			options.EventType == audit.EventTypeSchedulerJobCompleted {
			return true
		}
	}
	return false
}

func CreateAuditEventWithBuilder(
	ctx context.Context,
	projectRoot string,
	secCtx *pkgctx.SecurityContext,
	storageProvider ObjectStorageProvider,
	options *AuditEventOptions,
) error {
	failClosed := isFailClosedAuditEvent(options)

	if projectRoot == emptyValue {
		if failClosed {
			err := fmt.Errorf("fail-closed audit event failed: project root is empty")
			if options != nil && options.OnError != nil {
				options.OnError(err)
			}
			return err
		}
		return nil // Best effort - can't create without project root
	}

	// Prevent cycles: don't create audit events during audit event creation
	if audit.IsCreatingEvent() {
		return nil // Best effort - skip to prevent infinite recursion
	}

	audit.ApplySessionEnv(options)

	defer audit.BeginEventCreation()()

	// Get storage provider - use provided one, or get from cache (avoids expensive factory creation)
	if storageProvider == nil {
		// Get or create cached provider for this projectRoot (thread-safe using ResourceCache)
		provider, err := cachedStorageProviders.GetOrCreate(ctx, projectRoot)
		if err != nil {
			logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
			StorageLog(logger).Warn(LogEventStorageAuditStorageProviderUnavailable).
				WithError(err).
				Log()
			if failClosed {
				if options != nil && options.OnError != nil {
					options.OnError(err)
				}
				return err
			}
			return nil // Best effort - return early if provider creation fails
		}
		storageProvider = provider
	}

	now := time.Now().UTC()
	auditDir := audit.MonthlyDir(projectRoot, now)
	if err := fileutil.MkdirAll(auditDir, paths.DirPerm755); err != nil {
		if failClosed {
			if options != nil && options.OnError != nil {
				options.OnError(err)
			}
			return err
		}
		return nil // Best effort
	}

	// Use thread-safe batch ID generator (no retry logic needed)
	// The generator acquires IDs in batches to minimize lock contention
	// Pass storageProvider to enable CAS-aware ID generation (finds existing IDs in CAS)
	var ids audit.IDAllocator = GetAuditIDGenerator(ctx, auditDir, storageProvider)
	auditID, err := ids.GenerateNextID()
	if err != nil {
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		StorageLog(logger).Warn(LogEventStorageAuditIDGenerateFailed).
			String(logKeyAuditDir, auditDir).
			WithError(err).
			Log()
		if failClosed {
			if options != nil && options.OnError != nil {
				options.OnError(err)
			}
			return err
		}
		return nil // Best effort
	}

	// Get actor from security context or options
	actor := audit.ResolveActor(secCtx, options.CreatedBy)

	eventType, metadata := NormalizeAuditEventType(options.EventType, options.Metadata)
	options.EventType = eventType
	options.Metadata = metadata

	createdAt := options.CreatedAt
	if createdAt == emptyValue {
		createdAt = now.Format(time.RFC3339)
	}

	builder, err := audit.NewEventBuilder()
	if err != nil {
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		StorageLog(logger).Warn(LogEventStorageAuditBuilderUnavailable).
			String(logKeyAuditID, auditID).
			WithError(err).
			Log()
		if failClosed {
			if options != nil && options.OnError != nil {
				options.OnError(err)
			}
			return err
		}
		return nil // Best effort
	}
	audit.PopulateEvent(builder, auditID, projectRoot, actor, createdAt, options)

	// Get buffer and check if event should be aggregated
	bufferRegistry := GetGlobalBufferRegistry()
	buffer := bufferRegistry.GetOrCreate(projectRoot, secCtx)
	buffer.SetProjectRoot(projectRoot)
	if secCtx != nil {
		buffer.SetSecurityContext(secCtx)
	}

	usedCAS := false
	if fileStorage, ok := storageProvider.(*FileObjectStorage); ok {
		buffer.SetFileStorage(fileStorage)
		usedCAS = fileStorage.usesContentAddressableStorage(auditEventKind)
	}

	// Initialize buffer with config if not already initialized
	if err := InitializeGlobalBufferWithConfig(projectRoot, secCtx); err != nil {
		// Config load failed - log warning and continue with defaults (buffer already has defaults)
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		StorageLog(logger).Warn(LogEventStorageAuditBufferInitFailedContinueDefaults).
			String(logKeyProjectRoot, projectRoot).
			WithError(err).
			Log()
	}

	var buf audit.Buffer = buffer

	dispatched := audit.Dispatch(ctx, secCtx, builder, auditID, options, audit.DispatchDeps{
		Store:   storageProvider,
		Buffer:  buf,
		IDs:     ids,
		Metrics: GetGlobalAuditMetricsCollector(),
		UsedCAS: usedCAS,
		IsAlreadyExists: func(err error) bool {
			return audit.IsAlreadyExists(err, ErrObjectExists, ConstAuditAlreadyExists)
		},
	})
	switch audit.ClassifyDispatch(dispatched) {
	case audit.DispatchBufferBuildFailed:
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		StorageLog(logger).Warn(LogEventStorageAuditBuildForBufferFailed).
			String(logKeyAuditID, auditID).
			WithError(dispatched.BufferBuildErr).
			Log()
		if failClosed {
			if options != nil && options.OnError != nil {
				options.OnError(dispatched.BufferBuildErr)
			}
			return dispatched.BufferBuildErr
		}
		return nil
	case audit.DispatchImmediateBuildFailed:
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		StorageLog(logger).Warn(LogEventStorageAuditBuildImmediateFailed).
			String(logKeyAuditID, auditID).
			WithError(dispatched.ImmediateBuildErr).
			Log()
		if failClosed {
			if options != nil && options.OnError != nil {
				options.OnError(dispatched.ImmediateBuildErr)
			}
			return dispatched.ImmediateBuildErr
		}
		return nil
	case audit.DispatchBuffered:
		return nil
	case audit.DispatchRetrySucceeded:
		return nil
	}

	if dispatched.BufferAddErr != nil {
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		StorageLog(logger).Warn(LogEventStorageAuditBufferAddFailedImmediate).
			String(logKeyAuditID, auditID).
			WithError(dispatched.BufferAddErr).
			Log()
	}

	if eventID := audit.CacheableEventID(dispatched); eventID != emptyValue {
		updateHighVolumeEventCacheOnCreate(ctx, storageProvider, eventID, dispatched.Instance, options)
	}

	createErr := dispatched.Persist.Err
	switch audit.ClassifyPersistLog(dispatched) {
	case audit.PersistLogDuplicate:
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		// Duplicate IDs are already treated as success. Do not List audit_event
		// to increment occurrence_count: SortBy + Limit=1 still opens every
		// stream segment (thousands of files / hundreds of MB) and pegs
		// long-lived MCP/scheduler daemons at multi-core CPU.
		// TRACK: follow-up in kernel backlog
		StorageLog(logger).Debug(LogEventStorageAuditDuplicateMergeSkippedPerformance).
			String(logKeyAuditID, auditID).
			String(logKeyEventType, options.EventType).
			String(logKeyTargetID, options.TargetID).
			String(logKeyTargetKind, options.TargetKind).
			Log()
		return nil
	case audit.PersistLogError:
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		StorageLog(logger).Warn(LogEventStorageAuditCreateViaProviderFailed).
			String(logKeyAuditID, auditID).
			String(logKeyEventType, options.EventType).
			String(logKeyOperation, options.Operation).
			String(logKeyErrorDetail, createErr.Error()).
			WithError(createErr).
			Log()
	}
	if createErr != nil {
		if options.OnError != nil {
			options.OnError(createErr)
		}
		if failClosed {
			return createErr
		}
	}

	return nil
}

// BuildAuditEventInstance builds a single audit event instance map from options without creating or buffering.
// Used by FlushPendingAuditEvents to build a batch for BulkCreate. storageProvider is used for ID generation.
