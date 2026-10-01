package system

import (
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/config"
	"github.com/zqk-os/zqk/pkg/datacell"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"

	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/coordination"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/metrics"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/operational"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/pipeline"
	"github.com/zqk-os/zqk/pkg/process"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/zqktime"
)

const pipelineKindRunObjectCountReport = "system.run_object_count_report"

const (
	reportExtJSON = ".json"
	reportExtTXT  = ".txt"
)

const (
	ocrKeyTotalDisk         = "total_disk"
	ocrKeyTotalObject       = "total_object"
	ocrKeyAlertCount        = "alert_count"
	ocrKeyDirsWithDisparity = "dirs_with_disparity"
	ocrSeverityWarning      = "warning"
)

const (
	streamDirAudit        = "audit"
	streamDirMCPSessions  = "mcp_sessions"
	ocrProcessInternalDir = "_internal"
)

// NewObjectCountReportCmd creates a command that runs operational congruence report
// (disk vs object vs internal counts), writes to file, and can emit metrics/alerts via coordinator.
// Command structure and flags are from .zqk/cli/specs/system/object_count_report_command.yaml.
func NewObjectCountReportCmd() *cobra.Command {
	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemObjectCountReportCommandBuilder(), &cobra.Command{Use: "object-count-report"})
	cmd.Flags().Int("metrics-chunk-retention-days", 0,
		"Prune metrics time-series .chunk files older than N days under .zqk/metrics/ (object_volume, stream_volume, filesystem_snapshot). 0 uses "+zqkenv.MetricsChunkRetentionDays().Name()+" or default "+strconv.Itoa(defaultMetricsChunkRetentionDays))
	cli.BindAsyncProgress(cmd, runObjectCountReport)
	return cmd
}

func runObjectCountReport(cmd *cobra.Command, args []string) error {
	type state struct {
		err error
	}
	st := &state{}

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	runCtx := cmd.Context()
	if runCtx == nil {
		runCtx = pkgctx.NewSystemContext()
	}

	pl := pipeline.NewBuilder(pipelineKindRunObjectCountReport, logger).
		WithMetricsConfig(pipeline.DefaultMetricsConfig(logger)).
		WithProfile(string(pkgctx.ProfileSystem)).
		AddStage(pipeline.StageIngest, func(pctx *pipeline.Context, payload any) (any, error) {
			st.err = runObjectCountReportImpl(cmd, args)
			return st, nil
		}).
		AddStage(pipeline.StageFinalize, func(pctx *pipeline.Context, payload any) (any, error) {
			return st, nil
		}).
		Build()

	_, runErr := pl.Run(&pipeline.Context{Ctx: runCtx, Outcome: make(map[string]any)}, st)
	if runErr != nil {
		return runErr
	}
	return st.err
}

