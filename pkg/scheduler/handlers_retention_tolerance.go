package scheduler

import (
	"context"
	"fmt"
	"math"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	caspkg "github.com/zqk-os/zqk/pkg/storage/cas"

	"github.com/zqk-os/zqk/pkg/concurrency"
	"github.com/zqk-os/zqk/pkg/config"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/nildecode"
	"github.com/zqk-os/zqk/pkg/objects"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
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

		// 1) Archive: objects older than archive_after -> status = archived.
		// High-volume kinds with empty protect_statuses use OldestIDs cleanup/enforce; BulkUpdate
		// archive of thousands contends with audit aggregation WAL (stuck SCH-retention-* jobs).
		// TRACK: follow-up in kernel backlog
		if tol.ArchiveEnabled {
			if skipArchiveForOldestIDsPath(kind, tol.ProtectStatuses) {
				h.emitProgress(fmt.Sprintf("Skipping archive for %s (empty protect_statuses → OldestIDs cleanup/enforce)", kind))
				RetentionToleranceLog(h.logger).Info(LogEventRetentionToleranceSkippedArchiveOldestIDsPath).
					JobID(job.ID).
					Kind(kind).
					Log()
			} else {
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
			if flushErr := caspkg.GetListingIndexWriteQueueForProjectRoot(h.projectRoot).FlushKind(kind, 5*time.Second); flushErr != nil {
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
		// are GC'd, and runtime delta overlays are processed after a successful retention tolerance cycle.
		// When KINDS is set, only steward those kinds (dedicated jobs must not walk every HV stream).
		// TRACK: follow-up in kernel backlog
		storagepkg.PostRetentionStreamStewardshipFiltered(h.projectRoot, job.ID, h.logger, kindFilter)
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
			// Never auto-enroll a kernel-critical kind. kernel_critical means "must not be silently
			// hard-deleted", which is exactly what enrollment here causes: the safe defaults treat
			// `archived` as collectable (it is deliberately absent from DefaultProtectStatuses), and
			// cleanup deletes through BulkDeleteOptimized -> cas.BatchDelete, which does no dependent
			// check. The kernel_critical guard that would have refused this is bypassed because
			// retention runs as the system account.
			//
			// This is not hypothetical: it destroyed 270 archived objects in one window — 253 criteria
			// plus convergence_sessions, priority_plans, a mission, a strategic_plan, technical_debts,
			// tests and backlog items — and left GhostRefs behind, which the next system check
			// reported as referenced-but-missing criteria. Every one of those kinds is
			// kernel_critical and none was listed in isCatalogKind, so discovery enrolled them all.
			//
			// Resolved from the spec plane rather than by extending isCatalogKind, because that list
			// is hand-maintained and its omissions are precisely the damage: a kind is protected only
			// if someone remembered to type it. Kinds retention genuinely exists to trim
			// (audit_event, change_journal_entry, scheduler_job, agent_task) are not kernel_critical
			// and are unaffected.
			if objects.IsKernelCriticalKind(kind) {
				RetentionToleranceLog(h.logger).Debug("retention: skipping kernel-critical kind for auto-enrollment").
					WithFields(logging.String("kind", kind)).
					Log()
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
		"glossary_term_relation", "brand", "organization", "division", "department", "team",
		"team_configuration", "keystore_entry", "auth_strategy", "account",
		"prompt_template", "agent_skill", "watchdog_registration", "lifecycle", "resolver", "namespace",
		"namespace_registry", "provider_profile", "scenario", "strategic_context",
		"field_registry", "extensible_object", "command_spec", "partnership", "corporate_initiative",
		"organizational_change",
		"stakeholder_profile", "library", "workflow", "vitality_report", "maturation_report",
		"risk_blocker", "capacity_advertisement", "compute_advertisement", "fission_event",
		"partner_profile", "provider_profile_relation",
		"stakeholder_profile_relation",
		// Process Q&A: no lifecycle archive status; safe-default retention was writing invalid status=archived.
		"question":
		return true
	default:
		return false
	}
}

// archiveOldObjects lists objects of kind with created_at < cutoff and status != archiveStatus,
// then bulk-updates to the kind's lifecycle archive status. Skips kinds with no archive:true
// status (must not invent literal "archived" — that churned questions into Layer-1 error).
// TRACK: follow-up in kernel backlog
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
	archiveStatus, ok := objects.ArchiveStatusForKind(kind)
	if !ok {
		RetentionToleranceLog(h.logger).Info(LogEventRetentionToleranceArchiveSkippedNoLifecycleStatus).
			JobID(jobID).
			Kind(kind).
			Log()
		return 0
	}
	batchSize, maxBatches = normalizeRetentionBatchLimits(batchSize, maxBatches)

	cutoffStr := cutoff.Format(time.RFC3339)
	filters := map[string]any{
		objects.FieldKeyCreatedAt: map[string]any{"$lt": cutoffStr},
		objects.FieldKeyStatus:    map[string]any{"$ne": archiveStatus},
	}
	if kind == objects.KindPriorityPlan {
		// Only last-child-complete plans are promote→archived candidates.
		filters[objects.FieldKeyStatus] = objects.ObjectStatusComplete
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
				objects.FieldKeyStatus:    map[string]any{"$ne": archiveStatus},
			}
			if kind == objects.KindPriorityPlan {
				chunkFilters[objects.FieldKeyStatus] = objects.ObjectStatusComplete
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

			archived := h.archiveListedObjects(ctx, secCtx, jobID, kind, archiveStatus, result.Objects)
			totalArchived += archived
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

		n := h.archiveListedObjects(ctx, secCtx, jobID, kind, archiveStatus, result.Objects)
		if n == 0 {
			break
		}
		totalArchived += n

		if h.projectRoot != emptyValue {
			if queue := caspkg.GetListingIndexWriteQueueForProjectRoot(h.projectRoot); queue != nil {
				_ = queue.FlushKind(kind, 5*time.Second)
			}
		}

		if n < len(result.Objects) {
			break
		}
	}
	return totalArchived
}

// archiveListedObjects archives a List page: complete priority_plans via promote
// membrane; other kinds via BulkUpdate (plan-linked BLIs under non-archived plans skipped).
func (h *RetentionToleranceHandler) archiveListedObjects(
	ctx context.Context,
	secCtx *pkgctx.SecurityContext,
	jobID, kind, archiveStatus string,
	objs []map[string]any,
) int {
	if len(objs) == 0 {
		return 0
	}
	var toUpdate []storagepkg.BulkUpdateItem
	archived := 0
	for _, obj := range objs {
		id, _ := obj[objects.FieldKeyID].(string)
		if id == emptyValue {
			continue
		}
		if kind == objects.KindSchedulerJob {
			execMode, _ := obj[objects.FieldKeyExecutionMode].(string)
			if execMode == "reusable" {
				continue
			}
		}
		if !retentionArchiveCandidate(ctx, secCtx, h.storage, kind, obj, archiveStatus) {
			continue
		}
		if kind == objects.KindPriorityPlan {
			if err := h.archiveViaPromoteMembrane(ctx, secCtx, jobID, id); err != nil {
				RetentionToleranceLog(h.logger).Warn(LogEventRetentionToleranceBulkUpdateArchiveFailed).
					WithFields(append(jobLogFieldsByIDAndErr(jobID, err),
						logging.String("kind", kind),
						logging.String("seed_id", id),
						logging.String("path", "promote_membrane"))...).
					Log()
				continue
			}
			archived++
			continue
		}
		toUpdate = append(toUpdate, storagepkg.BulkUpdateItem{
			ID:      id,
			Updates: map[string]any{objects.FieldKeyStatus: archiveStatus},
		})
	}
	if len(toUpdate) == 0 {
		return archived
	}
	bulkResult, err := h.storage.BulkUpdate(ctx, secCtx, toUpdate)
	if err != nil {
		RetentionToleranceLog(h.logger).Warn(LogEventRetentionToleranceBulkUpdateArchiveFailed).
			WithFields(append(jobLogFieldsByIDAndErr(jobID, err), logging.String("kind", kind))...).
			Log()
		return archived
	}
	storagepkg.InvalidateListCacheForKind(kind)
	return archived + bulkResult.SuccessCount
}

func normalizeRetentionBatchLimits(batchSize, maxBatches int) (int, int) {
	if batchSize <= 0 {
		batchSize = defaultRetentionToleranceBatchSize
	}
	switch {
	case maxBatches == -1:
		maxBatches = retentionToleranceUnlimitedMaxBatches
	case maxBatches <= 0:
		maxBatches = defaultRetentionToleranceMaxBatches
	}
	return batchSize, maxBatches
}

func buildStatusExclusionFilter(protectStatuses []string) map[string]any {
	filters := map[string]any{}
	if len(protectStatuses) > 0 {
		filters[objects.FieldKeyStatus] = map[string]any{"$nin": protectStatuses}
	}
	return filters
}

