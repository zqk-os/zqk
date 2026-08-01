package scheduler

import (
	"context"
	"errors"
	"fmt"
	"math"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/lanceman/zqk/pkg/concurrency"
	"github.com/lanceman/zqk/pkg/config"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/datacell"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/nildecode"
	"github.com/lanceman/zqk/pkg/objects"
	storagepkg "github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/when"
)

// Default batch size and max batches when not set via job env (BATCH_SIZE, MAX_BATCHES).
// Batch size 5k allows 1k–5k+ deletes per batch with batched stream-deleted writes; 20 batches = 100k per kind per phase.
const (
	defaultRetentionToleranceBatchSize  = 5000
	defaultRetentionToleranceMaxBatches = 20
)

// retentionToleranceUnlimitedMaxBatches is used when MAX_BATCHES / --max-batches is -1 (unlimited batches).
// We avoid math.MaxInt in loops that multiply by batchSize (could overflow); callers should treat this
// sentinel as "no artificial batch cap" while still respecting ctx cancellation and natural termination.
const retentionToleranceUnlimitedMaxBatches = math.MaxInt32

// Bulk delete worker count for retention (BULK_DELETE_WORKERS env); cap to avoid excessive concurrency.
const (
	defaultBulkDeleteWorkers = 20
	maxBulkDeleteWorkers     = 64
)

// ErrObjectOverfill is returned when count exceeds max_count and all candidates are in protected (active-like) status.
// Human intervention is required to resolve (increase max_count, archive/complete objects, or add context).
var ErrObjectOverfill = errfmt.Errorf("object overfill: count exceeds max_count and all objects are in protected (active-like) status; human intervention required to resolve (increase max_count, archive/complete objects, or add context)")

// ProgressFunc is called to emit progress to the user (e.g. stderr). Optional; when set, the handler calls it at phase boundaries so the CLI can show updates instead of appearing to hang.
type ProgressFunc func(msg string)

// RetentionToleranceHandler applies configurable archive and cleanup tolerance per object kind.
// Config is loaded from YAML and merged with bucketing_strategy.retention_tolerance (strategy overrides).
// Strategy does not remove objects in active-like status (protect_statuses); if count cannot be reduced, returns ErrObjectOverfill.
type RetentionToleranceHandler struct {
	storage     storagepkg.ObjectStorageProvider
	projectRoot string
	logger      logging.Logger
	onProgress  ProgressFunc
}

// NewRetentionToleranceHandler creates a new retention tolerance handler.
// NewRetentionToleranceHandler creates a new retention tolerance handler
func NewRetentionToleranceHandler(storage storagepkg.ObjectStorageProvider, projectRoot string) RetentionToleranceHandlerInterface {
	if projectRoot == emptyValue {
		if fileStorage, ok := storage.(*storagepkg.FileObjectStorage); ok {
			projectRoot = fileStorage.GetProjectRoot()
		}
	}
	return &RetentionToleranceHandler{
		storage:     storage,
		projectRoot: projectRoot,
		logger:      logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)),
	}
}

// SetProgressFunc sets an optional callback so the handler can emit progress (e.g. to stderr). Call from CLI to avoid the command appearing to hang.
func (h *RetentionToleranceHandler) SetProgressFunc(fn ProgressFunc) {
	h.onProgress = fn
}

// getBatchConfig returns batch size and max batches from job env (BATCH_SIZE, MAX_BATCHES) or defaults.
func getBatchConfig(job *ScheduledJob) (batchSize, maxBatches int) {
	batchSize = defaultRetentionToleranceBatchSize
	maxBatches = defaultRetentionToleranceMaxBatches
	if scheduledJobEnvMissing(job) {
		return batchSize, maxBatches
	}
	if s, ok := job.EnvironmentVariables[EnvKeyBatchSize]; ok && s != emptyValue {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			batchSize = n
		}
	}
	if s, ok := job.EnvironmentVariables[EnvKeyMaxBatches]; ok && s != emptyValue {
		if n, err := strconv.Atoi(s); err == nil {
			switch {
			case n == -1:
				// Explicit "unlimited batches" (manual reset / catch-up). See OBJECT_COUNT_MANAGEMENT.md.
				maxBatches = retentionToleranceUnlimitedMaxBatches
			case n > 0:
				maxBatches = n
			}
		}
	}
	return batchSize, maxBatches
}

// getBulkDeleteWorkers returns bulk delete worker count from job env (BULK_DELETE_WORKERS) or default; capped at maxBulkDeleteWorkers.
func getBulkDeleteWorkers(job *ScheduledJob) int {
	w := defaultBulkDeleteWorkers
	if scheduledJobEnvMissing(job) {
		return w
	}
	if s, ok := job.EnvironmentVariables[EnvKeyBulkDeleteWorkers]; ok && s != emptyValue {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			w = n
			if w > maxBulkDeleteWorkers {
				w = maxBulkDeleteWorkers
			}
		}
	}
	return w
}

// retentionKindPriority returns a sort key so high-volume kinds are processed first (0 = audit_event, 1 = mcp_session, 2 = *_metric, 3 = other).
func retentionKindPriority(kind string) int {
	switch kind {
	case objects.KindAuditEvent:
		return 0
	case objects.KindMcpSession:
		return 1
	}
	if len(kind) > 7 && kind[len(kind)-7:] == "_metric" {
		return 2
	}
	return 3
}

// getKindFilter extracts optional kind filter from job env KINDS (comma-separated).
// Returns nil if no filter specified (process all configured kinds).
func getKindFilter(job *ScheduledJob) map[string]bool {
	if scheduledJobEnvMissing(job) {
		return nil
	}
	kindsStr, ok := job.EnvironmentVariables[EnvKeyKinds]
	if !ok || kindsStr == emptyValue {
		return nil
	}
	filter := make(map[string]bool)
	for k := range strings.SplitSeq(kindsStr, ",") {
		k = strings.TrimSpace(k)
		if k != emptyValue {
			filter[k] = true
		}
	}
	if len(filter) == 0 {
		return nil
	}
	return filter
}

// Execute loads retention tolerance (YAML + bucketing_strategy spec) and for each kind: archive then cleanup then enforce max_count.
// Only deletes objects whose status is not in protect_statuses (active-like). If over max_count and no deletable candidates, returns ErrObjectOverfill.
// Batch size and max batches are configurable via job env BATCH_SIZE and MAX_BATCHES.
// Kind filter is configurable via job env KINDS (comma-separated list of kinds to process).
func (h *RetentionToleranceHandler) emitProgress(msg string) {
	if h.onProgress != nil {
		h.onProgress(msg)
	}
}

// Execute runs retention tolerance via the pipeline (INGEST → NORMALIZE → FINALIZE).
func (h *RetentionToleranceHandler) Execute(ctx context.Context, job *ScheduledJob) error {
	return RunRetentionToleranceViaPipeline(ctx, h, job)
}

