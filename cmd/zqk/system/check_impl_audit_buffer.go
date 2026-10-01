package system

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"maps"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/concurrency"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/migration/parser"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	storageaudit "github.com/zqk-os/zqk/pkg/storage/audit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
	"github.com/zqk-os/zqk/pkg/zqktime"
)

type AuditEventBuffer struct {
	mu          sync.RWMutex
	buffer      map[string]*BufferedEvent // key -> event
	projectRoot string
	ctx         *cli.Context
	isBulk      bool // Whether we're in a bulk operation context
}

// BufferedEvent represents an audit event being buffered
type BufferedEvent struct {
	Key             string // "target_id:event_type" or "event_type"
	EventType       string
	TargetID        string
	TargetKind      string
	TargetPath      string
	Operation       string
	Severity        string
	Metadata        map[string]any
	Occurrences     []time.Time // Timestamps of occurrences
	FirstOccurrence time.Time
	LastOccurrence  time.Time
	// Additional fields for specific event types
	OriginalValue  string // For hash_mismatch_fix
	NewValue       string // For hash_mismatch_fix
	RecoveryMethod string // For hash_mismatch_fix
}

var (
	globalAuditBuffer *AuditEventBuffer
	// bufferOnce is reserved for future use
	// bufferOnce        sync.Once
	bufferMu sync.Mutex
)

// GetAuditEventBuffer returns the global audit event buffer instance
func GetAuditEventBuffer(ctx *cli.Context) *AuditEventBuffer {
	var buffer *AuditEventBuffer
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	_ = concurrency.WithLockTimeout(
		&bufferMu,
		pkgctx.NewSystemContext(),
		nil,
		logging.NewLockLoggerAdapter(logger),
		LockNameAuditEventBufferGet,
		func() error {
			if globalAuditBuffer == nil {
				projectRoot := ctx.ProjectRoot
				projectRoot = ProjectRootOrResolve(projectRoot)
				globalAuditBuffer = &AuditEventBuffer{
					buffer:      make(map[string]*BufferedEvent),
					projectRoot: projectRoot,
					ctx:         ctx,
					isBulk:      true, // Assume bulk operation when buffer is created
				}
			}
			buffer = globalAuditBuffer
			return nil
		},
	)
	return buffer
}

// ResetAuditEventBuffer resets the global buffer (call at start of new bulk operation)
func ResetAuditEventBuffer() {
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	_ = concurrency.WithLockTimeout(
		&bufferMu,
		pkgctx.NewSystemContext(),
		nil,
		logging.NewLockLoggerAdapter(logger),
		LockNameAuditEventBufferReset,
		func() error {
			globalAuditBuffer = nil
			return nil
		},
	)
}

// generateKey generates a unique key for an event based on target_id and event_type
func (b *AuditEventBuffer) generateKey(targetID, eventType string) string {
	if targetID != emptyValue {
		return fmt.Sprintf("%s:%s", targetID, eventType)
	}
	return eventType
}

// mergeMetadata merges src into dst (latest wins). It is nil-safe:
// - src==nil: returns dst unchanged
// - dst==nil: allocates dst and returns it
func mergeMetadata(dst map[string]any, src map[string]any) map[string]any {
	if src == nil {
		return dst
	}
	if dst == nil {
		dst = make(map[string]any, len(src))
	}
	maps.Copy(dst, src)
	return dst
}

// Add adds an event to the buffer or updates an existing one
func (b *AuditEventBuffer) Add(eventType, targetID, targetKind, targetPath, operation, severity string, metadata map[string]any) {
	key := b.generateKey(targetID, eventType)

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	_ = concurrency.WithLockTimeout(
		&b.mu,
		pkgctx.NewSystemContext(),
		nil,
		logging.NewLockLoggerAdapter(logger),
		LockNameAuditEventBufferAdd,
		func() error {
			now := time.Now().UTC()
			if existing, ok := b.buffer[key]; ok {
				// Update existing: increment count, append timestamp
				existing.Occurrences = append(existing.Occurrences, now)
				existing.LastOccurrence = now
				// Merge metadata (latest wins for conflicting keys)
				if metadata != nil {
					existing.Metadata = mergeMetadata(existing.Metadata, metadata)
				}
			} else {
				// Create new buffered event
				b.buffer[key] = &BufferedEvent{
					Key:             key,
					EventType:       eventType,
					TargetID:        targetID,
					TargetKind:      targetKind,
					TargetPath:      targetPath,
					Operation:       operation,
					Severity:        severity,
					Metadata:        metadata,
					Occurrences:     []time.Time{now},
					FirstOccurrence: now,
					LastOccurrence:  now,
				}
			}
			return nil
		},
	)
}

