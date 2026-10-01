package congruence

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/config"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/coordination"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objectidcache"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/operational"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/process"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqktime"
)

// RunCongruence executes operational congruence analysis, writes reports, and records metrics.
func RunCongruence(ctx context.Context, projectRoot string, storageProvider storage.ObjectStorageProvider, secCtx *pkgctx.SecurityContext, opts Options) (*Result, error) {
	if projectRoot == "" {
		return nil, errfmt.Errorf("project root not found")
	}
	if secCtx == nil {
		secCtx = pkgctx.NewSystemSecurityContext()
	}

	zqkBin := ""
	if opts.IncludeInternal {
		zqkBin = ResolveZQKBin(projectRoot)
	}

	objectCountByKindFromCache := GatherObjectCountByKindForReport(ctx, projectRoot, storageProvider, opts.NoCache)
	streamBackedDirsList := StreamBackedDirsForReport()

	opOpts := operational.RunOptions{
		ProjectRoot:                projectRoot,
		IncludeInternal:            opts.IncludeInternal,
		ZQKBin:                     zqkBin,
		InternalTimeout:            operational.DefaultInternalCountTimeout,
		DisparityThreshold:         opts.DisparityThreshold,
		ObjectCountByKindFromCache: objectCountByKindFromCache,
		StreamBackedDirs:           streamBackedDirsList,
	}

	report, err := operational.Run(ctx, storageProvider, secCtx, opOpts)
	if err != nil {
		return nil, errfmt.Newf("congruence report").Wrap(err)
	}
	process.TouchMeaningfulActivity()

	// If cache was used and total_object > total_disk, cache is stale; clean and re-run.
	if !opts.NoCache && objectCountByKindFromCache != nil && report.TotalObject > report.TotalDisk {
		objectidcache.CleanStaleCacheEntries(projectRoot)
		optsNoCache := opOpts
		optsNoCache.ObjectCountByKindFromCache = nil
		report, err = operational.Run(ctx, storageProvider, secCtx, optsNoCache)
		if err != nil {
			return nil, errfmt.Newf("congruence report (after stale cache cleanup)").Wrap(err)
		}
		process.TouchMeaningfulActivity()
	}

	reportDir := filepath.Join(projectRoot, paths.ProjectDataDir, paths.LogsDir, paths.LogsReportsSubdir)
	if err := fileutil.MkdirAll(reportDir, paths.DirPerm750); err != nil {
		return nil, errfmt.Newf("mkdir reports").Wrap(err)
	}

	outputPath := opts.OutputPath
	if outputPath == "" {
		outputPath = filepath.Join(reportDir, fmt.Sprintf("object-count-report-%s.txt", zqktime.NowLayoutUTC(zqktime.LayoutLogRotateStamp)))
	}

	textPath := outputPath
	jsonPath := ""
	if opts.UserProvidedReportFile && filepath.Ext(outputPath) == ReportExtJSON {
		base := strings.TrimSuffix(outputPath, ReportExtJSON)
		textPath = base + ReportExtTXT
		jsonPath = outputPath
	} else {
		jsonPath = textPath
		if filepath.Ext(jsonPath) == ReportExtTXT {
			jsonPath = jsonPath[:len(jsonPath)-4] + ReportExtJSON
		} else {
			jsonPath += ReportExtJSON
		}
	}

	cacheStatus := GatherCacheStatus(projectRoot)

	var integrity ProcessIntegrity
	integrityDone := make(chan bool, 1)
	goroutinelabels.NewGoroutine("ocr_integrity_scan", "scan "+paths.ProcessDir+" for misplaced/unmanaged files").
		WithSignalOnExit(integrityDone).
		StartSimple(func() {
			integrity = GatherProcessIntegrity(projectRoot, streamBackedDirsList)
		})
	select {
	case <-integrityDone:
	case <-time.After(IntegrityTimeout):
		tLogger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		logging.Fluent(tLogger).Debug("process integrity scan timed out; omitting from report (run system check for full scan)").
			String("timeout", IntegrityTimeout.String()).
			Log()
	}

	var fsProjectSnap *operational.FilesystemProjectSnapshot
	if opts.IncludeFilesystemSnapshot {
		process.TouchMeaningfulActivity()
		fsScope, scopeErr := operational.ParseFilesystemSnapshotScope(opts.FilesystemSnapshotScopeStr)
		if scopeErr != nil {
			return nil, errfmt.Newf("filesystem snapshot scope").Wrap(scopeErr)
		}
		fsLogger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		snapCtx, cancel := context.WithTimeout(ctx, 20*time.Minute)
		var snapErr error
		fsProjectSnap, snapErr = operational.RunFilesystemProjectSnapshot(snapCtx, projectRoot, fsScope)
		cancel()
		if snapErr != nil {
			logging.Fluent(fsLogger).Warn("Filesystem project snapshot failed; report omits full-tree counts").
				WithError(snapErr).
				Log()
		}
	}

	var totalLegacy int
	for _, n := range report.LegacyStreamBackedByDir {
		totalLegacy += n
	}
	if totalLegacy > LegacyAdvisoryThreshold {
		report.Alerts = append(report.Alerts, paths.RewriteCanonicalCLIInvocations(fmt.Sprintf("legacy pre-migration YAML files in stream-backed dirs: %d total (run 'zqk system migrate-legacy-to-stream --kind <kind> --remove-legacy' or 'scripts/delete_unmanaged_audit_yaml.py --execute')", totalLegacy)))
	}

	if err := WriteReportFile(textPath, report, cacheStatus, integrity, fsProjectSnap); err != nil {
		return nil, errfmt.Newf("write report").Wrap(err)
	}

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	if len(integrity.MisplacedByKind) > 0 {
		var n int
		for _, filePaths := range integrity.MisplacedByKind {
			n += len(filePaths)
		}
		report.Alerts = append(report.Alerts, fmt.Sprintf("misplaced files: %d (wrong directory for kind)", n))
	}
	if len(integrity.UnmanagedFiles) > 0 {
		report.Alerts = append(report.Alerts, fmt.Sprintf("unmanaged files (not in cache): %d", len(integrity.UnmanagedFiles)))
	}
	if len(integrity.UnknownDirs) > 0 || len(integrity.FilesAtRoot) > 0 {
		report.Alerts = append(report.Alerts, fmt.Sprintf("trash: %d unknown dirs, %d files at %s root", len(integrity.UnknownDirs), len(integrity.FilesAtRoot), paths.ProcessDir))
	}

	logging.Fluent(logger).Info("Object count report written").
		String("path", textPath).
		Int("total_disk", report.TotalDisk).
		Int("total_object", report.TotalObject).
		Int("alerts", len(report.Alerts)).
		Log()

	if opts.EmitEvents && len(report.Alerts) > 0 {
		emitCtx := context.WithoutCancel(ctx)
		goroutinelabels.NewGoroutine("ocr_emit_alert_event", "emit congruence alert event to audit storage").
			StartWithContext(emitCtx, func(bgCtx context.Context) error {
				EmitCongruenceAlertEvent(bgCtx, projectRoot, storageProvider, report, logger)
				return nil
			})
	}

	if err := WriteReportJSON(jsonPath, report, cacheStatus, integrity, fsProjectSnap); err != nil {
		logging.Fluent(logger).Debug("Could not write report JSON").Path(jsonPath).WithError(err).Log()
	}

	UpdateReportsIndex(reportDir, jsonPath, report, logger)
	WriteDashboardSnapshot(reportDir, report, logger, fsProjectSnap)

	goroutinelabels.NewGoroutine("ocr_hv_cache_ensure", "ensure high-volume event cache is ready for stream volume metrics").
		StartWithContext(context.WithoutCancel(ctx), func(bgCtx context.Context) error {
			return storage.EnsureHighVolumeEventCacheReady(bgCtx, projectRoot, storageProvider, false)
		})

	retentionDays := ResolveMetricsChunkRetentionDays(opts.MetricsChunkRetentionDays)
	RecordObjectVolumeMetrics(projectRoot, report, logger, retentionDays)
	RecordStreamVolumeMetrics(projectRoot, logger, retentionDays)
	RecordFilesystemSnapshotMetrics(projectRoot, fsProjectSnap, logger, retentionDays)

	return &Result{
		Report:                    report,
		CacheStatus:               cacheStatus,
		Integrity:                 integrity,
		FilesystemProjectSnapshot: fsProjectSnap,
		TextReportPath:            textPath,
		JSONReportPath:            jsonPath,
	}, nil
}