// executeRetentionToleranceCore runs the retention logic. Called from RunRetentionToleranceViaPipeline NORMALIZE stage. Caller must set storagepkg.WithCLIOperation on ctx.
func (h *RetentionToleranceHandler) executeRetentionToleranceCore(ctx context.Context, job *ScheduledJob) error {
	// Progress is committed per batch (archive/cleanup/enforce); timeout does not lose work. BATCH_SIZE/MAX_BATCHES/BULK_DELETE_WORKERS and job max_runtime_seconds control capacity (default 750*120 allows 90k deletes per kind per phase).
	h.emitProgress("Starting retention tolerance (archive and cleanup)...")
	RetentionToleranceLog(h.logger).Info(LogEventRetentionToleranceStarted).
		JobID(job.ID).
		Log()

	batchSize, maxBatches := getBatchConfig(job)
	bulkDeleteWorkers := getBulkDeleteWorkers(job)
	kindFilter := getKindFilter(job)

	h.emitProgress("Loading retention tolerance config...")
	loader := config.NewRetentionToleranceLoader(h.projectRoot)
	cfg, err := loader.Load()
	if err != nil {
		RetentionToleranceLog(h.logger).Error(LogEventRetentionToleranceConfigLoadFailed, err).
			JobID(job.ID).
			String("config_path", loader.GetConfigPath()).
			Log()
		return errfmt.Newf("load retention tolerance config").Wrap(err)
	}

	// Merge strategy-level retention_tolerance from bucketing_strategy objects (strategy overrides YAML for applies_to kinds).
	h.emitProgress("Merging config with bucketing strategies...")
	effective := h.mergeStrategyTolerance(ctx, cfg, job.ID)
	if len(effective) == 0 {
		h.emitProgress("No kinds configured for retention tolerance, skipping.")
		RetentionToleranceLog(h.logger).Info(LogEventRetentionToleranceNoKindsConfiguredSkip).
			JobID(job.ID).
			Log()
		return nil
	}

	// Apply kind filter if specified
	if kindFilter != nil {
		filtered := make(map[string]config.ParsedKindTolerance)
		for kind, tol := range effective {
			if kindFilter[kind] {
				filtered[kind] = tol
			}
		}
		effective = filtered
		if len(effective) == 0 {
			RetentionToleranceLog(h.logger).Info(LogEventRetentionToleranceNoKindsAfterFilterSkip).
				JobID(job.ID).
				Log()
			return nil
		}
	}

	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()

	// Build ordered list of kinds that have work so progress "N/total" is accurate.
	// Process high-volume kinds first (audit_event, mcp_session, *_metric) so timeout still clears the worst backlog.
	var kindsWithWork []string
	for kind, tol := range effective {
		if tol.ArchiveEnabled || tol.CleanupEnabled || tol.MaxCount > 0 {
			kindsWithWork = append(kindsWithWork, kind)
		}
	}
	sort.Slice(kindsWithWork, func(i, j int) bool {
		pi, pj := retentionKindPriority(kindsWithWork[i]), retentionKindPriority(kindsWithWork[j])
		if pi != pj {
			return pi < pj
		}
		return kindsWithWork[i] < kindsWithWork[j]
	})
	var totalArchived, totalDeleted int
	var casEntityStewardshipWork bool
	kindArchivedMap := make(map[string]int)
	kindDeletedMap := make(map[string]int)
	interrupt := concurrency.NewInterruptChecker(concurrency.DefaultInterruptCheckFrequency)
	for i, kind := range kindsWithWork {
		if err := interrupt.Check(ctx); err != nil {
			RetentionToleranceLog(h.logger).Warn(LogEventRetentionToleranceCancelled).
				JobID(job.ID).
				Int("kinds_processed", i).
				WithError(err).
				Log()
			return err
		}
		tol := effective[kind]
		h.emitProgress(fmt.Sprintf("Processing kind %d/%d: %s...", i+1, len(kindsWithWork), kind))

		var kindArchived, kindDeleted int

		// 1) Archive: objects older than archive_after -> status = archived
		if tol.ArchiveEnabled {
			archiveCutoff := time.Now().UTC().Add(-tol.ArchiveAfter)
			archived := h.archiveOldObjects(ctx, secCtx, storageCtx, job.ID, kind, archiveCutoff, batchSize, maxBatches)
			kindArchived = archived
			totalArchived += archived
			if archived > 0 {
				RetentionToleranceLog(h.logger).Info(LogEventRetentionToleranceArchivedByTolerance).
					JobID(job.ID).
					Kind(kind).
					Int("count", archived).
					Log()
			}
		}

		// 2) Cleanup by age: objects older than cleanup_after and not in protect_statuses -> delete
		if tol.CleanupEnabled {
			cleanupCutoff := time.Now().UTC().Add(-tol.CleanupAfter)
			deleted := h.cleanupOldObjects(ctx, secCtx, storageCtx, job.ID, kind, cleanupCutoff, tol.ProtectStatuses, batchSize, maxBatches, bulkDeleteWorkers)
			kindDeleted += deleted
			totalDeleted += deleted
			if deleted > 0 {
				RetentionToleranceLog(h.logger).Info(LogEventRetentionToleranceCleanedUpByTolerance).
					JobID(job.ID).
					Kind(kind).
					Int("count", deleted).
					Log()
			}
		}

		// 3) Enforce max_count: delete oldest (not in protect_statuses) until at or under max_count; if cannot reduce, return blocking error
		if tol.MaxCount > 0 {
			deleted, overfillErr := h.enforceMaxCount(ctx, secCtx, storageCtx, job.ID, kind, tol.MaxCount, tol.ProtectStatuses, batchSize, maxBatches, bulkDeleteWorkers)
			if overfillErr != nil {
				return overfillErr
			}
			kindDeleted += deleted
			totalDeleted += deleted
			if deleted > 0 {
				RetentionToleranceLog(h.logger).Info(LogEventRetentionToleranceKindEnforcedMaxCountDeleted).
					JobID(job.ID).
					Kind(kind).
					Deleted(deleted).
					MaxCount(tol.MaxCount).
					Log()
			}
		}

		kindArchivedMap[kind] = kindArchived
		kindDeletedMap[kind] = kindDeleted

		// Flush CAS index for this kind so deletes/updates are persisted; next run or CLI sees correct counts.
		if h.projectRoot != emptyValue {
			if flushErr := storagepkg.GetListingIndexWriteQueueForProjectRoot(h.projectRoot).FlushKind(kind, 5*time.Second); flushErr != nil {
				RetentionToleranceLog(h.logger).Warn(LogEventRetentionToleranceCASIndexFlushNonFatal).
					WithFields(append(jobLogFieldsByIDAndErr(job.ID, flushErr), logging.String("kind", kind))...).
					Log()
			}
		}

		// Record progress after each kind so on timeout we know exactly what was deleted (no guessing).
		if kindArchived > 0 || kindDeleted > 0 {
			if !storagepkg.StreamStorageEnabledForKind(kind) {
				casEntityStewardshipWork = true
			}
			WriteJobProgress(h.projectRoot, job.ID, map[string]any{
				objects.FieldKeyEventType: "progress",
				KeyJobID:                  job.ID,
				objects.FieldKeyJobType:   JobTypeRetentionTolerance,
				objects.FieldKeyKind:      kind,
				ProgressKeyArchived:       kindArchived,
				ProgressKeyDeleted:        kindDeleted,
			})
		}

		// Explicitly run GC between kinds to prevent memory accumulation in catch-all daily sweeps.
		runtime.GC()
	}

	h.emitProgress("\n--- Retention Influx vs Cleanup Summary (Last 24h) ---")
	yesterday := time.Now().UTC().Add(-24 * time.Hour).Format(time.RFC3339)
	for _, kind := range kindsWithWork {
		deleted := kindDeletedMap[kind]
		archived := kindArchivedMap[kind]
		createdCount, _ := h.storage.Count(ctx, secCtx, storagepkg.ListFilter{
			Kind: kind,
			Filters: map[string]any{
				objects.FieldKeyCreatedAt: map[string]any{
					"$gte": yesterday,
				},
			},
		})
		netInflux := createdCount - deleted
		var trend string
		if netInflux > 0 {
			trend = fmt.Sprintf("📈 LOSING GROUND (influx > cleanup): +%d net objects in system", netInflux)
		} else if netInflux < 0 {
			trend = fmt.Sprintf("📉 GAINING GROUND (cleanup > influx): %d net objects in system", netInflux)
		} else {
			trend = "⚖️ STABLE (influx matches cleanup exactly)"
		}
		summaryMsg := fmt.Sprintf(" - %s: Created=%d, Deleted=%d, Archived=%d | Trend: %s", kind, createdCount, deleted, archived, trend)
		h.emitProgress(summaryMsg)
		RetentionToleranceLog(h.logger).Info(summaryMsg).Log()
	}
	h.emitProgress("------------------------------------------------------\n")

	h.emitProgress("Retention tolerance completed.")
	WriteJobOutcome(h.projectRoot, job.ID, JobTypeRetentionTolerance, map[string]any{
		OutcomeKeyKindsProcessed: len(kindsWithWork),
		OutcomeKeyTotalArchived:  totalArchived,
		OutcomeKeyTotalDeleted:   totalDeleted,
	})
	if h.projectRoot != emptyValue && casEntityStewardshipWork {
		detail := fmt.Sprintf("job_id=%s|archived=%d|deleted=%d", job.ID, totalArchived, totalDeleted)
		if err := datacell.EnqueueStewardMaintenance(ctx, h.projectRoot, datacell.ProfileCASEntity,
			datacell.MaintenanceOp{Name: datacell.MaintenanceOpRetentionSweep, Detail: detail}, h.logger); err != nil {
			SLog(h.logger).Warn("Failed to enqueue steward maintenance after retention sweep").WithError(err).Log()
		}
	}

	if h.projectRoot != emptyValue {
		// Enforce STREAM_KIND_STEWARDSHIP.md contract: stream registries are compacted, and orphaned segments
		// are GC'd, and runtime delta overalys are processed after a successful retention tolerance cycle.
		storagepkg.PostRetentionStreamStewardship(h.projectRoot, job.ID, h.logger)
	}

	return nil
}