func runObjectCountReportImpl(cmd *cobra.Command, _ []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, _ []string, proc *cli.Processor) error {
		outputPath, _ := cmd.Flags().GetString("report-file")
		emitEvents, _ := cmd.Flags().GetBool("emit-events")
		includeInternal, _ := cmd.Flags().GetBool("include-internal")
		disparityThreshold, _ := cmd.Flags().GetInt("disparity-threshold")
		noCache, _ := cmd.Flags().GetBool("no-cache")
		includeFilesystemSnapshot, _ := cmd.Flags().GetBool("include-filesystem-snapshot")
		filesystemSnapshotScopeStr, _ := cmd.Flags().GetString("filesystem-snapshot-scope")
		projectRoot := ""
		if cliCtx := cli.GetContext(cmd); cliCtx != nil {
			projectRoot = cliCtx.ProjectRoot
		}
		projectRoot = ProjectRootOrResolve(projectRoot)
		if projectRoot == emptyValue {
			return errfmt.Errorf("project root not found")
		}

		reportFileFlag := cmd.Flags().Lookup("report-file")
		userProvidedReportFile := reportFileFlag != nil && reportFileFlag.Changed

		var err error
		_ = err
		storageProvider := proc.Storage()
		secCtx := proc.SecurityContext()
		if secCtx == nil {
			secCtx = pkgctx.NewSystemSecurityContext()
		}

		zqkBin := ""
		if includeInternal {
			zqkBin = resolveZQKBin(projectRoot)
		}

		// Single source of truth: same count logic as improvement-report (object_count_helpers.go).
		objectCountByKindFromCache := GatherObjectCountByKindForReport(cmd.Context(), projectRoot, storageProvider, noCache)
		streamBackedDirsList := StreamBackedDirsForReport()

		opts := operational.RunOptions{
			ProjectRoot:                projectRoot,
			IncludeInternal:            includeInternal,
			ZQKBin:                     zqkBin,
			InternalTimeout:            operational.DefaultInternalCountTimeout,
			DisparityThreshold:         disparityThreshold,
			ObjectCountByKindFromCache: objectCountByKindFromCache,
			StreamBackedDirs:           streamBackedDirsList,
		}
		report, err := operational.Run(cmd.Context(), storageProvider, secCtx, opts)
		if err != nil {
			return errfmt.Newf("congruence report").Wrap(err)
		}
		process.TouchMeaningfulActivity()

		// OBJECT_COUNT_SELF_MAINTENANCE: if cache was used and total_object > total_disk, cache is stale; clean and re-run with storage counts so report is accurate without --no-cache.
		if !noCache && objectCountByKindFromCache != nil && report.TotalObject > report.TotalDisk {
			CleanStaleCacheEntries(projectRoot)
			optsNoCache := opts
			optsNoCache.ObjectCountByKindFromCache = nil
			report, err = operational.Run(cmd.Context(), storageProvider, secCtx, optsNoCache)
			if err != nil {
				return errfmt.Newf("congruence report (after stale cache cleanup)").Wrap(err)
			}
			process.TouchMeaningfulActivity()
		}

		reportDir := filepath.Join(projectRoot, paths.ProjectDataDir, paths.LogsDir, paths.LogsReportsSubdir)
		if err := fileutil.MkdirAll(reportDir, paths.DirPerm750); err != nil {
			return errfmt.Newf("mkdir reports").Wrap(err)
		}
		if outputPath == emptyValue {
			outputPath = filepath.Join(reportDir, fmt.Sprintf("object-count-report-%s.txt", zqktime.NowLayoutUTC(zqktime.LayoutLogRotateStamp)))
		}

		// Normalize output paths:
		// - Default: text report is <...>.txt and JSON report is <...>.json (same base).
		// - If user passes a .json path via --report-file, treat that as the JSON path and
		//   write the text report alongside it with the same base and .txt extension.
		textPath := outputPath
		jsonPath := ""
		if userProvidedReportFile && filepath.Ext(outputPath) == reportExtJSON {
			base := strings.TrimSuffix(outputPath, reportExtJSON)
			textPath = base + reportExtTXT
			jsonPath = outputPath
		} else {
			jsonPath = textPath
			if filepath.Ext(jsonPath) == reportExtTXT {
				jsonPath = jsonPath[:len(jsonPath)-4] + reportExtJSON
			} else {
				jsonPath += reportExtJSON
			}
		}
		outputPath = textPath

		cacheStatus := gatherCacheStatus(projectRoot)

		// gatherProcessIntegrity walks all of .zqk/process and reads every YAML file for id+kind.
		// On large projects this takes 20-60s. Run concurrently with a tight deadline so the report
		// is fast; integrity findings appear when the scan completes within the budget.
		// WithSignalOnExit fires after the work function returns, establishing happens-before for
		// the integrity variable read below.
		const integrityTimeout = 3 * time.Second
		var integrity ProcessIntegrity
		integrityDone := make(chan bool, 1)
		goroutinelabels.NewGoroutine("ocr_integrity_scan", "scan "+paths.ProcessDir+" for misplaced/unmanaged files").
			WithSignalOnExit(integrityDone).
			StartSimple(func() {
				integrity = gatherProcessIntegrity(projectRoot, streamBackedDirsList)
			})
		select {
		case <-integrityDone:
			// integrity assigned before signal; happens-before guaranteed by channel receive
		case <-time.After(integrityTimeout):
			tLogger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
			logging.Fluent(tLogger).Debug("process integrity scan timed out; omitting from report (run system check for full scan)").
				String("timeout", integrityTimeout.String()).
				Log()
		}

		var fsProjectSnap *operational.FilesystemProjectSnapshot
		if includeFilesystemSnapshot {
			process.TouchMeaningfulActivity()
			fsScope, scopeErr := operational.ParseFilesystemSnapshotScope(filesystemSnapshotScopeStr)
			if scopeErr != nil {
				return errfmt.Newf("filesystem snapshot scope").Wrap(scopeErr)
			}
			fsLogger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
			snapCtx, cancel := context.WithTimeout(cmd.Context(), 20*time.Minute)
			var snapErr error
			fsProjectSnap, snapErr = operational.RunFilesystemProjectSnapshot(snapCtx, projectRoot, fsScope)
			cancel()
			if snapErr != nil {
				logging.Fluent(fsLogger).Warn("Filesystem project snapshot failed; report omits full-tree counts").
					WithError(snapErr).
					Log()
			}
		}

		// Advisory: legacy YAML files remaining in stream-backed dirs (populated by congruence.Run).
		// Added before writeReportFile so it appears in the written file AND in event emission.
		// Threshold: >100 files total is surfaced as an advisory so operators know cleanup is needed.
		const legacyAdvisoryThreshold = 100
		var totalLegacy int
		for _, n := range report.LegacyStreamBackedByDir {
			totalLegacy += n
		}
		if totalLegacy > legacyAdvisoryThreshold {
			report.Alerts = append(report.Alerts, paths.RewriteCanonicalCLIInvocations(fmt.Sprintf("legacy pre-migration YAML files in stream-backed dirs: %d total (run 'zqk system migrate-legacy-to-stream --kind <kind> --remove-legacy' or 'scripts/delete_unmanaged_audit_yaml.py --execute')", totalLegacy)))
		}

		if err := writeReportFile(outputPath, report, cacheStatus, integrity, fsProjectSnap); err != nil {
			return errfmt.Newf("write report").Wrap(err)
		}

		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		// Add integrity findings to alerts so they are included in event emission
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
			String("path", outputPath).
			Int("total_disk", report.TotalDisk).
			Int("total_object", report.TotalObject).
			Int("alerts", len(report.Alerts)).
			Log()

		if emitEvents && len(report.Alerts) > 0 {
			// Fire-and-forget: audit event write must not block the report latency budget.
			// context.WithoutCancel so the goroutine outlives the command's context.
			emitCtx := context.WithoutCancel(cmd.Context())
			goroutinelabels.NewGoroutine("ocr_emit_alert_event", "emit congruence alert event to audit storage").
				StartWithContext(emitCtx, func(ctx context.Context) error {
					emitCongruenceAlertEvent(ctx, projectRoot, storageProvider, report, logger)
					return nil
				})
		}

		// Also write JSON for programmatic use (same base path, .json)
		if err := writeReportJSON(jsonPath, report, cacheStatus, integrity, fsProjectSnap); err != nil {
			logging.Fluent(logger).Debug("Could not write report JSON").Path(jsonPath).WithError(err).Log()
		}

		// Durable index and dashboard-friendly snapshot for aggregation and dashboards (best-effort).
		updateReportsIndex(reportDir, jsonPath, report, logger)
		writeDashboardSnapshot(reportDir, report, logger, fsProjectSnap)

		// Best-effort: ensure high-volume cache is ready so stream_volume metrics have counts
		// (when run from CLI the cache may not yet be built). Run async so the cache build does
		// not block after the report is already written.
		goroutinelabels.NewGoroutine("ocr_hv_cache_ensure", "ensure high-volume event cache is ready for stream volume metrics").
			StartWithContext(context.WithoutCancel(cmd.Context()), func(ctx context.Context) error {
				return storage.EnsureHighVolumeEventCacheReady(ctx, projectRoot, storageProvider, false)
			})
		// Append compact object volume and stream volume metrics for health checks and trends.
		retentionDays := resolveMetricsChunkRetentionDays(cmd)
		recordObjectVolumeMetrics(projectRoot, report, logger, retentionDays)
		recordStreamVolumeMetrics(projectRoot, logger, retentionDays)
		recordFilesystemSnapshotMetrics(projectRoot, fsProjectSnap, logger, retentionDays)

		return nil
	})(cmd, nil)
}

