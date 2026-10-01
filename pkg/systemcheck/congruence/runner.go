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

	report, streamDirs, err := executeCongruenceAnalysis(ctx, projectRoot, storageProvider, secCtx, opts)
	if err != nil {
		return nil, err
	}

	reportDir, textPath, jsonPath, err := prepareReportPaths(projectRoot, opts)
	if err != nil {
		return nil, err
	}

	cacheStatus := GatherCacheStatus(projectRoot)
	integrity := scanIntegrityConcurrently(projectRoot, streamDirs)
	fsProjectSnap := captureFilesystemSnapshot(ctx, projectRoot, opts)

	appendCongruenceAlerts(report, integrity)

	if err := WriteReportFile(textPath, report, cacheStatus, integrity, fsProjectSnap); err != nil {
		return nil, errfmt.Newf("write report").Wrap(err)
	}

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	logging.Fluent(logger).Info("Object count report written").
		String("path", textPath).
		Int("total_disk", report.TotalDisk).
		Int("total_object", report.TotalObject).
		Int("alerts", len(report.Alerts)).
		Log()

	maybeEmitAlertEvent(ctx, projectRoot, storageProvider, report, logger, opts.EmitEvents)

	if jsonErr := WriteReportJSON(jsonPath, report, cacheStatus, integrity, fsProjectSnap); jsonErr != nil {
		logging.Fluent(logger).Debug("Could not write report JSON").Path(jsonPath).WithError(jsonErr).Log()
	}

	UpdateReportsIndex(reportDir, jsonPath, report, logger)
	WriteDashboardSnapshot(reportDir, report, logger, fsProjectSnap)

	ensureHighVolumeCacheAsync(ctx, projectRoot, storageProvider)
	recordCongruenceMetrics(projectRoot, report, fsProjectSnap, logger, opts.MetricsChunkRetentionDays)

	return &Result{
		Report:                    report,
		CacheStatus:               cacheStatus,
		Integrity:                 integrity,
		FilesystemProjectSnapshot: fsProjectSnap,
		TextReportPath:            textPath,
		JSONReportPath:            jsonPath,
	}, nil
}

func executeCongruenceAnalysis(ctx context.Context, projectRoot string, storageProvider storage.ObjectStorageProvider, secCtx *pkgctx.SecurityContext, opts Options) (*operational.CongruenceReport, []string, error) {
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
		return nil, nil, errfmt.Newf("congruence report").Wrap(err)
	}
	process.TouchMeaningfulActivity()

	if !opts.NoCache && objectCountByKindFromCache != nil && report.TotalObject > report.TotalDisk {
		cleanCount := objectidcache.CleanStaleCacheEntries(projectRoot)
		if cleanCount > 0 {
			process.TouchMeaningfulActivity()
		}
		optsNoCache := opOpts
		optsNoCache.ObjectCountByKindFromCache = nil
		report, err = operational.Run(ctx, storageProvider, secCtx, optsNoCache)
		if err != nil {
			return nil, nil, errfmt.Newf("congruence report (after stale cache cleanup)").Wrap(err)
		}
		process.TouchMeaningfulActivity()
	}

	return report, streamBackedDirsList, nil
}

func prepareReportPaths(projectRoot string, opts Options) (reportDir, textPath, jsonPath string, err error) {
	reportDir = filepath.Join(projectRoot, paths.ProjectDataDir, paths.LogsDir, paths.LogsReportsSubdir)
	if mkdirErr := fileutil.MkdirAll(reportDir, paths.DirPerm750); mkdirErr != nil {
		return "", "", "", errfmt.Newf("mkdir reports").Wrap(mkdirErr)
	}

	outputPath := opts.OutputPath
	if outputPath == "" {
		outputPath = filepath.Join(reportDir, fmt.Sprintf("object-count-report-%s.txt", zqktime.NowLayoutUTC(zqktime.LayoutLogRotateStamp)))
	}

	textPath = outputPath
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
	return reportDir, textPath, jsonPath, nil
}

func scanIntegrityConcurrently(projectRoot string, streamBackedDirs []string) ProcessIntegrity {
	var integrity ProcessIntegrity
	integrityDone := make(chan bool, 1)
	goroutinelabels.NewGoroutine("ocr_integrity_scan", "scan "+paths.ProcessDir+" for misplaced/unmanaged files").
		WithSignalOnExit(integrityDone).
		StartSimple(func() {
			integrity = GatherProcessIntegrity(projectRoot, streamBackedDirs)
		})
	select {
	case <-integrityDone:
	case <-time.After(IntegrityTimeout):
		tLogger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		logging.Fluent(tLogger).Debug("process integrity scan timed out; omitting from report (run system check for full scan)").
			String("timeout", IntegrityTimeout.String()).
			Log()
	}
	return integrity
}