// mergeStrategyTolerance builds kind -> ParsedKindTolerance from YAML and then overrides with bucketing_strategy.retention_tolerance for applies_to kinds.
func (h *RetentionToleranceHandler) mergeStrategyTolerance(ctx context.Context, cfg *config.RetentionToleranceConfig, jobID string) map[string]config.ParsedKindTolerance {
	effective := make(map[string]config.ParsedKindTolerance)
	for _, kind := range cfg.EnabledKinds() {
		tol, err := cfg.GetToleranceForKind(kind)
		if err != nil {
			RetentionToleranceLog(h.logger).Warn(LogEventRetentionToleranceSkipKindConfigError).
				WithFields(append(jobLogFieldsByIDAndErr(jobID, err), logging.String("kind", kind))...).
				Log()
			continue
		}
		effective[kind] = tol
	}

	if h.storage == nil {
		return effective
	}
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()
	filter := storagepkg.ListFilter{Kind: objects.KindBucketingStrategy}
	result, err := h.storage.List(ctx, secCtx, storageCtx, filter)
	if err != nil || len(result.Objects) == 0 {
		return effective
	}
	for _, obj := range result.Objects {
		enabled, _ := obj[objects.FieldKeyEnabled].(bool)
		if !enabled {
			continue
		}
		rtAny, ok := obj[objects.FieldKeyRetentionTolerance]
		if !ok {
			continue
		}
		rtAny, ok = nildecode.DecodeNonNilPayload[any](rtAny)
		if !ok {
			continue
		}
		rt, ok := rtAny.(map[string]any)
		if !ok {
			continue
		}
		appliesTo, _ := obj[objects.FieldKeyAppliesTo].([]any)
		if len(appliesTo) == 0 {
			continue
		}
		for _, k := range appliesTo {
			kind, _ := k.(string)
			if kind == emptyValue {
				continue
			}
			tol, err := config.ParseToleranceFromMap(rt, kind)
			if err != nil {
				RetentionToleranceLog(h.logger).Warn(LogEventRetentionToleranceSkipStrategyForKind).
					WithFields(append(jobLogFieldsByIDAndErr(jobID, err), logging.String("kind", kind))...).
					Log()
				continue
			}
			effective[kind] = tol
		}
	}

	// Dynamic unconfigured kind discovery (safe defaults protection)
	mapper := objects.GetGlobalKindMapper()
	if mapper != nil {
		allKinds := mapper.GetAllKinds()
		for _, kind := range allKinds {
			if _, ok := effective[kind]; ok {
				continue
			}
			if isCatalogKind(kind) {
				continue
			}
			count, err := h.storage.Count(ctx, secCtx, storagepkg.ListFilter{Kind: kind})
			if err == nil && count > 0 {
				tol, err := cfg.GetToleranceForKind(kind)
				if err == nil {
					effective[kind] = tol
					msg := fmt.Sprintf("⚠️ Safe default retention tolerance automatically applied to unconfigured kind %q (%d objects found). Configure this kind explicitly in retention_tolerance.yaml to override.", kind, count)
					h.emitProgress(msg)
					RetentionToleranceLog(h.logger).Warn(msg).Log()
				}
			}
		}
	}

	return effective
}

func isCatalogKind(kind string) bool {
	switch kind {
	case "roadmap", "goal", "requirement", "milestone", "policy", "rule", "template", "persona", "role",
		"object_spec", "doc_entry", "capability", "important_date", "vocabulary_scheme", "glossary_term",
		"glossary_term_relation", "brand", "brand_asset", "organization", "division", "department", "team",
		"team_configuration", "keystore_entry", "auth_strategy", "account", "infrastructure_adapter",
		"prompt_template", "agent_skill", "watchdog_registration", "lifecycle", "resolver", "namespace",
		"namespace_registry", "provider_profile", "scenario", "strategic_context", "visual_plan",
		"field_registry", "extensible_object", "command_spec", "partnership", "corporate_initiative",
		"organizational_change", "job_listing", "job_search_profile",
		"stakeholder_profile", "library", "application_record", "workflow", "vitality_report", "maturation_report",
		"risk_blocker", "capacity_advertisement", "compute_advertisement", "metrics_exchange_contract", "fission_event",
		"partner_profile", "provider_profile_relation",
		"stakeholder_profile_relation":
		return true
	default:
		return false
	}
}

// archiveOldObjects lists objects of kind with created_at < cutoff and status != archived, then bulk-updates status to objects.ObjectStatusArchived.
// Excludes already-archived objects from the query to avoid redundant WAL entries.
func (h *RetentionToleranceHandler) archiveOldObjects(
	ctx context.Context,
	secCtx *pkgctx.SecurityContext,
	storageCtx *pkgctx.StorageContext,
	jobID, kind string,
	cutoff time.Time,
	batchSize, maxBatches int,
) int {
	if ctx.Err() != nil {
		return 0
	}
	if batchSize <= 0 {
		batchSize = defaultRetentionToleranceBatchSize
	}
	switch {
	case maxBatches == -1:
		maxBatches = retentionToleranceUnlimitedMaxBatches
	case maxBatches <= 0:
		maxBatches = defaultRetentionToleranceMaxBatches
	}

	cutoffStr := cutoff.Format(time.RFC3339)
	filters := map[string]any{
		objects.FieldKeyCreatedAt: map[string]any{"$lt": cutoffStr},
		objects.FieldKeyStatus:    map[string]any{"$ne": objects.ObjectStatusArchived}, // Exclude already-archived
	}

	var allIDs []string
	if fileStorage, ok := h.storage.(*storagepkg.FileObjectStorage); ok {
		if cas, casErr := fileStorage.GetContentAddressableStorage(kind); casErr == nil && cas != nil {
			if idx := cas.GetIndex(); idx != nil {
				limit := batchSize * maxBatches
				if maxBatches == retentionToleranceUnlimitedMaxBatches {
					limit = 100000
				} else {
					limit *= 2
				}
				allIDs = idx.OldestIDs(limit)
			}
		}
	}

	var totalArchived int
	interrupt := concurrency.NewInterruptChecker(concurrency.DefaultInterruptCheckFrequency)

	if len(allIDs) > 0 {
		for i := 0; i < len(allIDs); i += batchSize {
			if err := interrupt.Check(ctx); err != nil {
				break
			}
			if maxBatches != retentionToleranceUnlimitedMaxBatches && (i/batchSize) >= maxBatches {
				break
			}

			end := i + batchSize
			if end > len(allIDs) {
				end = len(allIDs)
			}
			chunk := allIDs[i:end]

			h.emitProgress(fmt.Sprintf("Archive %s: batch %d/%d (using ID pagination)...", kind, (i/batchSize)+1, (len(allIDs)+batchSize-1)/batchSize))

			chunkFilters := map[string]any{
				objects.FieldKeyID:        map[string]any{"$in": chunk},
				objects.FieldKeyCreatedAt: map[string]any{"$lt": cutoffStr},
				objects.FieldKeyStatus:    map[string]any{"$ne": objects.ObjectStatusArchived},
			}

			filter := storagepkg.ListFilter{
				Kind:    kind,
				Filters: chunkFilters,
				Limit:   batchSize,
			}
			result, err := h.storage.List(ctx, secCtx, storageCtx, filter)
			if err != nil || len(result.Objects) == 0 {
				// If no objects matched in this chunk (e.g. all newer than cutoff), we continue.
				// Since we iterate oldest to newest, if none match, we could theoretically break early,
				// but to be safe against unordered created_at strings, we just continue.
				continue
			}

			var toUpdate []storagepkg.BulkUpdateItem
			for _, obj := range result.Objects {
				id, _ := obj[objects.FieldKeyID].(string)
				if id == emptyValue {
					continue
				}
				status, _ := obj[objects.FieldKeyStatus].(string)
				if status == objects.ObjectStatusArchived {
					continue
				}
				if kind == objects.KindSchedulerJob {
					execMode, _ := obj[objects.FieldKeyExecutionMode].(string)
					if execMode == "reusable" {
						continue
					}
				}
				toUpdate = append(toUpdate, storagepkg.BulkUpdateItem{
					ID:      id,
					Updates: map[string]any{objects.FieldKeyStatus: objects.ObjectStatusArchived},
				})
			}

			if len(toUpdate) == 0 {
				continue
			}

			bulkResult, err := h.storage.BulkUpdate(ctx, secCtx, toUpdate)
			if err != nil {
				RetentionToleranceLog(h.logger).Warn(LogEventRetentionToleranceBulkUpdateArchiveFailed).
					WithFields(append(jobLogFieldsByIDAndErr(jobID, err), logging.String("kind", kind))...).
					Log()
				break
			}

			totalArchived += bulkResult.SuccessCount
			storagepkg.InvalidateListCacheForKind(kind)
		}
		return totalArchived
	}

	for batch := 0; batch < maxBatches; batch++ {
		if err := interrupt.Check(ctx); err != nil {
			break
		}
		if maxBatches == retentionToleranceUnlimitedMaxBatches {
			h.emitProgress(fmt.Sprintf("Archive %s: batch %d (unlimited cap)...", kind, batch+1))
		} else {
			h.emitProgress(fmt.Sprintf("Archive %s: batch %d/%d...", kind, batch+1, maxBatches))
		}

		filter := storagepkg.ListFilter{
			Kind:    kind,
			Filters: filters,
			Limit:   batchSize,
		}
		result, err := h.storage.List(ctx, secCtx, storageCtx, filter)
		if err != nil || len(result.Objects) == 0 {
			break
		}

		var toUpdate []storagepkg.BulkUpdateItem
		for _, obj := range result.Objects {
			id, _ := obj[objects.FieldKeyID].(string)
			if id == emptyValue {
				continue
			}
			status, _ := obj[objects.FieldKeyStatus].(string)
			if status == objects.ObjectStatusArchived {
				continue
			}
			if kind == objects.KindSchedulerJob {
				execMode, _ := obj[objects.FieldKeyExecutionMode].(string)
				if execMode == "reusable" {
					continue
				}
			}
			toUpdate = append(toUpdate, storagepkg.BulkUpdateItem{
				ID:      id,
				Updates: map[string]any{objects.FieldKeyStatus: objects.ObjectStatusArchived},
			})
		}

		if len(toUpdate) == 0 {
			break
		}

		bulkResult, err := h.storage.BulkUpdate(ctx, secCtx, toUpdate)
		if err != nil {
			RetentionToleranceLog(h.logger).Warn(LogEventRetentionToleranceBulkUpdateArchiveFailed).
				WithFields(append(jobLogFieldsByIDAndErr(jobID, err), logging.String("kind", kind))...).
				Log()
			break
		}

		n := bulkResult.SuccessCount
		totalArchived += n
		storagepkg.InvalidateListCacheForKind(kind)

		if h.projectRoot != emptyValue {
			if queue := storagepkg.GetListingIndexWriteQueueForProjectRoot(h.projectRoot); queue != nil {
				_ = queue.FlushKind(kind, 5*time.Second)
			}
		}

		if n < len(result.Objects) {
			break
		}
	}
	return totalArchived
}