// AddHashMismatchFix adds a hash mismatch fix event with additional fields
func (b *AuditEventBuffer) AddHashMismatchFix(obj *parser.ParsedObject, filePath, kind, originalHash, newHash string) {
	key := b.generateKey(obj.ID, "hash_mismatch_fix")

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	_ = concurrency.WithLockTimeout(
		&b.mu,
		pkgctx.NewSystemContext(),
		nil,
		logging.NewLockLoggerAdapter(logger),
		LockNameAuditEventBufferAddHashFix,
		func() error {
			now := time.Now().UTC()
			relPath := filePath
			if b.projectRoot != emptyValue {
				if rel, err := filepath.Rel(b.projectRoot, filePath); err == nil {
					relPath = rel
				}
			}

			operation := fmt.Sprintf("Regenerated integrity hash for %s (hash mismatch resolved)", obj.ID)
			metadata := map[string]any{
				objects.FieldKeyCommand: fmt.Sprintf("%s check --force", paths.CLICommandName),
				objects.FieldKeyContext: b.ctx.Profile,
				"project_root":          b.projectRoot,
			}

			if existing, ok := b.buffer[key]; ok {
				// Update existing
				existing.Occurrences = append(existing.Occurrences, now)
				existing.LastOccurrence = now
				// Update with latest values
				existing.NewValue = newHash
				existing.Metadata = metadata
			} else {
				// Create new
				b.buffer[key] = &BufferedEvent{
					Key:             key,
					EventType:       "hash_mismatch_fix",
					TargetID:        obj.ID,
					TargetKind:      kind,
					TargetPath:      relPath,
					Operation:       operation,
					Severity:        "high",
					Metadata:        metadata,
					Occurrences:     []time.Time{now},
					FirstOccurrence: now,
					LastOccurrence:  now,
					OriginalValue:   originalHash,
					NewValue:        newHash,
					RecoveryMethod:  "force",
				}
			}
			return nil
		},
	)
}

// findExistingAuditEvent finds an existing audit event by target_id and event_type
// Searches in current month's audit directory
func (b *AuditEventBuffer) findExistingAuditEvent(targetID, eventType string) (event map[string]any, filePath string, err error) {
	if targetID == emptyValue {
		// Can't reliably find events without target_id
		return nil, "", errfmt.Errorf("no target_id provided")
	}

	// Search in current month's audit directory (stream first, fallback to legacy)
	now := time.Now().UTC()
	auditDir := storageaudit.MonthlyDir(b.projectRoot, now)
	entries, err := fileutil.ReadDir(auditDir)
	if err != nil || len(entries) == 0 {
		month := now.Format("2006-01")
		processBase := paths.ResolvePathFromCacheOrConstant(b.projectRoot, "process", paths.ProcessDir)
		legacyDir := filepath.Join(processBase, "audit", month)
		if legacyEntries, lErr := fileutil.ReadDir(legacyDir); lErr == nil && len(legacyEntries) > 0 {
			auditDir = legacyDir
			entries = legacyEntries
			err = nil
		}
	}
	if err != nil {
		return nil, "", err
	}

	// Scan audit events in this month
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}

		auditPath := filepath.Join(auditDir, entry.Name())
		data, err := fileutil.ReadFile(auditPath)
		if err != nil {
			continue
		}

		var event map[string]any
		if err := yaml.Unmarshal(data, &event); err != nil {
			continue
		}

		// Check if this event matches
		eventTargetID, _ := event[objects.FieldKeyTargetID].(string)
		eventTypeVal, _ := event[objects.FieldKeyEventType].(string)

		if eventTargetID == targetID && eventTypeVal == eventType {
			return event, auditPath, nil
		}
	}

	return nil, "", errfmt.Errorf("event not found")
}