// EmitCongruenceAlertEvent emits operational congruence alert events to logging and audit streams.
func EmitCongruenceAlertEvent(ctx context.Context, projectRoot string, storageProvider storage.ObjectStorageProvider, report *operational.CongruenceReport, logger logging.Logger) {
	auditRouter := coordination.NewStorageAuditRouter(projectRoot, storageProvider)
	loggingRouter := &coordination.DefaultLoggingRouter{}

	auditMetadata := map[string]any{
		objects.FieldKeyEventType: "operational_congruence_alert",
		objects.FieldKeyOperation: "Object count congruence report",
		OCRKeyTotalDisk:           report.TotalDisk,
		OCRKeyTotalObject:         report.TotalObject,
		OCRKeyAlertCount:          len(report.Alerts),
		OCRKeyDirsWithDisparity:   report.KindsWithDisparity,
		"disparity_by_dir":        report.DisparityByDir,
		objects.FieldKeySeverity:  OCRSeverityWarning,
	}
	loggingFields := []coordination.LoggingField{
		{Key: OCRKeyTotalDisk, Value: report.TotalDisk},
		{Key: OCRKeyTotalObject, Value: report.TotalObject},
		{Key: OCRKeyAlertCount, Value: len(report.Alerts)},
		{Key: OCRKeyDirsWithDisparity, Value: report.KindsWithDisparity},
	}
	eventData := &coordination.EventData{
		LoggingFields: loggingFields,
		AuditMetadata: auditMetadata,
		MetricsData:   map[string]any{OCRKeyTotalDisk: report.TotalDisk, OCRKeyTotalObject: report.TotalObject, "disparity_dirs": report.KindsWithDisparity},
	}
	operationID := fmt.Sprintf("congruence_report_%d", time.Now().Unix())
	eventCtx := coordination.NewEventContext(operationID, "operational_congruence", OCRSeverityWarning).
		WithEventData(eventData).
		WithContext(ctx).
		WithChannels(true, true, true, false)
	_ = loggingRouter.Emit(ctx, eventCtx)
	_ = auditRouter.Emit(ctx, eventCtx)
}

// ResolveZQKBin locates the zqk binary for internal commands.
func ResolveZQKBin(projectRoot string) string {
	adminBinName := "zqk-admin"
	for _, dir := range []string{filepath.Join(projectRoot, "bin"), projectRoot} {
		p := filepath.Join(dir, adminBinName)
		if info, err := fileutil.Stat(p); err == nil && info.Mode().IsRegular() {
			return p
		}
	}
	return adminBinName
}

// ResolveMetricsChunkRetentionDays returns the resolved retention days.
func ResolveMetricsChunkRetentionDays(flagValue int) int {
	if flagValue > 0 {
		if flagValue > MaxMetricsChunkRetentionDays {
			return MaxMetricsChunkRetentionDays
		}
		return flagValue
	}
	if n := config.MaintenanceMetricsChunkRetentionDays().OrDefault(30); n > 0 {
		if n > MaxMetricsChunkRetentionDays {
			return MaxMetricsChunkRetentionDays
		}
		return n
	}
	return DefaultMetricsChunkRetentionDays
}