// cleanupOldObjects lists objects of kind with created_at < cutoff and status not in protect_statuses, then deletes in batches.
// Strategy does not remove objects in active-like status.
func (h *RetentionToleranceHandler) cleanupOldObjects(
	ctx context.Context,
	secCtx *pkgctx.SecurityContext,
	storageCtx *pkgctx.StorageContext,
	jobID, kind string,
	cutoff time.Time,
	protectStatuses []string,
	batchSize, maxBatches, bulkDeleteWorkers int,
) int {
	if batchSize <= 0 {
		batchSize = defaultRetentionToleranceBatchSize
	}
	switch {
	case maxBatches == -1:
		maxBatches = retentionToleranceUnlimitedMaxBatches
	case maxBatches <= 0:
		maxBatches = defaultRetentionToleranceMaxBatches
	}
	cutoffStr := cutoff.Format(time.RFC3339)
	// Fast path: high volume cache avoids full List() and Stream Registry parsing overheads.
	if storagepkg.IsHighVolumeKindForCache(kind) {
		if cache := storagepkg.GetGlobalHighVolumeEventCache(); cache != nil && cache.IsPopulatedForProject(h.projectRoot) {
			toDelete := batchSize * maxBatches
			if maxBatches == retentionToleranceUnlimitedMaxBatches {
				toDelete = cache.CountByKind(kind) // or some large number
			}

			// If we have protect statuses, request more IDs from the cache so we still have enough after filtering
			queryCount := toDelete
			if len(protectStatuses) > 0 {
				queryCount = toDelete * 5
				if queryCount > cache.CountByKind(kind) {
					queryCount = cache.CountByKind(kind)
				}
			}

			idsToDelete := cache.QueryOlderThan(cutoff, queryCount)
			if len(idsToDelete) > 0 {
				// Filter by protect statuses
				if len(protectStatuses) > 0 {
					var filteredIDs []string
					for _, id := range idsToDelete {
						obj, err := h.storage.Read(ctx, secCtx, id)
						if err == nil {
							status, _ := obj[objects.FieldKeyStatus].(string)
							protected := false
							for _, p := range protectStatuses {
								if status == p {
									protected = true
									break
								}
							}
							if !protected {
								filteredIDs = append(filteredIDs, id)
								if len(filteredIDs) >= toDelete {
									break
								}
							}
						} else if errors.Is(err, storagepkg.ErrObjectNotFound) || strings.Contains(err.Error(), "not found") {
							// If it's not found, it's already deleted or phantom, so we can "delete" it to clear it
							filteredIDs = append(filteredIDs, id)
							if len(filteredIDs) >= toDelete {
								break
							}
						}
					}
					idsToDelete = filteredIDs
				}

				if len(idsToDelete) > 0 {
					RetentionToleranceLog(h.logger).Info(LogEventRetentionToleranceCleanedUpByTolerance).
						JobID(jobID).
						Kind(kind).
						CandidateCount(len(idsToDelete)).
						Log()

					var totalDeleted int
					interrupt := concurrency.NewInterruptChecker(concurrency.DefaultInterruptCheckFrequency)
					for i := 0; i < len(idsToDelete) && totalDeleted < len(idsToDelete); i += batchSize {
						if err := interrupt.Check(ctx); err != nil {
							break
						}
						end := i + batchSize
						if end > len(idsToDelete) {
							end = len(idsToDelete)
						}
						batch := idsToDelete[i:end]

						h.emitProgress(fmt.Sprintf("Cleanup (fast-path) %s: batch %d/%d...", kind, (i/batchSize)+1, (len(idsToDelete)+batchSize-1)/batchSize))

						var n int
						when.When(func() bool { _, ok := h.storage.(*storagepkg.FileObjectStorage); return ok }).Then(func() {
							fileStorage := h.storage.(*storagepkg.FileObjectStorage)
							// CRITICAL: force=true bypasses f.Get(), which completely bypasses loadStreamRegistrySnapshot() memory OOMs!
							optRes, delErr := fileStorage.BulkDeleteOptimized(ctx, secCtx, batch, true, bulkDeleteWorkers)
							if delErr == nil {
								n = optRes.SuccessCount
							}
						}).OrElse(func() {
							bulkRes, delErr := h.storage.BulkDelete(ctx, secCtx, batch, true)
							if delErr == nil {
								n = bulkRes.SuccessCount
							}
						}).Run()

						totalDeleted += n
						storagepkg.InvalidateListCacheForKind(kind)
					}
					if totalDeleted > 0 {
						cache.InvalidateForProject(h.projectRoot)
					}
					return totalDeleted
				}

				// If the cache was populated but yielded 0 items to delete (either because none were older than cutoff,
				// or all were protected statuses), DO NOT fall back to the slow path. For high volume stream kinds,
				// the slow path will OOM. We trust the cache.
				return 0
			}

			// If cache is populated but QueryOlderThan returned 0 items, DO NOT fall back.
			return 0
		}
	}

	return h.cleanupOldObjectsSlowPath(ctx, secCtx, storageCtx, jobID, kind, protectStatuses, batchSize, maxBatches, bulkDeleteWorkers, cutoffStr)
}