// CacheStatus holds object ID cache and reverse reference index status for the report.
type CacheStatus struct {
	ObjectIDCache struct {
		Path        string         `json:"path"`
		Loaded      bool           `json:"loaded"`
		Total       int            `json:"total,omitempty"`
		CountByKind map[string]int `json:"count_by_kind,omitempty"`
	} `json:"object_id_cache"`
	ReverseReferenceIndex struct {
		Path              string `json:"path"`
		Loaded            bool   `json:"loaded"`
		ReferencedIDCount int    `json:"referenced_id_count,omitempty"`
	} `json:"reverse_reference_index"`
}

func gatherCacheStatus(projectRoot string) CacheStatus {
	var status CacheStatus
	cacheDir := filepath.Join(projectRoot, paths.ProjectDataDir, paths.CacheDir)
	status.ObjectIDCache.Path = filepath.Join(cacheDir, paths.ObjectIDCacheFile)
	objCache := GetGlobalObjectIDCache()
	if loaded := TryLoadObjectIDCacheOnly(projectRoot); loaded {
		status.ObjectIDCache.Loaded = true
		status.ObjectIDCache.Total = objCache.GetEntryCount()
		status.ObjectIDCache.CountByKind = objCache.CountByKind()
	}

	revIndex := storage.GetGlobalReverseReferenceIndex()
	status.ReverseReferenceIndex.Path = revIndex.GetCacheFilePath(projectRoot)
	if loaded, _ := revIndex.LoadCache(projectRoot); loaded {
		status.ReverseReferenceIndex.Loaded = true
		status.ReverseReferenceIndex.ReferencedIDCount = revIndex.ReferencedIDCount()
	}
	return status
}

// ProcessIntegrity holds misplaced files, unmanaged files (not in object ID cache), and trash (unknown dirs, files at root).
type ProcessIntegrity struct {
	MisplacedByKind map[string][]string `json:"misplaced_by_kind,omitempty"` // kind -> paths in wrong dir
	UnmanagedFiles  []string            `json:"unmanaged_files,omitempty"`   // YAML on disk not in cache (not managed by CAS/index)
	UnknownDirs     []string            `json:"unknown_dirs,omitempty"`      // dirs under .zqk/process that don't map to a kind
	FilesAtRoot     []string            `json:"files_at_root,omitempty"`     // files directly under .zqk/process (scattered trash)
}