// updateAuditEvent updates an existing audit event with occurrence data
func (b *AuditEventBuffer) updateAuditEvent(existing map[string]any, auditPath string, buffered *BufferedEvent) error {
	// Get current occurrence count
	currentCount := 1
	switch count := existing[objects.FieldKeyOccurrenceCount].(type) {
	case int:
		currentCount = count
	case int64:
		currentCount = int(count)
	}

	// Get existing timestamps
	timestamps := []string{}
	switch ts := existing[objects.FieldKeyOccurrenceTimestamps].(type) {
	case []any:
		for _, t := range ts {
			if str, ok := t.(string); ok {
				timestamps = append(timestamps, str)
			}
		}
	default:
		// If no timestamps, use created_at as first occurrence
		if created, ok := existing[objects.FieldKeyCreatedAt].(string); ok {
			timestamps = append(timestamps, created)
		}
	}

	// Append new timestamps
	for _, t := range buffered.Occurrences {
		timestamps = append(timestamps, t.Format("2006-01-02T15:04:05Z"))
	}

	// Update event
	existing[objects.FieldKeyOccurrenceCount] = currentCount + len(buffered.Occurrences)
	existing[objects.FieldKeyOccurrenceTimestamps] = timestamps
	existing[objects.FieldKeyUpdatedAt] = zqktime.NowLayoutUTC(zqktime.LayoutObjectDateTimeZ)
	existing[objects.FieldKeyUpdatedBy] = pkgctx.ActorIDForAttribution(b.ctx.Profile)

	// Update with latest values if applicable
	if buffered.NewValue != emptyValue {
		existing[objects.FieldKeyNewValue] = buffered.NewValue
	}
	if buffered.Metadata != nil {
		// Merge metadata
		if existingMeta, ok := existing[objects.FieldKeyMetadata].(map[string]any); ok {
			existing[objects.FieldKeyMetadata] = mergeMetadata(existingMeta, buffered.Metadata)
		} else {
			existing[objects.FieldKeyMetadata] = buffered.Metadata
		}
	}

	// Write updated event using CAS routing
	// Extract auditID from the event or file path
	auditID, ok := existing[objects.FieldKeyID].(string)
	if !ok {
		// Fallback: extract from file path
		baseName := filepath.Base(auditPath)
		auditID = strings.TrimSuffix(baseName, ".yaml")
	}

	auditDir := filepath.Dir(auditPath)
	projectRoot := b.projectRoot

	if err := writeAuditEventWithCAS(projectRoot, auditDir, auditID, existing); err != nil {
		return errfmt.Newf("failed to write updated audit event").Wrap(err)
	}

	return nil
}

// createAuditEvent creates a new audit event from a buffered event using instance builder
func (b *AuditEventBuffer) createAuditEvent(buffered *BufferedEvent) error {
	// Get storage provider - create via StorageFactory
	factory, err := storage.NewStorageFactory(pkgctx.NewSystemContext(), b.projectRoot)
	var fileStorage storage.ObjectStorageProvider
	if factory != nil {
		fileStorage = factory.GetStorage()
		defer func() { _ = fileStorage.Shutdown(context.Background()) }() // Background: request-or-shutdown derived
	}
	if err != nil {
		logger := logging.GetLoggerFromProfile(b.ctx.Profile)
		logging.Fluent(logger).Warn("Failed to create FileObjectStorage for buffered audit event, skipping creation (requires CAS routing)").
			WithError(err).
			Log()
		return nil // Best effort - skip rather than create non-CAS audit event
	}

	// Emit via coordinator (async, non-blocking)
	stdctx := pkgctx.NewSystemContext()
	profile := b.ctx.Profile
	if profile == emptyValue {
		profile = systemProfileSystem
	}
	emitBufferedAuditEventViaCoordinator(
		stdctx,
		b.projectRoot,
		fileStorage,
		buffered,
		profile,
	)

	return nil
}