func (h *RetentionToleranceHandler) cleanupOldObjectsSlowPath(
	ctx context.Context,
	secCtx *pkgctx.SecurityContext,
	storageCtx *pkgctx.StorageContext,
	jobID, kind string,
	protectStatuses []string,
	batchSize, maxBatches, bulkDeleteWorkers int,
	cutoffStr string,
) int {
	filters := map[string]any{
		objects.FieldKeyCreatedAt: map[string]any{"$lt": cutoffStr},
	}
	if len(protectStatuses) > 0 {
		filters[objects.FieldKeyStatus] = map[string]any{"$nin": protectStatuses}
	}
	var totalDeleted int
	interrupt := concurrency.NewInterruptChecker(concurrency.DefaultInterruptCheckFrequency)

	var allIDs []string
	if fileStorage, ok := h.storage.(*storagepkg.FileObjectStorage); ok {
		if cas, casErr := fileStorage.GetContentAddressableStorage(kind); casErr == nil && cas != nil {
			if idx := cas.GetIndex(); idx != nil {
				limit := batchSize * maxBatches
				if maxBatches == retentionToleranceUnlimitedMaxBatches {
					limit = 100000
				} else {
					limit *= 2
				}
				allIDs = idx.OldestIDs(limit)
			}
		}
	}

	if len(allIDs) > 0 {
		for i := 0; i < len(allIDs); i += batchSize {
			if err := interrupt.Check(ctx); err != nil {
				break
			}
			if maxBatches != retentionToleranceUnlimitedMaxBatches && (i/batchSize) >= maxBatches {
				break
			}

			end := i + batchSize
			if end > len(allIDs) {
				end = len(allIDs)
			}
			chunk := allIDs[i:end]

			h.emitProgress(fmt.Sprintf("Cleanup %s: batch %d/%d (using ID pagination)...", kind, (i/batchSize)+1, (len(allIDs)+batchSize-1)/batchSize))

			chunkFilters := map[string]any{
				objects.FieldKeyID:        map[string]any{"$in": chunk},
				objects.FieldKeyCreatedAt: map[string]any{"$lt": cutoffStr},
			}
			if len(protectStatuses) > 0 {
				chunkFilters[objects.FieldKeyStatus] = map[string]any{"$nin": protectStatuses}
			}

			filter := storagepkg.ListFilter{
				Kind:    kind,
				Filters: chunkFilters,
				Limit:   batchSize,
			}
			result, err := h.storage.List(ctx, secCtx, storageCtx, filter)
			if err != nil || len(result.Objects) == 0 {
				continue
			}

			ids := make([]string, 0, len(result.Objects))
			for _, obj := range result.Objects {
				if id, ok := obj[objects.FieldKeyID].(string); ok && id != emptyValue {
					ids = append(ids, id)
				}
			}

			if len(ids) == 0 {
				continue
			}

			var n int
			if fileStorage, ok := h.storage.(*storagepkg.FileObjectStorage); ok {
				optRes, delErr := fileStorage.BulkDeleteOptimized(ctx, secCtx, ids, false, bulkDeleteWorkers)
				if delErr != nil {
					RetentionToleranceLog(h.logger).Warn(LogEventRetentionToleranceBulkDeleteCleanupByAgeFailed).
						WithFields(append(jobLogFieldsByIDAndErr(jobID, delErr), logging.String("kind", kind))...).
						Log()
					break
				}
				n = optRes.SuccessCount
			} else {
				bulkRes, delErr := h.storage.BulkDelete(ctx, secCtx, ids, false)
				if delErr != nil {
					RetentionToleranceLog(h.logger).Warn(LogEventRetentionToleranceBulkDeleteCleanupByAgeFailed).
						WithFields(append(jobLogFieldsByIDAndErr(jobID, delErr), logging.String("kind", kind))...).
						Log()
					break
				}
				n = bulkRes.SuccessCount
			}
			totalDeleted += n
			storagepkg.InvalidateListCacheForKind(kind)
		}

		if totalDeleted > 0 && isHighVolumeKind(kind) {
			if cache := storagepkg.GetGlobalHighVolumeEventCache(); cache != nil {
				cache.InvalidateForProject(h.projectRoot)
			}
		}
		return totalDeleted
	}

	for batch := 0; batch < maxBatches; batch++ {
		if err := interrupt.Check(ctx); err != nil {
			break
		}
		if maxBatches == retentionToleranceUnlimitedMaxBatches {
			h.emitProgress(fmt.Sprintf("Cleanup %s: batch %d (unlimited cap)...", kind, batch+1))
		} else {
			h.emitProgress(fmt.Sprintf("Cleanup %s: batch %d/%d...", kind, batch+1, maxBatches))
		}
		filter := storagepkg.ListFilter{
			Kind:    kind,
			Filters: filters,
			Limit:   batchSize,
		}
		result, err := h.storage.List(ctx, secCtx, storageCtx, filter)
		if err != nil || len(result.Objects) == 0 {
			break
		}
		ids := make([]string, 0, len(result.Objects))
		for _, obj := range result.Objects {
			if id, ok := obj[objects.FieldKeyID].(string); ok && id != emptyValue {
				ids = append(ids, id)
			}
		}
		if len(ids) == 0 {
			break
		}
		var n int
		when.When(func() bool { _, ok := h.storage.(*storagepkg.FileObjectStorage); return ok }).Then(func() {
			fileStorage := h.storage.(*storagepkg.FileObjectStorage)
			optRes, delErr := fileStorage.BulkDeleteOptimized(ctx, secCtx, ids, false, bulkDeleteWorkers)
			err = delErr
			if delErr == nil {
				n = optRes.SuccessCount
			}
		}).OrElse(func() {
			bulkRes, delErr := h.storage.BulkDelete(ctx, secCtx, ids, false)
			err = delErr
			if delErr == nil {
				n = bulkRes.SuccessCount
			}
		}).Run()
		if err != nil {
			RetentionToleranceLog(h.logger).Warn(LogEventRetentionToleranceBulkDeleteCleanupByAgeFailed).
				WithFields(append(jobLogFieldsByIDAndErr(jobID, err), logging.String("kind", kind))...).
				Log()
			break
		}
		totalDeleted += n
		storagepkg.InvalidateListCacheForKind(kind)

		if h.projectRoot != emptyValue {
			if queue := storagepkg.GetListingIndexWriteQueueForProjectRoot(h.projectRoot); queue != nil {
				_ = queue.FlushKind(kind, 5*time.Second)
			}
		}

		if n < len(ids) {
			break
		}
	}
	if totalDeleted > 0 && isHighVolumeKind(kind) {
		if cache := storagepkg.GetGlobalHighVolumeEventCache(); cache != nil {
			cache.InvalidateForProject(h.projectRoot)
		}
	}
	return totalDeleted
}

// isHighVolumeKind returns true for kinds that use the high-volume event cache (audit_event, *_metric, change_journal_entry, mcp_session, scheduler_job, zqk_session). See HIGH_VOLUME_EVENT_INDEXES.md, high_volume_kinds.yaml.
func isHighVolumeKind(kind string) bool {
	return kind == objects.KindAuditEvent || kind == objects.KindChangeJournalEntry || kind == objects.KindMcpSession ||
		kind == objects.KindSchedulerJob || kind == objects.KindZqkSession ||
		(len(kind) > 7 && kind[len(kind)-7:] == "_metric")
}