func gatherProcessIntegrity(projectRoot string, streamBackedDirs []string) ProcessIntegrity {
	var out ProcessIntegrity
	processDir := datacell.ProcessPrimaryDir(projectRoot)
	if _, err := fileutil.Stat(processDir); err != nil {
		return out
	}

	// Misplaced: files whose declared kind doesn't match their directory
	if misplaced, err := storage.FindMisplacedObjectFiles(projectRoot); err == nil && len(misplaced) > 0 {
		out.MisplacedByKind = misplaced
	}

	// Ensure object ID cache is loaded for unmanaged check
	objCache := GetGlobalObjectIDCache()
	_ = TryLoadObjectIDCacheOnly(projectRoot)

	// Stream-backed dirs (audit/, metrics/, etc.) hold tens of thousands of YAML files managed
	// by stream storage, not the object-id cache. Flagging them as "unmanaged" is a false
	// positive: their authoritative count comes from stream storage (same reason we skip them
	// in the disk walk in operational.Run). Skip the unmanaged check for these dirs.
	streamBackedSet := make(map[string]bool, len(streamBackedDirs))
	for _, d := range streamBackedDirs {
		streamBackedSet[d] = true
	}

	// Unmanaged: YAML files on disk that are not in the object ID cache (not managed by CAS/index)
	var unmanaged []string
	err := filepath.Walk(processDir, func(path string, info fileutil.FileInfo, err error) error {
		if err != nil {
			return nil //nolint:nilerr // skip errors during best-effort unmanaged scan
		}
		if info.IsDir() {
			// Do not treat _internal (config/spec/migrations) as process data
			if info.Name() == ocrProcessInternalDir {
				return filepath.SkipDir
			}
			// Skip stream-backed dirs: their objects are managed by stream storage,
			// not the object-id cache, so they would all appear "unmanaged".
			if streamBackedSet[info.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		base := info.Name()
		if !strings.HasSuffix(base, ".yaml") && !strings.HasSuffix(base, ".yml") {
			return nil
		}
		if strings.HasPrefix(base, ".") {
			return nil
		}
		id, _ := objects.ReadIDAndKindFromYAMLFile(path)
		if id == emptyValue {
			return nil
		}
		if _, ok := objCache.Get(id); !ok {
			unmanaged = append(unmanaged, path)
		}
		return nil
	})
	if err == nil && len(unmanaged) > 0 {
		out.UnmanagedFiles = unmanaged
	}

	// Unknown dirs: top-level dirs under .zqk/process that don't map to a known kind
	// Files at root: files (not dirs) directly under .zqk/process
	entries, err := fileutil.ReadDir(processDir)
	if err != nil {
		return out
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() {
			if name == ocrProcessInternalDir || strings.HasPrefix(name, ".") {
				continue
			}
			if objects.GetKindFromDirectory(name) == emptyValue {
				out.UnknownDirs = append(out.UnknownDirs, name)
			}
		} else {
			out.FilesAtRoot = append(out.FilesAtRoot, name)
		}
	}
	sort.Strings(out.UnknownDirs)
	sort.Strings(out.FilesAtRoot)
	return out
}

const (
	defaultMetricsChunkRetentionDays = 14
	maxMetricsChunkRetentionDays     = 365
)

// resolveMetricsChunkRetentionDays returns days of .chunk retention: --metrics-chunk-retention-days if set,
// else brand-prefixed METRICS_CHUNK_RETENTION_DAYS, else defaultMetricsChunkRetentionDays.
func resolveMetricsChunkRetentionDays(cmd *cobra.Command) int {
	if cmd != nil {
		if f := cmd.Flags().Lookup("metrics-chunk-retention-days"); f != nil && f.Changed {
			v, _ := cmd.Flags().GetInt("metrics-chunk-retention-days")
			if v > 0 {
				if v > maxMetricsChunkRetentionDays {
					return maxMetricsChunkRetentionDays
				}
				return v
			}
		}
	}
	if n := config.MaintenanceMetricsChunkRetentionDays().OrDefault(30); n > 0 {
		if n > maxMetricsChunkRetentionDays {
			return maxMetricsChunkRetentionDays
		}
		return n
	}
	return defaultMetricsChunkRetentionDays
}

// recordObjectVolumeMetrics appends a single time-series sample per kind using the
// compact metrics timeseries writer. Best-effort only; failures are logged and do not
// affect the object-count-report command result. After writing, prunes chunk files
// older than retentionDays so .zqk/metrics/object_volume/ does not grow unbounded.
func recordObjectVolumeMetrics(projectRoot string, report *operational.CongruenceReport, logger logging.Logger, retentionDays int) {
	if report == nil {
		return
	}
	metricsDir := filepath.Join(projectRoot, paths.ProjectDataDir, paths.MetricsDir, paths.MetricsObjectVolumeSubdir)
	ts := report.GeneratedAt

	for kind, count := range report.ObjectCountByKind {
		cfg := metrics.TimeSeriesConfig{
			ChunkDuration: time.Hour,
			Dir:           metricsDir,
			Series:        fmt.Sprintf("object_volume/%s", kind),
		}
		w, err := metrics.NewTimeSeriesWriter(cfg)
		if err != nil {
			logging.Fluent(logger).Debug("Object volume metrics: writer init failed").
				Kind(kind).
				WithError(err).
				Log()
			continue
		}
		if err := w.Append(metrics.TimeSeriesPoint{
			Ts:    ts,
			Value: int64(count),
		}); err != nil {
			logging.Fluent(logger).Debug("Object volume metrics: append failed").
				Kind(kind).
				WithError(err).
				Log()
			_ = w.Close()
			continue
		}
		if err := w.Close(); err != nil {
			logging.Fluent(logger).Debug("Object volume metrics: close failed").
				Kind(kind).
				WithError(err).
				Log()
		}
	}

	pruneObjectVolumeChunks(metricsDir, retentionDays, logger)
}

// recordStreamVolumeMetrics appends a time-series sample per high-volume stream kind (e.g. audit_event)
// using the high-volume event cache as the source of truth. Series are written under metrics/stream_volume/
// with series name "stream_volume/<kind>" so they are distinct from CAS-backed object_volume metrics.
// Best-effort only; failures are logged and do not affect the object-count-report command result.
func recordStreamVolumeMetrics(projectRoot string, logger logging.Logger, retentionDays int) {
	if projectRoot == emptyValue {
		return
	}
	cache := storage.GetGlobalHighVolumeEventCache()
	if cache == nil {
		return
	}
	metricsDir := filepath.Join(projectRoot, paths.ProjectDataDir, paths.MetricsDir, paths.MetricsStreamVolumeSubdir)
	ts := time.Now().UTC()

	// Record stream volume for every stream-backed kind (audit_event, scheduler_job, zqk_session, metrics, etc.).
	streamKinds := storage.StreamStorageEnabledKindsList()
	for _, kind := range streamKinds {
		count := cache.CountByKind(kind)
		cfg := metrics.TimeSeriesConfig{
			ChunkDuration: time.Hour,
			Dir:           metricsDir,
			Series:        fmt.Sprintf("stream_volume/%s", kind),
		}
		w, err := metrics.NewTimeSeriesWriter(cfg)
		if err != nil {
			logging.Fluent(logger).Debug("Stream volume metrics: writer init failed").
				Kind(kind).
				WithError(err).
				Log()
			continue
		}
		if err := w.Append(metrics.TimeSeriesPoint{
			Ts:    ts,
			Value: int64(count),
		}); err != nil {
			logging.Fluent(logger).Debug("Stream volume metrics: append failed").
				Kind(kind).
				WithError(err).
				Log()
			_ = w.Close()
			continue
		}
		if err := w.Close(); err != nil {
			logging.Fluent(logger).Debug("Stream volume metrics: close failed").
				Kind(kind).
				WithError(err).
				Log()
		}
	}
	pruneObjectVolumeChunks(metricsDir, retentionDays, logger)
}

// pruneObjectVolumeChunks removes chunk files in dir older than retentionDays (by ModTime).
// Best-effort; errors are logged and do not fail the report.
func writeFilesystemSnapshotSection(f *fileutil.File, snap *operational.FilesystemProjectSnapshot) {
	if snap == nil {
		return
	}
	fmt.Fprintf(f, "--- Filesystem snapshot ---\n")
	if snap.SnapshotScope != emptyValue {
		fmt.Fprintf(f, "snapshot_scope: %s\n", snap.SnapshotScope)
	}
	fmt.Fprintf(f, "generated: %s\n", snap.GeneratedAt)
	fmt.Fprintf(f, "total_files: %d\n", snap.TotalFiles)
	fmt.Fprintf(f, "total_bytes: %d\n", snap.TotalBytes)
	if snap.Volume != nil && snap.Volume.Source != emptyValue {
		fmt.Fprintf(f, "volume (%s): filesystem total_bytes=%d available_bytes=%d free_bytes=%d\n",
			snap.Volume.Source, snap.Volume.TotalBytes, snap.Volume.AvailableBytes, snap.Volume.FreeBytes)
	}
	fmt.Fprintf(f, "top-level buckets by file count (sample):\n")
	type kv struct {
		name  string
		files int64
	}
	var tops []kv
	for n, st := range snap.ByTopLevel {
		tops = append(tops, kv{n, st.Files})
	}
	sort.Slice(tops, func(i, j int) bool {
		if tops[i].files != tops[j].files {
			return tops[i].files > tops[j].files
		}
		return tops[i].name < tops[j].name
	})
	maxShow := 25
	for i, e := range tops {
		if i >= maxShow {
			fmt.Fprintf(f, "  ... (%d more buckets)\n", len(tops)-maxShow)
			break
		}
		fmt.Fprintf(f, "  %s: %d files, %d bytes\n", e.name, e.files, snap.ByTopLevel[e.name].Bytes)
	}
	if len(snap.ZqkByChild) > 0 {
		fmt.Fprintf(f, ".zqk immediate children:\n")
		var zc []kv
		for n, st := range snap.ZqkByChild {
			zc = append(zc, kv{n, st.Files})
		}
		sort.Slice(zc, func(i, j int) bool {
			if zc[i].files != zc[j].files {
				return zc[i].files > zc[j].files
			}
			return zc[i].name < zc[j].name
		})
		for _, e := range zc {
			fmt.Fprintf(f, "  .zqk/%s: %d files, %d bytes\n", e.name, e.files, snap.ZqkByChild[e.name].Bytes)
		}
	}
	fmt.Fprintf(f, "\n")
}

func int64OrZero(u uint64) int64 {
	const maxInt64 = 1<<63 - 1
	if u > uint64(maxInt64) {
		return maxInt64
	}
	return int64(u)
}

func sanitizeFSSeriesPart(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	out := strings.Trim(b.String(), "_")
	if out == emptyValue {
		return "unknown"
	}
	return out
}

func recordFilesystemSnapshotMetrics(projectRoot string, snap *operational.FilesystemProjectSnapshot, logger logging.Logger, retentionDays int) {
	if snap == nil || projectRoot == emptyValue {
		return
	}
	metricsDir := filepath.Join(projectRoot, paths.ProjectDataDir, paths.MetricsDir, paths.MetricsFilesystemSnapshotSubdir)
	ts, err := time.Parse(time.RFC3339Nano, snap.GeneratedAt)
	if err != nil {
		ts = time.Now().UTC()
	}

	appendFSPoint := func(series string, val int64) {
		cfg := metrics.TimeSeriesConfig{
			ChunkDuration: time.Hour,
			Dir:           metricsDir,
			Series:        series,
		}
		w, werr := metrics.NewTimeSeriesWriter(cfg)
		if werr != nil {
			logging.Fluent(logger).Debug("Filesystem snapshot metrics: writer init failed").
				String("series", series).
				WithError(werr).
				Log()
			return
		}
		if err := w.Append(metrics.TimeSeriesPoint{Ts: ts, Value: val}); err != nil {
			logging.Fluent(logger).Debug("Filesystem snapshot metrics: append failed").
				String("series", series).
				WithError(err).
				Log()
			_ = w.Close()
			return
		}
		if err := w.Close(); err != nil {
			logging.Fluent(logger).Debug("Filesystem snapshot metrics: close failed").
				String("series", series).
				WithError(err).
				Log()
		}
	}

	appendFSPoint("filesystem_snapshot/total_files", snap.TotalFiles)
	appendFSPoint("filesystem_snapshot/total_bytes", snap.TotalBytes)
	if snap.Volume != nil {
		appendFSPoint("filesystem_snapshot/volume_total_bytes", int64OrZero(snap.Volume.TotalBytes))
		appendFSPoint("filesystem_snapshot/volume_available_bytes", int64OrZero(snap.Volume.AvailableBytes))
	}
	for child, st := range snap.ZqkByChild {
		if st == nil {
			continue
		}
		series := fmt.Sprintf("filesystem_snapshot/zqk_%s/files", sanitizeFSSeriesPart(child))
		appendFSPoint(series, st.Files)
	}

	pruneObjectVolumeChunks(metricsDir, retentionDays, logger)
}

func pruneObjectVolumeChunks(dir string, retentionDays int, logger logging.Logger) {
	if retentionDays <= 0 {
		return
	}
	cutoff := time.Now().UTC().Add(-time.Duration(retentionDays) * 24 * time.Hour)
	entries, err := fileutil.ReadDir(dir)
	if err != nil {
		if fileutil.IsNotExist(err) {
			return
		}
		logging.Fluent(logger).Debug("Object volume prune: read dir failed").Dir(dir).WithError(err).Log()
		return
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if filepath.Ext(e.Name()) != ".chunk" {
			continue
		}
		path := filepath.Join(dir, e.Name())
		info, err := fileutil.Stat(path)
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			if err := fileutil.Remove(path); err != nil {
				logging.Fluent(logger).Debug("Object volume prune: remove failed").Path(path).WithError(err).Log()
			}
		}
	}
}

func resolveZQKBin(projectRoot string) string {
	adminBinName := "zqk-admin"
	for _, dir := range []string{filepath.Join(projectRoot, "bin"), projectRoot} {
		p := filepath.Join(dir, adminBinName)
		if info, err := fileutil.Stat(p); err == nil && info.Mode().IsRegular() {
			return p
		}
	}
	return adminBinName
}

func writeReportFile(path string, report *operational.CongruenceReport, cacheStatus CacheStatus, integrity ProcessIntegrity, fsSnap *operational.FilesystemProjectSnapshot) error {
	f, err := fileutil.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	fmt.Fprintf(f, "=== Object count report ===\n")
	fmt.Fprintf(f, "generated: %s\n", report.GeneratedAt.Format(time.RFC3339))
	fmt.Fprintf(f, "project_root: %s\n", report.ProjectRoot)
	fmt.Fprintf(f, "output: %s\n\n", path)

	writeCacheStatusSection(f, cacheStatus)
	fmt.Fprintf(f, "\n")

	writeProcessIntegritySections(f, integrity)
	if hasIntegrityFindings(integrity) {
		fmt.Fprintf(f, "\n")
	}

	if len(report.StrayDirs) > 0 {
		fmt.Fprintf(f, "--- Stray process dirs (unmapped) ---\n")
		for _, d := range report.StrayDirs {
			fmt.Fprintf(f, "  %s\n", d)
		}
		fmt.Fprintf(f, "\n")
	}

	fmt.Fprintf(f, "--- Totals ---\n")
	fmt.Fprintf(f, "total_disk: %d\n", report.TotalDisk)
	fmt.Fprintf(f, "total_object: %d\n", report.TotalObject)
	if report.TotalInternal > 0 {
		fmt.Fprintf(f, "total_internal: %d\n", report.TotalInternal)
	}
	fmt.Fprintf(f, "\n")
	writeInterpretation(f, report)
	fmt.Fprintf(f, "\n")

	fmt.Fprintf(f, "--- Disk count by dir ---\n")
	var dirs []string
	for d := range report.DiskCountByDir {
		dirs = append(dirs, d)
	}
	sort.Strings(dirs)
	for _, d := range dirs {
		fmt.Fprintf(f, "  %s: %d\n", d, report.DiskCountByDir[d])
	}
	fmt.Fprintf(f, "\n")

	fmt.Fprintf(f, "--- Object count by kind ---\n")
	var kinds []string
	for k := range report.ObjectCountByKind {
		kinds = append(kinds, k)
	}
	sort.Strings(kinds)
	for _, k := range kinds {
		fmt.Fprintf(f, "  %s: %d\n", k, report.ObjectCountByKind[k])
	}
	fmt.Fprintf(f, "\n")

	if len(report.InternalCountByKind) > 0 {
		fmt.Fprintf(f, "--- Internal count by kind ---\n")
		var ikinds []string
		for k := range report.InternalCountByKind {
			ikinds = append(ikinds, k)
		}
		sort.Strings(ikinds)
		for _, k := range ikinds {
			fmt.Fprintf(f, "  %s: %d\n", k, report.InternalCountByKind[k])
		}
		fmt.Fprintf(f, "\n")
	}

	fmt.Fprintf(f, "--- Disparity by dir (disk - object) ---\n")
	for _, d := range dirs {
		disp := report.DisparityByDir[d]
		fmt.Fprintf(f, "  %s: %d\n", d, disp)
	}
	if len(report.StreamBackedDirs) > 0 {
		fmt.Fprintf(f, "  (Stream-backed dirs: object count from stream storage; disk = legacy YAML in %s/<dir>. Negative disparity there means more objects in stream than legacy files.)\n", paths.ProcessDir)
	}
	fmt.Fprintf(f, "\n")

	if len(report.LegacyStreamBackedByDir) > 0 {
		var legacyDirs []string
		for d := range report.LegacyStreamBackedByDir {
			legacyDirs = append(legacyDirs, d)
		}
		sort.Strings(legacyDirs)
		var total int
		for _, n := range report.LegacyStreamBackedByDir {
			total += n
		}
		fmt.Fprintf(f, "--- Legacy pre-migration files (stream-backed dirs) ---\n")
		fmt.Fprintf(f, "  %d total legacy YAML file(s) remain in stream-backed directories.\n", total)
		fmt.Fprintf(f, "  These predate stream-storage migration; stream is authoritative; disk files can be removed.\n")
		for _, d := range legacyDirs {
			fmt.Fprintf(f, "  %s: %d file(s)\n", d, report.LegacyStreamBackedByDir[d])
		}
		fmt.Fprintf(f, "%s", paths.RewriteCanonicalCLIInvocations("  Cleanup: zqk system migrate-legacy-to-stream --kind <kind> --remove-legacy\n"))
		fmt.Fprintf(f, "           scripts/delete_unmanaged_audit_yaml.py --execute  (audit/metrics)\n")
		fmt.Fprintf(f, "\n")
	}

	if len(report.Alerts) > 0 {
		fmt.Fprintf(f, "--- Alerts ---\n")
		for _, a := range report.Alerts {
			fmt.Fprintf(f, "  %s\n", a)
		}
		fmt.Fprintf(f, "\n")
	}
	if fsSnap != nil {
		writeFilesystemSnapshotSection(f, fsSnap)
	}
	fmt.Fprintf(f, "--- When counts are off: investigate ---\n")
	fmt.Fprintf(f, "%s", paths.RewriteCanonicalCLIInvocations(fmt.Sprintf("  See %s and run zqk system retention-status.\n", filepath.Join(paths.ProcessDir, "observability", "RETENTION_TARGET_VISIBILITY_GAP.md"))))
	if len(report.StreamBackedDirs) > 0 {
		fmt.Fprintf(f, "  Stream-backed dirs (object count from stream): legacy YAML in %s/<dir> is not the source of truth;\n", paths.ProcessDir)
		fmt.Fprintf(f, "  you may prune legacy files or leave them; see %s.\n", filepath.Join(paths.DocsDir, "architecture", "HIGH_VOLUME_STORAGE_DEPRECATION.md"))
	}
	return nil
}

func hasIntegrityFindings(integrity ProcessIntegrity) bool {
	if len(integrity.MisplacedByKind) > 0 || len(integrity.UnmanagedFiles) > 0 ||
		len(integrity.UnknownDirs) > 0 || len(integrity.FilesAtRoot) > 0 {
		return true
	}
	return false
}

func writeProcessIntegritySections(f *fileutil.File, integrity ProcessIntegrity) {
	if len(integrity.MisplacedByKind) > 0 {
		fmt.Fprintf(f, "--- Misplaced files (wrong directory for kind) ---\n")
		var kinds []string
		for k := range integrity.MisplacedByKind {
			kinds = append(kinds, k)
		}
		sort.Strings(kinds)
		for _, kind := range kinds {
			filePaths := integrity.MisplacedByKind[kind]
			fmt.Fprintf(f, "  kind %s: %d file(s)\n", kind, len(filePaths))
			for _, p := range filePaths {
				fmt.Fprintf(f, "    %s\n", p)
			}
		}
		fmt.Fprintf(f, "\n")
	}
	if len(integrity.UnmanagedFiles) > 0 {
		fmt.Fprintf(f, "--- Unmanaged files (YAML on disk not in object ID cache) ---\n")
		fmt.Fprintf(f, "  %d file(s) not managed by CAS/index\n", len(integrity.UnmanagedFiles))
		for _, p := range integrity.UnmanagedFiles {
			fmt.Fprintf(f, "    %s\n", p)
		}
		fmt.Fprintf(f, "\n")
	}
	if len(integrity.UnknownDirs) > 0 || len(integrity.FilesAtRoot) > 0 {
		fmt.Fprintf(f, "--- Unknown dirs and trash (%s) ---\n", paths.ProcessDir)
		if len(integrity.UnknownDirs) > 0 {
			fmt.Fprintf(f, "  unknown_dirs (no kind mapping): %v\n", integrity.UnknownDirs)
		}
		if len(integrity.FilesAtRoot) > 0 {
			fmt.Fprintf(f, "  files_at_root: %v\n", integrity.FilesAtRoot)
		}
		fmt.Fprintf(f, "\n")
	}
}

func writeCacheStatusSection(f *fileutil.File, s CacheStatus) {
	fmt.Fprintf(f, "--- Cache status ---\n")
	fmt.Fprintf(f, "  object_id_cache: path=%s loaded=%v", s.ObjectIDCache.Path, s.ObjectIDCache.Loaded)
	if s.ObjectIDCache.Loaded {
		fmt.Fprintf(f, " total=%d", s.ObjectIDCache.Total)
		if len(s.ObjectIDCache.CountByKind) > 0 {
			fmt.Fprintf(f, " count_by_kind=%v", s.ObjectIDCache.CountByKind)
		}
	}
	fmt.Fprintf(f, "\n")
	fmt.Fprintf(f, "  reverse_reference_index: path=%s loaded=%v", s.ReverseReferenceIndex.Path, s.ReverseReferenceIndex.Loaded)
	if s.ReverseReferenceIndex.Loaded {
		fmt.Fprintf(f, " referenced_id_count=%d", s.ReverseReferenceIndex.ReferencedIDCount)
	}
	fmt.Fprintf(f, "\n")
}

// highCountThreshold: above this, report urges cleanup (retention/aggregation or delete script).
const highCountThreshold = 10000

// writeInterpretation writes a short explanation of totals and large gaps so the report is self-explanatory.
func writeInterpretation(f *fileutil.File, report *operational.CongruenceReport) {
	fmt.Fprintf(f, "--- Interpretation ---\n")
	fmt.Fprintf(f, "  total_disk:   count of .yaml files under %s (one file per object or legacy).\n", paths.ProcessDir)
	fmt.Fprintf(f, "  total_object: sum of per-kind counts (CAS index or stream registry for stream-backed kinds).\n")
	fmt.Fprintf(f, "%s", paths.RewriteCanonicalCLIInvocations("  total_internal: sum of per-kind counts from 'zqk internal count' (in-memory view).\n"))
	if len(report.StreamBackedDirs) > 0 {
		fmt.Fprintf(f, "  Stream-backed kinds (audit_event, change_journal_entry, scheduler_job, metrics, etc.): object count\n")
		fmt.Fprintf(f, "  comes from stream storage; disk count is legacy YAML in %s/<dir>. Negative disparity for those\n", paths.ProcessDir)
		fmt.Fprintf(f, "  dirs is expected (more in stream than legacy files). Legacy files can be pruned or left as-is; stream is source of truth.\n")
	}
	if report.TotalDisk > highCountThreshold || report.TotalObject > highCountThreshold {
		fmt.Fprintf(f, "  HIGH TOTAL: %d disk / %d object. To reduce: run retention and audit aggregation jobs\n", report.TotalDisk, report.TotalObject)
		fmt.Fprintf(f, "  (scheduler SCH-002, SCH-020); or one-time cleanup: scripts/delete_unmanaged_audit_yaml.py --execute.\n")
		fmt.Fprintf(f, "  See %s and %s.\n", filepath.Join(paths.DocsDir, "architecture", "OBJECT_COUNT_SELF_MAINTENANCE.md"), filepath.Join(paths.ProcessDir, "observability", "OBJECT_COUNT_MANAGEMENT.md"))
	}
	gap := report.TotalObject - report.TotalDisk
	if gap > 1000 {
		fmt.Fprintf(f, "  Large gap (total_object - total_disk = %d): usually CAS index bloat or stream-backed counts.\n", gap)
		fmt.Fprintf(f, "  Dirs like metrics/ and audit/ have stream-backed kinds (object from stream); disk is legacy YAML.\n")
		fmt.Fprintf(f, "  If not stream-backed: when files are pruned/aggregated, index entries may not be removed. Run CAS\n")
		fmt.Fprintf(f, "  reconcile or ensure retention/aggregation removes index entries when deleting files.\n")
	} else if gap < -1000 {
		fmt.Fprintf(f, "  Large gap (total_disk - total_object = %d): more files than index/stream entries.\n", -gap)
		fmt.Fprintf(f, "  If you recently deleted files manually (e.g. rm), object count may be from stale cache;\n")
		fmt.Fprintf(f, "%s", paths.RewriteCanonicalCLIInvocations("  run 'zqk system check' to refresh caches and CAS indexes, or use --no-cache for this report.\n"))
		fmt.Fprintf(f, "  Otherwise run CAS reconcile to repopulate indexes from disk, or investigate orphans.\n")
	} else {
		fmt.Fprintf(f, "  Small gap between total_object and total_disk is normal (e.g. _internal, multi-object YAML, stream-backed legacy dirs).\n")
	}
	// Suggest retention when high-volume dirs have large disparity (audit_event/mcp_session explosion).
	if d := report.DisparityByDir[streamDirAudit]; d > 5000 {
		fmt.Fprintf(f, "%s", paths.RewriteCanonicalCLIInvocations(fmt.Sprintf("  audit/ disparity=%d: to keep audit_event under control, run 'zqk system retention-tolerance --kind audit_event'\n", d)))
		fmt.Fprintf(f, "  or ensure the retention_tolerance scheduler job runs often; see %s.\n", filepath.Join(paths.ProcessInternalConfigsDir, "retention_tolerance.yaml"))
	}
	if d := report.DisparityByDir[streamDirMCPSessions]; d > 500 {
		fmt.Fprintf(f, "%s", paths.RewriteCanonicalCLIInvocations(fmt.Sprintf("  mcp_sessions/ disparity=%d: run 'zqk system retention-tolerance --kind mcp_session' or schedule it more frequently.\n", d)))
	}
}

// ObjectCountReportEntry is one run's metadata for the reports index (discovery and aggregation).
type ObjectCountReportEntry struct {
	Path        string `json:"path"`         // basename of the report JSON file
	GeneratedAt string `json:"generated_at"` // RFC3339
	TotalDisk   int    `json:"total_disk"`
	TotalObject int    `json:"total_object"`
}

// ObjectCountReportsIndex is the durable index of object-count-report runs for dashboards and aggregation.
type ObjectCountReportsIndex struct {
	Latest  *ObjectCountReportEntry  `json:"latest,omitempty"`
	Reports []ObjectCountReportEntry `json:"reports,omitempty"` // newest first, capped at reportsIndexMaxEntries
}

const reportsIndexMaxEntries = 100

// updateReportsIndex updates .zqk/logs/reports/reports_index.json with this run (best-effort).
func updateReportsIndex(reportDir, jsonPath string, report *operational.CongruenceReport, logger logging.Logger) {
	entry := ObjectCountReportEntry{
		Path:        filepath.Base(jsonPath),
		GeneratedAt: zqktime.FormatRFC3339NanoUTC(report.GeneratedAt),
		TotalDisk:   report.TotalDisk,
		TotalObject: report.TotalObject,
	}
	indexPath := filepath.Join(reportDir, paths.ReportsIndexFile)
	var index ObjectCountReportsIndex
	if data, err := fileutil.ReadFile(indexPath); err == nil {
		_ = json.Unmarshal(data, &index)
	}
	index.Latest = &entry
	// Prepend and cap
	newReports := make([]ObjectCountReportEntry, 0, len(index.Reports)+1)
	newReports = append(newReports, entry)
	for _, e := range index.Reports {
		if e.Path != entry.Path {
			newReports = append(newReports, e)
		}
		if len(newReports) >= reportsIndexMaxEntries {
			break
		}
	}
	index.Reports = newReports
	data, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		logging.Fluent(logger).Debug("Reports index marshal failed").WithError(err).Log()
		return
	}
	if err := fileutil.WriteFile(indexPath, data, paths.FilePerm600); err != nil {
		logging.Fluent(logger).Debug("Reports index write failed").Path(indexPath).WithError(err).Log()
	}
}