// GetBufferCount returns the number of buffered events (thread-safe)
func (b *AuditEventBuffer) GetBufferCount() int {
	var count int
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	_ = concurrency.WithRLockTimeout(
		&b.mu,
		pkgctx.NewSystemContext(),
		nil,
		logging.NewLockLoggerAdapter(logger),
		LockNameAuditEventBufferGetCount,
		func() error {
			count = len(b.buffer)
			return nil
		},
	)
	return count
}

// GetBufferSummary returns a summary of buffered events (thread-safe)
func (b *AuditEventBuffer) GetBufferSummary() map[string]int {
	var summary map[string]int
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	_ = concurrency.WithRLockTimeout(
		&b.mu,
		pkgctx.NewSystemContext(),
		nil,
		logging.NewLockLoggerAdapter(logger),
		LockNameAuditEventBufferGetSummary,
		func() error {
			summary = make(map[string]int)
			for _, buffered := range b.buffer {
				summary[buffered.EventType] += len(buffered.Occurrences)
			}
			return nil
		},
	)
	return summary
}

// Flush flushes all buffered events, creating new ones or updating existing ones
// Returns the number of events flushed and any errors
func (b *AuditEventBuffer) Flush() (int, error) {
	var eventsToFlush []*BufferedEvent
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	_ = concurrency.WithLockTimeout(
		&b.mu,
		pkgctx.NewSystemContext(),
		nil,
		logging.NewLockLoggerAdapter(logger),
		LockNameAuditEventBufferFlushCopy,
		func() error {
			// Copy events to flush (release lock before I/O)
			eventsToFlush = make([]*BufferedEvent, 0, len(b.buffer))
			for _, buffered := range b.buffer {
				eventsToFlush = append(eventsToFlush, buffered)
			}
			// Clear buffer after copying
			b.buffer = make(map[string]*BufferedEvent)
			return nil
		},
	)

	// Process events (NO LOCK HELD during I/O)
	var errors []error
	eventsFlushed := 0
	eventsUpdated := 0
	eventsCreated := 0

	for _, buffered := range eventsToFlush {
		key := buffered.Key
		// Check if audit event already exists
		if buffered.TargetID != emptyValue {
			existingEvent, auditPath, err := b.findExistingAuditEvent(buffered.TargetID, buffered.EventType)
			if err == nil && existingEvent != nil {
				// Update existing event
				if err := b.updateAuditEvent(existingEvent, auditPath, buffered); err != nil {
					errors = append(errors, errfmt.Errorf("failed to update audit event %s: %w", key, err))
					continue
				}
				eventsUpdated++
				eventsFlushed++
				continue
			}
		}

		// Create new event
		if err := b.createAuditEvent(buffered); err != nil {
			errors = append(errors, errfmt.Errorf("failed to create audit event %s: %w", key, err))
			continue
		}
		eventsCreated++
		eventsFlushed++
	}

	if len(errors) > 0 {
		return eventsFlushed, errfmt.Errorf("errors flushing audit events: %v", errors)
	}

	// Log flush summary if there were events
	if eventsFlushed > 0 {
		logger := logging.GetLoggerFromProfile(b.ctx.Profile)
		logging.Fluent(logger).Info("Audit events flushed").
			Total(eventsFlushed).
			EventsCreated(eventsCreated).
			EventsUpdated(eventsUpdated).
			Log()
	}

	return eventsFlushed, nil
}