// enforceMaxCountViaHVNoProtect uses CAS OldestIDs or high-volume cache when protect_statuses is empty.
// Returns handled=true when this path completed enforcement (caller should return totalDeleted, nil).
func (h *RetentionToleranceHandler) enforceMaxCountViaHVNoProtect(
	ctx context.Context,
	secCtx *pkgctx.SecurityContext,
	jobID, kind string,
	toDelete, batchSize int,
	bulkDeleteWorkers, maxCount int,
) (totalDeleted int, handled bool) {
	var idsToDelete []string
	if fileStorage, ok := h.storage.(*storagepkg.FileObjectStorage); ok {
		if cas, casErr := fileStorage.GetContentAddressableStorage(kind); casErr == nil && cas != nil {
			idx := cas.GetIndex()
			if idx != nil {
				idsToDelete = idx.OldestIDs(toDelete)
				if len(idsToDelete) > 0 {
					RetentionToleranceLog(h.logger).Info(LogEventRetentionToleranceUsingCASOldestIDsForMaxCount).
						JobID(jobID).
						Kind(kind).
						CandidateCount(len(idsToDelete)).
						Log()
				}
			}
		}
	}
	if len(idsToDelete) == 0 {
		if cache := storagepkg.GetGlobalHighVolumeEventCache(); cache != nil && cache.IsPopulatedForProject(h.projectRoot) {
			idsToDelete = cache.QueryOldestByKind(kind, toDelete)
			if len(idsToDelete) > 0 {
				RetentionToleranceLog(h.logger).Info(LogEventRetentionToleranceUsingHVCacheForMaxCount).
					JobID(jobID).
					Kind(kind).
					CandidateCount(len(idsToDelete)).
					Log()
			}
		}
	}
	if len(idsToDelete) == 0 {
		return 0, false
	}
	interrupt := concurrency.NewInterruptChecker(concurrency.DefaultInterruptCheckFrequency)
	for i := 0; i < len(idsToDelete) && totalDeleted < toDelete; i += batchSize {
		if err := interrupt.Check(ctx); err != nil {
			break
		}
		end := i + batchSize
		if end > len(idsToDelete) {
			end = len(idsToDelete)
		}
		if totalDeleted+(end-i) > toDelete {
			end = i + (toDelete - totalDeleted)
		}
		batch := idsToDelete[i:end]
		if len(batch) == 0 {
			break
		}
		h.emitProgress(fmt.Sprintf("Enforcing max_count %s: batch %d/%d (deleted: %d/%d)...", kind, (i/batchSize)+1, (len(idsToDelete)+batchSize-1)/batchSize, totalDeleted, toDelete))
		var n int
		if fileStorage, ok := h.storage.(*storagepkg.FileObjectStorage); ok {
			optRes, delErr := fileStorage.BulkDeleteOptimized(ctx, secCtx, batch, true, bulkDeleteWorkers)
			if delErr != nil {
				RetentionToleranceLog(h.logger).Warn(LogEventRetentionToleranceBulkDeleteMaxCountPathFailed).
					WithFields(append(jobLogFieldsByIDAndErr(jobID, delErr), logging.String("kind", kind))...).
					Log()
				break
			}
			n = optRes.SuccessCount
		} else {
			bulkRes, delErr := h.storage.BulkDelete(ctx, secCtx, batch, true)
			if delErr != nil {
				RetentionToleranceLog(h.logger).Warn(LogEventRetentionToleranceBulkDeleteMaxCountPathFailed).
					WithFields(append(jobLogFieldsByIDAndErr(jobID, delErr), logging.String("kind", kind))...).
					Log()
				break
			}
			n = bulkRes.SuccessCount
		}
		totalDeleted += n
		storagepkg.InvalidateListCacheForKind(kind)
		if n == 0 && totalDeleted == 0 {
			break
		}
	}
	if totalDeleted > 0 {
		if cache := storagepkg.GetGlobalHighVolumeEventCache(); cache != nil {
			cache.InvalidateForProject(h.projectRoot)
		}
		RetentionToleranceLog(h.logger).Info(LogEventRetentionToleranceEnforcedMaxCountDeletedOldestObjects).
			JobID(jobID).
			Kind(kind).
			Deleted(totalDeleted).
			MaxCount(maxCount).
			Log()
		return totalDeleted, true
	}
	return 0, false
}

// enforceMaxCountViaHVWithProtect uses the high-volume cache excluding protect_statuses when populated.
// Returns handled=true when this path completed enforcement.
func (h *RetentionToleranceHandler) enforceMaxCountViaHVWithProtect(
	ctx context.Context,
	secCtx *pkgctx.SecurityContext,
	jobID, kind string,
	toDelete, batchSize int,
	bulkDeleteWorkers, maxCount int,
	protectStatuses []string,
) (totalDeleted int, handled bool) {
	cache := storagepkg.GetGlobalHighVolumeEventCache()
	if cache == nil || !cache.IsPopulatedForProject(h.projectRoot) || !cache.HasStatus() {
		return 0, false
	}
	idsToDelete := cache.QueryOldestByKindExcludingStatus(kind, toDelete, protectStatuses)
	if len(idsToDelete) == 0 {
		return 0, false
	}
	RetentionToleranceLog(h.logger).Info(LogEventRetentionToleranceUsingHVCacheExcludeProtectMaxCount).
		JobID(jobID).
		Kind(kind).
		CandidateCount(len(idsToDelete)).
		Log()
	interrupt := concurrency.NewInterruptChecker(concurrency.DefaultInterruptCheckFrequency)
	for i := 0; i < len(idsToDelete) && totalDeleted < toDelete; i += batchSize {
		if err := interrupt.Check(ctx); err != nil {
			break
		}
		end := i + batchSize
		if end > len(idsToDelete) {
			end = len(idsToDelete)
		}
		if totalDeleted+(end-i) > toDelete {
			end = i + (toDelete - totalDeleted)
		}
		batch := idsToDelete[i:end]
		if len(batch) == 0 {
			break
		}
		h.emitProgress(fmt.Sprintf("Enforcing max_count %s: batch %d/%d (deleted: %d/%d)...", kind, (i/batchSize)+1, (len(idsToDelete)+batchSize-1)/batchSize, totalDeleted, toDelete))
		var n int
		if fileStorage, ok := h.storage.(*storagepkg.FileObjectStorage); ok {
			optRes, delErr := fileStorage.BulkDeleteOptimized(ctx, secCtx, batch, false, bulkDeleteWorkers)
			if delErr != nil {
				RetentionToleranceLog(h.logger).Warn(LogEventRetentionToleranceBulkDeleteMaxCountCachePathFailed).
					WithFields(append(jobLogFieldsByIDAndErr(jobID, delErr), logging.String("kind", kind))...).
					Log()
				break
			}
			n = optRes.SuccessCount
		} else {
			bulkRes, delErr := h.storage.BulkDelete(ctx, secCtx, batch, false)
			if delErr != nil {
				RetentionToleranceLog(h.logger).Warn(LogEventRetentionToleranceBulkDeleteMaxCountCachePathFailed).
					WithFields(append(jobLogFieldsByIDAndErr(jobID, delErr), logging.String("kind", kind))...).
					Log()
				break
			}
			n = bulkRes.SuccessCount
		}
		totalDeleted += n
		storagepkg.InvalidateListCacheForKind(kind)
		if n == 0 && totalDeleted == 0 {
			break
		}
	}
	if totalDeleted > 0 {
		if cache := storagepkg.GetGlobalHighVolumeEventCache(); cache != nil {
			cache.InvalidateForProject(h.projectRoot)
		}
		RetentionToleranceLog(h.logger).Info(LogEventRetentionToleranceEnforcedMaxCountDeletedOldestCacheExcludeProtect).
			JobID(jobID).
			Kind(kind).
			Deleted(totalDeleted).
			MaxCount(maxCount).
			Log()
		return totalDeleted, true
	}
	return 0, false
}