// ObjectCountDashboardSnapshot is the dashboard-friendly subset of a congruence report (single file for polling/aggregation).
type ObjectCountDashboardSnapshot struct {
	GeneratedAt        string         `json:"generated_at"`
	ProjectRoot        string         `json:"project_root,omitempty"`
	ObjectCountByKind  map[string]int `json:"object_count_by_kind"`
	TotalDisk          int            `json:"total_disk"`
	TotalObject        int            `json:"total_object"`
	TotalInternal      int            `json:"total_internal,omitempty"`
	Alerts             []string       `json:"alerts,omitempty"`
	KindsWithDisparity []string       `json:"kinds_with_disparity,omitempty"`
	// Set when object-count-report runs with --include-filesystem-snapshot.
	FilesystemSnapshotTotalFiles *int64 `json:"filesystem_snapshot_total_files,omitempty"`
	FilesystemSnapshotZqkFiles   *int64 `json:"filesystem_snapshot_zqk_files,omitempty"`
}

// writeDashboardSnapshot writes .zqk/logs/reports/latest_object_count_snapshot.json for dashboards and aggregators (best-effort).
func writeDashboardSnapshot(reportDir string, report *operational.CongruenceReport, logger logging.Logger, fsProjectSnap *operational.FilesystemProjectSnapshot) {
	if report == nil {
		return
	}
	snap := ObjectCountDashboardSnapshot{
		GeneratedAt:        zqktime.FormatRFC3339NanoUTC(report.GeneratedAt),
		ProjectRoot:        report.ProjectRoot,
		ObjectCountByKind:  report.ObjectCountByKind,
		TotalDisk:          report.TotalDisk,
		TotalObject:        report.TotalObject,
		TotalInternal:      report.TotalInternal,
		Alerts:             report.Alerts,
		KindsWithDisparity: report.KindsWithDisparity,
	}
	if fsProjectSnap != nil {
		tf := fsProjectSnap.TotalFiles
		snap.FilesystemSnapshotTotalFiles = &tf
		if b := fsProjectSnap.ByTopLevel[paths.ProjectDataDir]; b != nil {
			zf := b.Files
			snap.FilesystemSnapshotZqkFiles = &zf
		}
	}
	path := filepath.Join(reportDir, paths.LatestObjectCountSnapshotFile)
	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		logging.Fluent(logger).Debug("Dashboard snapshot marshal failed").WithError(err).Log()
		return
	}
	if err := fileutil.WriteFile(path, data, paths.FilePerm600); err != nil {
		logging.Fluent(logger).Debug("Dashboard snapshot write failed").Path(path).WithError(err).Log()
	}
}