func createHashMismatchFixAuditEvent(ctx *cli.Context, obj *parser.ParsedObject, filePath, kind, originalHash string) error {
	projectRoot := ctx.ProjectRoot
	projectRoot = ProjectRootOrResolve(projectRoot)

	// Read file to get new hash
	content, err := fileutil.ReadFile(filePath)
	if err != nil {
		return errfmt.Newf("failed to read file for audit event").Wrap(err)
	}
	hash := sha256.Sum256(content)
	newHash := hex.EncodeToString(hash[:])

	// Check if we're in a bulk operation (buffer exists and is active)
	bufferMu.Lock()
	isBulkOperation := globalAuditBuffer != nil
	bufferMu.Unlock()

	if isBulkOperation {
		// Use buffer to aggregate events
		buffer := GetAuditEventBuffer(ctx)
		buffer.AddHashMismatchFix(obj, filePath, kind, originalHash, newHash)
		return nil
	}

	// Non-bulk: create event immediately using instance builder
	// Get relative file path
	relPath, err := filepath.Rel(projectRoot, filePath)
	if err != nil {
		relPath = filePath
	}

	// Get storage provider - create via StorageFactory
	factory, err := storage.NewStorageFactory(pkgctx.NewSystemContext(), projectRoot)
	var fileStorage storage.ObjectStorageProvider
	if factory != nil {
		fileStorage = factory.GetStorage()
		defer func() { _ = fileStorage.Shutdown(context.Background()) }() // Background: request-or-shutdown derived
	}
	if err != nil {
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		logging.Fluent(logger).Warn("Failed to create StorageFactory for hash mismatch fix audit event, skipping creation").
			WithError(err).
			Log()
		return nil // Best effort - skip rather than create non-CAS audit event
	}

	// Emit via coordinator (async, non-blocking)
	stdctx := pkgctx.NewSystemContext()
	profile := profileOrDefault(ctx.Profile, systemProfileSystem)
	_ = emitHashMismatchFixEventViaCoordinator(
		stdctx,
		projectRoot,
		fileStorage,
		obj.ID,
		kind,
		relPath,
		originalHash,
		newHash,
		profile,
	) //nolint:errcheck // Best-effort audit fan-out

	return nil
}

// createCacheAuditEvent creates an audit event for cache operations
func createCacheAuditEvent(eventType, targetID, targetKind, targetPath, operation, severity string, profile string) {
	projectRoot := ProjectRootOrResolve("")
	if projectRoot == emptyValue {
		// Can't create audit event without project root
		return
	}

	// Get current user/actor (from environment or default)
	actor := zqkenv.POSIXUser().Get()
	if actor == emptyValue {
		actor = zqkenv.POSIXUsername().Get()
	}
	if actor == emptyValue {
		actor = "system"
	}

	// Get relative file path if provided
	relPath := targetPath
	if targetPath != emptyValue {
		var err error
		relPath, err = filepath.Rel(projectRoot, targetPath)
		if err != nil {
			relPath = targetPath
		}
	}

	// Build metadata
	metadata := map[string]any{
		"cache_operation": eventType,
		"project_root":    projectRoot,
	}

	// Build options for audit event creation
	options := &storage.AuditEventOptions{
		EventType:  eventType,
		Operation:  operation,
		Severity:   severity,
		TargetKind: targetKind,
		TargetID:   targetID,
		TargetPath: relPath,
		Metadata:   metadata,
		CreatedBy:  actor,
	}

	// Use CreateAuditEventWithBuilder (handles buffering internally, no hardcoding)
	// This uses the audit_event_builder pattern and handles aggregation automatically
	// Best practice: Pass nil for storageProvider - CreateAuditEventWithBuilder will create it
	// if needed (it handles caching internally), avoiding expensive factory creation on every call
	stdctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()
	secCtx.AccountID = actor

	// CreateAuditEventWithBuilder handles storage provider creation internally if nil
	// It checks ShouldAggregate and buffers or writes immediately as needed
	if err := storage.CreateAuditEventWithBuilder(stdctx, projectRoot, secCtx, nil, options); err != nil {
		// Builder failed - log but continue (best effort)
		logger := logging.GetLoggerFromProfile(profile)
		logging.Fluent(logger).Debug("Failed to create cache audit event via builder").
			WithError(err).
			Log()
	}
}