func captureFilesystemSnapshot(ctx context.Context, projectRoot string, opts Options) *operational.FilesystemProjectSnapshot {
	if !opts.IncludeFilesystemSnapshot {
		return nil
	}
	process.TouchMeaningfulActivity()
	fsScope, scopeErr := operational.ParseFilesystemSnapshotScope(opts.FilesystemSnapshotScopeStr)
	if scopeErr != nil {
		return nil
	}
	fsLogger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	snapCtx, cancel := context.WithTimeout(ctx, 20*time.Minute)
	fsProjectSnap, snapErr := operational.RunFilesystemProjectSnapshot(snapCtx, projectRoot, fsScope)
	cancel()
	if snapErr != nil {
		logging.Fluent(fsLogger).Warn("Filesystem project snapshot failed; report omits full-tree counts").
			WithError(snapErr).
			Log()
	}
	return fsProjectSnap
}

func appendCongruenceAlerts(report *operational.CongruenceReport, integrity ProcessIntegrity) {
	var totalLegacy int
	for _, n := range report.LegacyStreamBackedByDir {
		totalLegacy += n
	}
	if totalLegacy > LegacyAdvisoryThreshold {
		report.Alerts = append(report.Alerts, paths.RewriteCanonicalCLIInvocations(fmt.Sprintf("legacy pre-migration YAML files in stream-backed dirs: %d total (run 'zqk system migrate-legacy-to-stream --kind <kind> --remove-legacy' or 'scripts/delete_unmanaged_audit_yaml.py --execute')", totalLegacy)))
	}

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
}

func maybeEmitAlertEvent(ctx context.Context, projectRoot string, storageProvider storage.ObjectStorageProvider, report *operational.CongruenceReport, logger logging.Logger, emitEvents bool) {
	if emitEvents && len(report.Alerts) > 0 {
		emitCtx := context.WithoutCancel(ctx)
		goroutinelabels.NewGoroutine("ocr_emit_alert_event", "emit congruence alert event to audit storage").
			StartWithContext(emitCtx, func(bgCtx context.Context) error {
				EmitCongruenceAlertEvent(bgCtx, projectRoot, storageProvider, report, logger)
				return nil
			})
	}
}

func ensureHighVolumeCacheAsync(ctx context.Context, projectRoot string, storageProvider storage.ObjectStorageProvider) {
	goroutinelabels.NewGoroutine("ocr_hv_cache_ensure", "ensure high-volume event cache is ready for stream volume metrics").
		StartWithContext(context.WithoutCancel(ctx), func(bgCtx context.Context) error {
			return storage.EnsureHighVolumeEventCacheReady(bgCtx, projectRoot, storageProvider, false)
		})
}

func recordCongruenceMetrics(projectRoot string, report *operational.CongruenceReport, fsProjectSnap *operational.FilesystemProjectSnapshot, logger logging.Logger, chunkRetentionDays int) {
	retentionDays := ResolveMetricsChunkRetentionDays(chunkRetentionDays)
	RecordObjectVolumeMetrics(projectRoot, report, logger, retentionDays)
	RecordStreamVolumeMetrics(projectRoot, logger, retentionDays)
	RecordFilesystemSnapshotMetrics(projectRoot, fsProjectSnap, logger, retentionDays)
}

// EmitCongruenceAlertEvent emits operational congruence alert events to logging and audit streams.
func EmitCongruenceAlertEvent(ctx context.Context, projectRoot string, storageProvider storage.ObjectStorageProvider, report *operational.CongruenceReport, logger logging.Logger) {
	auditRouter := coordination.AuditRouter(projectRoot, storageProvider)
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

	if emitErr := loggingRouter.Emit(ctx, eventCtx); emitErr != nil {
		logging.Fluent(logger).Debug("Failed to emit logging event").WithError(emitErr).Log()
	}
	if emitErr := auditRouter.Emit(ctx, eventCtx); emitErr != nil {
		logging.Fluent(logger).Debug("Failed to emit audit event").WithError(emitErr).Log()
	}
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
