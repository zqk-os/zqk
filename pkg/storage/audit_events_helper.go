package storage

import (
	"context"
	"sync"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/storage/audit"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

const (
	// Audit event kind and spec (avoid magic strings).
	auditEventKind     = audit.Kind
	auditEventSpecFile = audit.SpecFile

	// Audit event field names (spec-aligned; use for SetField and map keys).
	auditFieldID             = audit.FieldID
	auditFieldTargetID       = audit.FieldTargetID
	auditFieldEventType      = audit.FieldEventType
	auditFieldCreatedAt      = audit.FieldCreatedAt
	auditStatusCompleted     = audit.StatusCompleted
	auditFieldOccurrenceCnt  = audit.FieldOccurrenceCnt
	auditFieldOccurrenceTs   = audit.FieldOccurrenceTs
	auditFieldOriginSystem   = audit.FieldOriginSystem
	auditFieldOriginProject  = audit.FieldOriginProject
	auditFieldNamespaceID    = audit.FieldNamespaceID
	auditFieldOperation      = audit.FieldOperation
	auditFieldSeverity       = audit.FieldSeverity
	auditFieldTitle          = audit.FieldTitle
	auditFieldCreatedBy      = audit.FieldCreatedBy
	auditFieldUpdatedAt      = audit.FieldUpdatedAt
	auditFieldUpdatedBy      = audit.FieldUpdatedBy
	auditFieldTargetKind     = audit.FieldTargetKind
	auditFieldTargetPath     = audit.FieldTargetPath
	auditFieldMetadata       = audit.FieldMetadata
	auditFieldOriginalValue  = audit.FieldOriginalValue
	auditFieldNewValue       = audit.FieldNewValue
	auditFieldRecoveryMethod = audit.FieldRecoveryMethod
	auditFieldReason         = audit.FieldReason
	auditFieldSessionID      = audit.FieldSessionID

	// Logging key names (structured log fields).
	logKeyAuditDir    = "audit_dir"
	logKeyAuditID     = "audit_id"
	logKeyEventType   = "event_type"
	logKeyTargetID    = "target_id"
	logKeyOperation   = "operation"
	logKeyTargetKind  = "target_kind"
	logKeyErrorDetail = "error_detail"
	logKeyProjectRoot = "project_root"

	// Target kind for merge-skip (scheduler_job).
	targetKindSchedulerJob = objects.KindSchedulerJob

	// Event type fallback when spec allowlist is loaded (stable, general-purpose).
	eventTypeFallbackSystemConfig = audit.EventTypeSystemConfigChange
)

// AuditEventOptions is the storage-root alias for audit.EventOptions.
type AuditEventOptions = audit.EventOptions

// AllowedAuditEventTypes mirrors the audit_event spec enum (loaded at runtime).
// Exported so coordinator routers can normalize inferred event types.
var (
	AllowedAuditEventTypes        map[string]struct{}
	allowedAuditEventTypesOnce    sync.Once
	allowedAuditEventTypeFallback string
)

// Cached storage providers per projectRoot to avoid expensive factory creation on every audit event
// Uses ResourceCache abstraction for thread-safe caching
var (
	cachedStorageProviders = GetGlobalStorageProviderCache()
)

type allowedAuditEventTypeInfo struct {
	allowed  map[string]struct{}
	fallback string
}

// loadAllowedAuditEventTypeInfo loads the enum list from audit_event.yaml to avoid drift.
// It also derives a deterministic fallback value:
// - Prefer "system_config_change" if present (stable and general-purpose)
// - Otherwise, use the first enum entry as defined in the spec
func loadAllowedAuditEventTypeInfo() allowedAuditEventTypeInfo {
	loader := objects.GetGlobalSpecLoader()
	spec, err := loader.LoadSpecWithInheritance(auditEventSpecFile)
	if err != nil || spec == nil {
		StorageLog(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).
			Warn(LogEventStorageAuditAllowedTypesSpecLoadFailed).
			WithError(err).
			Log()
		return allowedAuditEventTypeInfo{allowed: map[string]struct{}{}, fallback: ""}
	}

	fieldDef, ok := spec.ResolvedFields[auditFieldEventType].(map[string]any)
	if !ok {
		StorageLog(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).
			Warn(LogEventStorageAuditAllowedTypesMissingFieldDef).
			Log()
		return allowedAuditEventTypeInfo{allowed: map[string]struct{}{}, fallback: ""}
	}

	validation, ok := fieldDef["validation"].(map[string]any)
	if !ok {
		StorageLog(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).
			Warn(LogEventStorageAuditAllowedTypesMissingValidation).
			Log()
		return allowedAuditEventTypeInfo{allowed: map[string]struct{}{}, fallback: ""}
	}

	rawEnum, ok := validation["enum"].([]any)
	if !ok {
		// YAML decoding can yield []any
		if alt, okAlt := validation["enum"].([]any); okAlt {
			rawEnum = alt
		} else {
			StorageLog(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).
				Warn(LogEventStorageAuditAllowedTypesEnumNotFound).
				Log()
			return allowedAuditEventTypeInfo{allowed: map[string]struct{}{}, fallback: ""}
		}
	}

	allowed := make(map[string]struct{}, len(rawEnum))
	fallback := ""
	for _, v := range rawEnum {
		if s, ok := v.(string); ok && s != emptyValue {
			allowed[s] = struct{}{}
			if fallback == emptyValue {
				fallback = s // first enum entry, preserves spec order
			}
		}
	}

	// Prefer a stable, general-purpose fallback if the spec includes it.
	if _, ok := allowed[eventTypeFallbackSystemConfig]; ok {
		fallback = eventTypeFallbackSystemConfig
	}

	return allowedAuditEventTypeInfo{allowed: allowed, fallback: fallback}
}

func getAllowedAuditEventTypes() map[string]struct{} {
	allowedAuditEventTypesOnce.Do(func() {
		info := loadAllowedAuditEventTypeInfo()
		AllowedAuditEventTypes = info.allowed
		allowedAuditEventTypeFallback = info.fallback
	})
	return AllowedAuditEventTypes
}

func getAuditEventTypeFallback() string {
	// Ensures allowlist init has run and fallback is set.
	getAllowedAuditEventTypes()
	return allowedAuditEventTypeFallback
}

// NormalizeAuditEventType ensures event_type is allowed; if not, it coerces to
// system_config_change and records the original value in metadata.
func NormalizeAuditEventType(eventType string, metadata map[string]any) (string, map[string]any) {
	return audit.NormalizeEventType(eventType, metadata, getAllowedAuditEventTypes(), getAuditEventTypeFallback())
}

// CreateAuditEventWithBuilder creates an audit event using instance builder and storageProvider.Create()
// This ensures CAS routing and provides metrics tracking
// storageProvider can be any backend (file, graph, etc.) - backend-agnostic
// If storageProvider is nil, creates one from storageFactory
//
// NOTE: This is the low-level implementation used by coordinator's StorageAuditRouter.
// For cmd/ level code, use coordinator helpers (e.g., emitHashMismatchFixEventViaCoordinator).
// This function is exported for use by the coordinator router and internal storage operations.
func BuildAuditEventInstance(
	ctx context.Context,
	projectRoot string,
	secCtx *pkgctx.SecurityContext,
	options *AuditEventOptions,
	storageProvider ObjectStorageProvider,
) (map[string]any, error) {
	if projectRoot == emptyValue || options == nil {
		return nil, nil
	}
	if storageProvider == nil {
		provider, err := cachedStorageProviders.GetOrCreate(ctx, projectRoot)
		if err != nil {
			return nil, err
		}
		storageProvider = provider
	}

	now := time.Now().UTC()
	auditDir := audit.MonthlyDir(projectRoot, now)
	if err := fileutil.MkdirAll(auditDir, paths.DirPerm755); err != nil {
		return nil, err
	}
	generator := GetAuditIDGenerator(ctx, auditDir, storageProvider)
	auditID, err := generator.GenerateNextID()
	if err != nil {
		return nil, err
	}
	return audit.BuildEventMap(auditID, projectRoot, secCtx, options, now, getAllowedAuditEventTypes(), getAuditEventTypeFallback())
}