// createCacheAuditEventWithBuilder creates a cache audit event using the coordination system
// This routes cache audit events through the central event coordinator
func createCacheAuditEventWithBuilder(projectRoot string, secCtx *pkgctx.SecurityContext, options *storage.AuditEventOptions, profile string) {
	// Get storage provider - create via StorageFactory
	factory, err := storage.NewStorageFactory(pkgctx.NewSystemContext(), projectRoot)
	var fileStorage storage.ObjectStorageProvider
	if factory != nil {
		fileStorage = factory.GetStorage()
		defer func() { _ = fileStorage.Shutdown(context.Background()) }() // Background: request-or-shutdown derived
	}
	if err != nil {
		// If we can't get storage, we can't route through CAS
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		logging.Fluent(logger).Warn("Failed to create StorageFactory for cache audit event, skipping creation").
			WithError(err).
			Log()
		return // Best effort - skip rather than create non-CAS audit event
	}

	// Emit via coordinator (async, non-blocking)
	stdctx := pkgctx.NewSystemContext()
	emitCacheAuditEventViaCoordinator(stdctx, projectRoot, fileStorage, secCtx, options, profile)
	// Coordinator emits asynchronously, always succeeds
}

// writeAuditEventWithCAS writes an audit event. When stream storage is enabled for audit_event,
// uses storage.Create so the event goes to the stream (no YAML under .zqk/process). Otherwise
// uses WriteSystemObjectAndRegisterHash for CAS.
func writeAuditEventWithCAS(projectRoot, auditDir, auditID string, auditEvent map[string]any) error {
	factory, err := storage.NewStorageFactory(pkgctx.NewSystemContext(), projectRoot)
	var fileStorage storage.ObjectStorageProvider
	if factory != nil {
		fileStorage = factory.GetStorage()
		defer func() { _ = fileStorage.Shutdown(context.Background()) }() // Background: request-or-shutdown derived
	}
	if err != nil {
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		logging.Fluent(logger).Warn("Failed to create StorageFactory for audit event, skipping creation").
			String("audit_id", auditID).
			WithError(err).
			Log()
		return errfmt.Newf("cannot create audit event without storage").Wrap(err)
	}

	if storage.StreamStorageEnabledForKind(objects.KindAuditEvent) {
		ctx := pkgctx.NewSystemContext()
		secCtx := pkgctx.NewSystemSecurityContext()
		obj := make(map[string]any, len(auditEvent)+2)
		maps.Copy(obj, auditEvent)
		if obj[objects.FieldKeyID] == nil {
			obj[objects.FieldKeyID] = auditID
		}
		if obj[objects.FieldKeyKind] == nil {
			obj[objects.FieldKeyKind] = objects.KindAuditEvent
		}
		if err := fileStorage.Create(ctx, secCtx, obj); err != nil {
			return errfmt.Newf("failed to write audit event to stream").Wrap(err)
		}
		return nil
	}

	// Stream disabled: use CAS (WriteSystemObjectAndRegisterHash)
	data, err := storage.FormatMultiLineYAML(auditEvent)
	if err != nil {
		return errfmt.Newf("failed to format audit event").Wrap(err)
	}
	baseAuditDir := filepath.Dir(auditDir)
	auditFilePath := filepath.Join(auditDir, fmt.Sprintf("%s.yaml", auditID))
	rawFileStorage := storage.UnwrapToFileObjectStorage(fileStorage)
	if err := storage.WriteSystemObjectAndRegisterHash(auditFilePath, data, objects.KindAuditEvent, baseAuditDir, auditID, rawFileStorage); err != nil {
		return errfmt.Newf("failed to write audit event").Wrap(err)
	}
	return nil
}

// Helper functions

func isObjectKind(s string) bool {
	switch s {
	case objects.KindBacklogItem, objects.KindGoal, objects.KindMilestone, objects.KindWorkstream, objects.KindPriorityPlan,
		objects.KindRequirement, objects.KindTestCase, objects.KindCriteria, objects.KindDecision, objects.KindRoadmap:
		return true
	default:
		return false
	}
}

// discoverObjectKinds discovers object kinds using field registry (follows architecture pattern)
// Uses field registry abstraction instead of direct os.ReadDir to support both file and graph backends