func writeReportJSON(path string, report *operational.CongruenceReport, cacheStatus CacheStatus, integrity ProcessIntegrity, fsSnap *operational.FilesystemProjectSnapshot) error {
	out := struct {
		*operational.CongruenceReport
		CacheStatus               CacheStatus                            `json:"cache_status"`
		ProcessIntegrity          ProcessIntegrity                       `json:"process_integrity,omitempty"`
		FilesystemProjectSnapshot *operational.FilesystemProjectSnapshot `json:"filesystem_project_snapshot,omitempty"`
	}{report, cacheStatus, integrity, fsSnap}
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	return fileutil.WriteFile(path, data, paths.FilePerm600)
}

func emitCongruenceAlertEvent(ctx context.Context, projectRoot string, storageProvider storage.ObjectStorageProvider, report *operational.CongruenceReport, logger logging.Logger) {
	auditRouter := coordination.NewStorageAuditRouter(projectRoot, storageProvider)
	loggingRouter := &coordination.DefaultLoggingRouter{}

	auditMetadata := map[string]any{
		objects.FieldKeyEventType: "operational_congruence_alert",
		objects.FieldKeyOperation: "Object count congruence report",
		ocrKeyTotalDisk:           report.TotalDisk,
		ocrKeyTotalObject:         report.TotalObject,
		ocrKeyAlertCount:          len(report.Alerts),
		ocrKeyDirsWithDisparity:   report.KindsWithDisparity,
		"disparity_by_dir":        report.DisparityByDir,
		objects.FieldKeySeverity:  ocrSeverityWarning,
	}
	loggingFields := []coordination.LoggingField{
		{Key: ocrKeyTotalDisk, Value: report.TotalDisk},
		{Key: ocrKeyTotalObject, Value: report.TotalObject},
		{Key: ocrKeyAlertCount, Value: len(report.Alerts)},
		{Key: ocrKeyDirsWithDisparity, Value: report.KindsWithDisparity},
	}
	eventData := &coordination.EventData{
		LoggingFields: loggingFields,
		AuditMetadata: auditMetadata,
		MetricsData:   map[string]any{ocrKeyTotalDisk: report.TotalDisk, ocrKeyTotalObject: report.TotalObject, "disparity_dirs": report.KindsWithDisparity},
	}
	operationID := fmt.Sprintf("congruence_report_%d", time.Now().Unix())
	eventCtx := coordination.NewEventContext(operationID, "operational_congruence", ocrSeverityWarning).
		WithEventData(eventData).
		WithContext(ctx).
		WithChannels(true, true, true, false)
	_ = loggingRouter.Emit(ctx, eventCtx)
	_ = auditRouter.Emit(ctx, eventCtx)
}