// enforceMaxCount deletes oldest objects (not in protect_statuses) when count > max_count.
// Processes in batches to avoid timeouts and memory issues when there are many objects to delete.
// If count > max_count and no deletable candidates (all in active-like status), returns ErrObjectOverfill so a human must resolve.
func (h *RetentionToleranceHandler) enforceMaxCount(
	ctx context.Context,
	secCtx *pkgctx.SecurityContext,
	storageCtx *pkgctx.StorageContext,
	jobID, kind string,
	maxCount int,
	protectStatuses []string,
	batchSize, maxBatches, bulkDeleteWorkers int,
) (int, error) {
	if batchSize <= 0 {
		batchSize = defaultRetentionToleranceBatchSize
	}
	switch {
	case maxBatches == -1:
		maxBatches = retentionToleranceUnlimitedMaxBatches
	case maxBatches <= 0:
		maxBatches = defaultRetentionToleranceMaxBatches
	}
	h.emitProgress(fmt.Sprintf("Enforcing max_count for %s (max=%d)...", kind, maxCount))
	var count int
	var err error
	if isHighVolumeKind(kind) && h.projectRoot != emptyValue {
		if cache := storagepkg.GetGlobalHighVolumeEventCache(); cache != nil && cache.IsPopulatedForProject(h.projectRoot) {
			count = cache.CountByKind(kind)
		} else {
			filter := storagepkg.ListFilter{Kind: kind}
			count, err = h.storage.Count(ctx, secCtx, filter)
		}
	} else {
		filter := storagepkg.ListFilter{Kind: kind}
		count, err = h.storage.Count(ctx, secCtx, filter)
	}
	// If Count() fails (e.g., timeout), try to get count from CAS index as fallback
	var allIDs []string
	if err != nil {
		RetentionToleranceLog(h.logger).Warn(LogEventRetentionToleranceCountFailedCASFallback).
			JobID(jobID).
			Kind(kind).
			WithError(err).
			Log()
		// Try to get count from CAS index directly
		if fileStorage, ok := h.storage.(*storagepkg.FileObjectStorage); ok {
			if cas, casErr := fileStorage.GetContentAddressableStorage(kind); casErr == nil && cas != nil {
				if ids, listErr := cas.ListIDs(); listErr == nil {
					count = len(ids)
					allIDs = ids
					err = nil // Clear error so we can proceed
					RetentionToleranceLog(h.logger).Info(LogEventRetentionToleranceUsingCASIndexCountFallback).
						JobID(jobID).
						Kind(kind).
						Count(count).
						Log()
				}
			}
		}
	}
	if err != nil || count <= maxCount {
		return 0, nil
	}
	toDelete := count - maxCount
	if toDelete <= 0 {
		return 0, nil
	}
	RetentionToleranceLog(h.logger).Info(LogEventRetentionToleranceEnforcingMaxCountExceeds).
		JobID(jobID).
		Kind(kind).
		Count(count).
		MaxCount(maxCount).
		ToDelete(toDelete).
		BatchSize(batchSize).
		MaxBatches(maxBatches).
		Log()

	// Ensure List sees current data (avoid stale empty result from list cache)
	storagepkg.InvalidateListCacheForKind(kind)

	// Cap toDelete so we don't exceed batch capacity this run (remaining in subsequent runs).
	// When maxBatches is "unlimited" (-1), do not artificially cap deletes in a single run.
	if maxBatches != retentionToleranceUnlimitedMaxBatches {
		maxToProcess := batchSize * maxBatches
		if toDelete > maxToProcess {
			RetentionToleranceLog(h.logger).Info(LogEventRetentionToleranceLimitingMaxCountToBatchLimit).
				JobID(jobID).
				Kind(kind).
				ToDelete(toDelete).
				Int("max_to_process", maxToProcess).
				Log()
			toDelete = maxToProcess
		}
	}

	// CRITICAL: For high-volume kinds with no protect_statuses, get oldest IDs from index or cache (HIGH_VOLUME_EVENT_INDEXES.md).
	var totalDeleted int
	if isHighVolumeKind(kind) && len(protectStatuses) == 0 && h.projectRoot != emptyValue {
		td, done := h.enforceMaxCountViaHVNoProtect(ctx, secCtx, jobID, kind, toDelete, batchSize, bulkDeleteWorkers, maxCount)
		if done {
			return td, nil
		}
	}

	// When protect_statuses is set, avoid full List (85k reads per batch): use high-volume cache if built with status. See RETENTION_MAX_COUNT_PERFORMANCE.md.
	if isHighVolumeKind(kind) && len(protectStatuses) > 0 && h.projectRoot != emptyValue {
		td, done := h.enforceMaxCountViaHVWithProtect(ctx, secCtx, jobID, kind, toDelete, batchSize, bulkDeleteWorkers, maxCount, protectStatuses)
		if done {
			return td, nil
		}
	}

	// OPTIMIZATION: Use CAS index directly instead of List() with SortBy (which reads all files)
	// Get all IDs from CAS index if we don't have them already
	if len(allIDs) == 0 {
		if fileStorage, ok := h.storage.(*storagepkg.FileObjectStorage); ok {
			if cas, casErr := fileStorage.GetContentAddressableStorage(kind); casErr == nil && cas != nil {
				if idx := cas.GetIndex(); idx != nil {
					limit := toDelete
					maxToProcess := batchSize * maxBatches
					if maxBatches != retentionToleranceUnlimitedMaxBatches && limit > maxToProcess {
						limit = maxToProcess
					} else if limit > 100000 {
						limit = 100000
					}
					if len(protectStatuses) > 0 {
						limit *= 5
					}
					allIDs = idx.OldestIDs(limit)
				}
			}
		}
		// Fallback to List() if CAS index unavailable (should be rare)
		if len(allIDs) == 0 {
			RetentionToleranceLog(h.logger).Warn(LogEventRetentionToleranceCASUnavailableListFallback).
				JobID(jobID).
				Kind(kind).
				Log()
			// Continue with old List() approach below
		}
	}

	// Process in batches to avoid timeouts and memory issues
	filters := map[string]any{}
	if len(protectStatuses) > 0 {
		filters[objects.FieldKeyStatus] = map[string]any{"$nin": protectStatuses}
	}

	// Fast path: Use CAS index IDs directly when no protect_statuses (delete first N without loading objects).
	// When protect_statuses is set, use batched List (filter status $nin) instead of Read() per object to avoid 1000s of Read calls.
	if len(allIDs) > 0 && len(protectStatuses) == 0 {
		RetentionToleranceLog(h.logger).Info(LogEventRetentionToleranceFastPathCASDelete).
			JobID(jobID).
			Kind(kind).
			Int("total_ids", len(allIDs)).
			Int("to_delete", toDelete).
			Log()

		interrupt := concurrency.NewInterruptChecker(concurrency.DefaultInterruptCheckFrequency)
		for i := 0; i < len(allIDs) && totalDeleted < toDelete; i += batchSize {
			if err := interrupt.Check(ctx); err != nil {
				RetentionToleranceLog(h.logger).Warn(LogEventRetentionToleranceMaxCountFastPathCancelled).
					JobID(jobID).
					Kind(kind).
					Int("total_deleted", totalDeleted).
					WithError(err).
					Log()
				break
			}
			batch := allIDs[i:]
			if len(batch) > batchSize {
				batch = batch[:batchSize]
			}
			if totalDeleted+len(batch) > toDelete {
				batch = batch[:toDelete-totalDeleted]
			}
			if len(batch) == 0 {
				break
			}

			h.emitProgress(fmt.Sprintf("Enforcing max_count %s: batch %d (deleting %d objects)...", kind, (i/batchSize)+1, len(batch)))
			var n int
			if fileStorage, ok := h.storage.(*storagepkg.FileObjectStorage); ok {
				optRes, delErr := fileStorage.BulkDeleteOptimized(ctx, secCtx, batch, false, bulkDeleteWorkers)
				if delErr != nil {
					RetentionToleranceLog(h.logger).Warn(LogEventRetentionToleranceBulkDeleteEnforceMaxCountFailed).
						WithFields(append(jobLogFieldsByIDAndErr(jobID, delErr), logging.String("kind", kind))...).
						Log()
					break
				}
				n = optRes.SuccessCount
			} else {
				bulkRes, delErr := h.storage.BulkDelete(ctx, secCtx, batch, false)
				if delErr != nil {
					RetentionToleranceLog(h.logger).Warn(LogEventRetentionToleranceBulkDeleteEnforceMaxCountFailed).
						WithFields(append(jobLogFieldsByIDAndErr(jobID, delErr), logging.String("kind", kind))...).
						Log()
					break
				}
				n = bulkRes.SuccessCount
			}
			totalDeleted += n
			storagepkg.InvalidateListCacheForKind(kind)
			// Break only when no progress in this batch and no progress so far (allows continuing past partial batches)
			if n == 0 && totalDeleted == 0 {
				break
			}
		}
	} else if len(allIDs) > 0 && len(protectStatuses) > 0 {
		// Have CAS IDs but must respect protect_statuses: use batched List (status $nin) instead of Read() per object.
		RetentionToleranceLog(h.logger).Info(LogEventRetentionToleranceUsingBatchedListMaxCountProtect).
			JobID(jobID).
			Kind(kind).
			Int("to_delete", toDelete).
			Log()
		var batchedErr error
		totalDeleted, batchedErr = h.enforceMaxCountBatchedList(ctx, secCtx, storageCtx, jobID, kind, toDelete, protectStatuses, batchSize, maxBatches, bulkDeleteWorkers, count, maxCount, allIDs)
		if batchedErr != nil {
			return 0, batchedErr
		}
	} else {
		// Slow path: Fallback to List() (CAS index unavailable or no allIDs)
		var batchedErr error
		totalDeleted, batchedErr = h.enforceMaxCountBatchedList(ctx, secCtx, storageCtx, jobID, kind, toDelete, protectStatuses, batchSize, maxBatches, bulkDeleteWorkers, count, maxCount, allIDs)
		if batchedErr != nil {
			return 0, batchedErr
		}
	}

	if totalDeleted > 0 {
		RetentionToleranceLog(h.logger).Info(LogEventRetentionToleranceEnforcedMaxCountDeletedOldestObjects).
			JobID(jobID).
			Kind(kind).
			Deleted(totalDeleted).
			MaxCount(maxCount).
			Int("remaining_over_limit", count-totalDeleted-maxCount).
			Log()
		if isHighVolumeKind(kind) {
			if cache := storagepkg.GetGlobalHighVolumeEventCache(); cache != nil {
				cache.InvalidateForProject(h.projectRoot)
			}
		}
	}
	return totalDeleted, nil
}

// enforceMaxCountBatchedList uses batched List (filter status $nin protectStatuses, sort by created_at) + delete.
// Avoids per-object Read() when protect_statuses is set. Returns (deleted count, nil) or (0, ErrObjectOverfill).
func (h *RetentionToleranceHandler) enforceMaxCountBatchedList(
	ctx context.Context,
	secCtx *pkgctx.SecurityContext,
	storageCtx *pkgctx.StorageContext,
	jobID, kind string,
	toDelete int,
	protectStatuses []string,
	batchSize, maxBatches, bulkDeleteWorkers int,
	count, maxCount int,
	allIDs []string,
) (int, error) {
	filters := map[string]any{}
	if len(protectStatuses) > 0 {
		filters[objects.FieldKeyStatus] = map[string]any{"$nin": protectStatuses}
	}
	var totalDeleted int
	interrupt := concurrency.NewInterruptChecker(concurrency.DefaultInterruptCheckFrequency)

	if len(allIDs) > 0 {
		// Fast batched path: paginate through allIDs using $in filter. Avoids List() reading all files into memory.
		for i := 0; i < len(allIDs) && totalDeleted < toDelete; i += batchSize {
			if err := interrupt.Check(ctx); err != nil {
				RetentionToleranceLog(h.logger).Warn(LogEventRetentionToleranceMaxCountBatchedListCancelled).
					JobID(jobID).
					Kind(kind).
					Int("batches_processed", i/batchSize).
					Int("total_deleted", totalDeleted).
					WithError(err).
					Log()
				break
			}

			end := i + batchSize
			if end > len(allIDs) {
				end = len(allIDs)
			}
			chunk := allIDs[i:end]

			chunkFilters := map[string]any{
				objects.FieldKeyID: map[string]any{"$in": chunk},
			}
			if len(protectStatuses) > 0 {
				chunkFilters[objects.FieldKeyStatus] = map[string]any{"$nin": protectStatuses}
			}

			h.emitProgress(fmt.Sprintf("Enforcing max_count %s: ID batch %d/%d (deleted: %d/%d)...", kind, (i/batchSize)+1, (len(allIDs)+batchSize-1)/batchSize, totalDeleted, toDelete))

			listFilter := storagepkg.ListFilter{
				Kind:    kind,
				Limit:   batchSize,
				Filters: chunkFilters,
			}
			result, err := h.storage.List(ctx, secCtx, storageCtx, listFilter)
			if err != nil || len(result.Objects) == 0 {
				continue
			}

			var batchToDelete []string
			for _, obj := range result.Objects {
				if id, ok := obj[objects.FieldKeyID].(string); ok && id != emptyValue {
					batchToDelete = append(batchToDelete, id)
				}
			}

			if len(batchToDelete) == 0 {
				continue
			}

			// Only delete up to the remaining amount
			if totalDeleted+len(batchToDelete) > toDelete {
				batchToDelete = batchToDelete[:toDelete-totalDeleted]
			}

			var n int
			if fileStorage, ok := h.storage.(*storagepkg.FileObjectStorage); ok {
				optRes, delErr := fileStorage.BulkDeleteOptimized(ctx, secCtx, batchToDelete, false, bulkDeleteWorkers)
				if delErr != nil {
					RetentionToleranceLog(h.logger).Warn(LogEventRetentionToleranceBulkDeleteEnforceMaxCountBatchedListFailed).
						WithFields(append(jobLogFieldsByIDAndErr(jobID, delErr), logging.String("kind", kind))...).
						Log()
					break
				}
				n = optRes.SuccessCount
			} else {
				bulkRes, delErr := h.storage.BulkDelete(ctx, secCtx, batchToDelete, false)
				if delErr != nil {
					RetentionToleranceLog(h.logger).Warn(LogEventRetentionToleranceBulkDeleteEnforceMaxCountBatchedListFailed).
						WithFields(append(jobLogFieldsByIDAndErr(jobID, delErr), logging.String("kind", kind))...).
						Log()
					break
				}
				n = bulkRes.SuccessCount
			}
			totalDeleted += n
			storagepkg.InvalidateListCacheForKind(kind)
		}

		if totalDeleted == 0 && len(allIDs) > 0 && toDelete > 0 {
			overfillErr := errfmt.Errorf("kind %s: %w (count=%d max_count=%d)", kind, ErrObjectOverfill, count, maxCount)
			RetentionToleranceLog(h.logger).Error(LogEventRetentionToleranceObjectOverfillProtectedStatus, overfillErr).
				JobID(jobID).
				Kind(kind).
				Int("count", count).
				Int("max_count", maxCount).
				Log()
			return 0, overfillErr
		}

		return totalDeleted, nil
	}

	for batch := 0; batch < maxBatches && totalDeleted < toDelete; batch++ {
		if err := interrupt.Check(ctx); err != nil {
			RetentionToleranceLog(h.logger).Warn(LogEventRetentionToleranceMaxCountBatchedListCancelled).
				JobID(jobID).
				Kind(kind).
				Int("batches_processed", batch).
				Int("total_deleted", totalDeleted).
				WithError(err).
				Log()
			break
		}
		remaining := toDelete - totalDeleted
		batchLimit := batchSize
		if remaining < batchLimit {
			batchLimit = remaining
		}
		if maxBatches == retentionToleranceUnlimitedMaxBatches {
			h.emitProgress(fmt.Sprintf("Enforcing max_count %s: batch %d (unlimited cap) (deleted: %d/%d)...", kind, batch+1, totalDeleted, toDelete))
		} else {
			h.emitProgress(fmt.Sprintf("Enforcing max_count %s: batch %d/%d (deleted: %d/%d)...", kind, batch+1, maxBatches, totalDeleted, toDelete))
		}
		listFilter := storagepkg.ListFilter{
			Kind:    kind,
			Limit:   batchLimit,
			Filters: filters,
		}
		result, err := h.storage.List(ctx, secCtx, storageCtx, listFilter)
		if err != nil || len(result.Objects) == 0 {
			break
		}
		ids := make([]string, 0, len(result.Objects))
		for _, obj := range result.Objects {
			if id, ok := obj[objects.FieldKeyID].(string); ok && id != emptyValue {
				ids = append(ids, id)
			}
		}
		if len(ids) == 0 {
			if totalDeleted == 0 {
				overfillErr := errfmt.Errorf("kind %s: %w (count=%d max_count=%d)", kind, ErrObjectOverfill, count, maxCount)
				RetentionToleranceLog(h.logger).Error(LogEventRetentionToleranceObjectOverfillProtectedStatus, overfillErr).
					JobID(jobID).
					Kind(kind).
					Int("count", count).
					Int("max_count", maxCount).
					Log()
				return 0, overfillErr
			}
			break
		}
		var n int
		if fileStorage, ok := h.storage.(*storagepkg.FileObjectStorage); ok {
			optRes, delErr := fileStorage.BulkDeleteOptimized(ctx, secCtx, ids, false, bulkDeleteWorkers)
			if delErr != nil {
				RetentionToleranceLog(h.logger).Warn(LogEventRetentionToleranceBulkDeleteEnforceMaxCountBatchedListFailed).
					WithFields(append(jobLogFieldsByIDAndErr(jobID, delErr), logging.String("kind", kind))...).
					Log()
				break
			}
			n = optRes.SuccessCount
		} else {
			bulkRes, delErr := h.storage.BulkDelete(ctx, secCtx, ids, false)
			if delErr != nil {
				RetentionToleranceLog(h.logger).Warn(LogEventRetentionToleranceBulkDeleteEnforceMaxCountBatchedListFailed).
					WithFields(append(jobLogFieldsByIDAndErr(jobID, delErr), logging.String("kind", kind))...).
					Log()
				break
			}
			n = bulkRes.SuccessCount
		}
		totalDeleted += n
		storagepkg.InvalidateListCacheForKind(kind)

		if h.projectRoot != emptyValue {
			if queue := storagepkg.GetListingIndexWriteQueueForProjectRoot(h.projectRoot); queue != nil {
				_ = queue.FlushKind(kind, 5*time.Second)
			}
		}

		if n < len(ids) {
			break
		}
	}
	return totalDeleted, nil
}
