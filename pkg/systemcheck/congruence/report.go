package congruence

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"time"

	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/operational"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqktime"
)

// WriteReportFile formats and writes the human-readable text report.
func WriteReportFile(path string, report *operational.CongruenceReport, cacheStatus CacheStatus, integrity ProcessIntegrity, fsSnap *operational.FilesystemProjectSnapshot) error {
	f, err := fileutil.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	writeReportHeader(f, path, report)
	writeCacheStatusSection(f, cacheStatus)
	fmt.Fprintf(f, "\n")

	writeProcessIntegritySections(f, integrity)
	if hasIntegrityFindings(integrity) {
		fmt.Fprintf(f, "\n")
	}

	writeReportCounts(f, report)
	writeReportDisparities(f, report)
	writeReportAlertsAndSnapshots(f, report, fsSnap)
	return nil
}

func writeReportHeader(f *fileutil.File, path string, report *operational.CongruenceReport) {
	fmt.Fprintf(f, "=== Object count report ===\n")
	fmt.Fprintf(f, "generated: %s\n", report.GeneratedAt.Format(time.RFC3339))
	fmt.Fprintf(f, "project_root: %s\n", report.ProjectRoot)
	fmt.Fprintf(f, "output: %s\n\n", path)
}

func writeReportCounts(f *fileutil.File, report *operational.CongruenceReport) {
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
}

func writeReportDisparities(f *fileutil.File, report *operational.CongruenceReport) {
	fmt.Fprintf(f, "--- Disparity by dir (disk - object) ---\n")
	var dirs []string
	for d := range report.DiskCountByDir {
		dirs = append(dirs, d)
	}
	sort.Strings(dirs)
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
}

func writeReportAlertsAndSnapshots(f *fileutil.File, report *operational.CongruenceReport, fsSnap *operational.FilesystemProjectSnapshot) {
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
}

func hasIntegrityFindings(integrity ProcessIntegrity) bool {
	return len(integrity.MisplacedByKind) > 0 || len(integrity.UnmanagedFiles) > 0 ||
		len(integrity.UnknownDirs) > 0 || len(integrity.FilesAtRoot) > 0
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
	if report.TotalDisk > HighCountThreshold || report.TotalObject > HighCountThreshold {
		fmt.Fprintf(f, "  HIGH TOTAL: %d disk / %d object. To reduce: run retention and audit aggregation jobs\n", report.TotalDisk, report.TotalObject)
		fmt.Fprintf(f, "  (scheduler SCH-002, SCH-020); or one-time cleanup: scripts/delete_unmanaged_audit_yaml.py --execute.\n")
		fmt.Fprintf(f, "  See %s and %s.\n", filepath.Join(paths.DocsDir, "architecture", "OBJECT_COUNT_SELF_MAINTENANCE.md"), filepath.Join(paths.ProcessDir, "observability", "OBJECT_COUNT_MANAGEMENT.md"))
	}
	gap := report.TotalObject - report.TotalDisk
	switch {
	case gap > 1000:
		fmt.Fprintf(f, "  Large gap (total_object - total_disk = %d): usually CAS index bloat or stream-backed counts.\n", gap)
		fmt.Fprintf(f, "  Dirs like metrics/ and audit/ have stream-backed kinds (object from stream); disk is legacy YAML.\n")
		fmt.Fprintf(f, "  If not stream-backed: when files are pruned/aggregated, index entries may not be removed. Run CAS\n")
		fmt.Fprintf(f, "  reconcile or ensure retention/aggregation removes index entries when deleting files.\n")
	case gap < -1000:
		fmt.Fprintf(f, "  Large gap (total_disk - total_object = %d): more files than index/stream entries.\n", -gap)
		fmt.Fprintf(f, "  If you recently deleted files manually (e.g. rm), object count may be from stale cache;\n")
		fmt.Fprintf(f, "%s", paths.RewriteCanonicalCLIInvocations("  run 'zqk system check' to refresh caches and CAS indexes, or use --no-cache for this report.\n"))
		fmt.Fprintf(f, "  Otherwise run CAS reconcile to repopulate indexes from disk, or investigate orphans.\n")
	default:
		fmt.Fprintf(f, "  Small gap between total_object and total_disk is normal (e.g. _internal, multi-object YAML, stream-backed legacy dirs).\n")
	}
	if d := report.DisparityByDir[StreamDirAudit]; d > 5000 {
		fmt.Fprintf(f, "%s", paths.RewriteCanonicalCLIInvocations(fmt.Sprintf("  audit/ disparity=%d: to keep audit_event under control, run 'zqk system retention-tolerance --kind audit_event'\n", d)))
		fmt.Fprintf(f, "  or ensure the retention_tolerance scheduler job runs often; see %s.\n", filepath.Join(paths.ProcessInternalConfigsDir, "retention_tolerance.yaml"))
	}
	if d := report.DisparityByDir[StreamDirMCPSessions]; d > 500 {
		fmt.Fprintf(f, "%s", paths.RewriteCanonicalCLIInvocations(fmt.Sprintf("  mcp_sessions/ disparity=%d: run 'zqk system retention-tolerance --kind mcp_session' or schedule it more frequently.\n", d)))
	}
}

func writeFilesystemSnapshotSection(f *fileutil.File, snap *operational.FilesystemProjectSnapshot) {
	if snap == nil {
		return
	}
	fmt.Fprintf(f, "--- Filesystem snapshot ---\n")
	if snap.SnapshotScope != "" {
		fmt.Fprintf(f, "snapshot_scope: %s\n", snap.SnapshotScope)
	}
	fmt.Fprintf(f, "generated: %s\n", snap.GeneratedAt)
	fmt.Fprintf(f, "total_files: %d\n", snap.TotalFiles)
	fmt.Fprintf(f, "total_bytes: %d\n", snap.TotalBytes)
	if snap.Volume != nil && snap.Volume.Source != "" {
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

// UpdateReportsIndex updates .zqk/logs/reports/reports_index.json with this run (best-effort).
func UpdateReportsIndex(reportDir, jsonPath string, report *operational.CongruenceReport, logger logging.Logger) {
	entry := ObjectCountReportEntry{
		Path:        filepath.Base(jsonPath),
		GeneratedAt: zqktime.FormatRFC3339NanoUTC(report.GeneratedAt),
		TotalDisk:   report.TotalDisk,
		TotalObject: report.TotalObject,
	}
	indexPath := filepath.Join(reportDir, paths.ReportsIndexFile)
	var index ObjectCountReportsIndex
	if data, readErr := fileutil.ReadFile(indexPath); readErr == nil {
		if unmarshalErr := json.Unmarshal(data, &index); unmarshalErr != nil {
			// Best-effort index recovery
		}
	}
	index.Latest = &entry
	newReports := make([]ObjectCountReportEntry, 0, len(index.Reports)+1)
	newReports = append(newReports, entry)
	for _, e := range index.Reports {
		if e.Path != entry.Path {
			newReports = append(newReports, e)
		}
		if len(newReports) >= ReportsIndexMaxEntries {
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

// WriteDashboardSnapshot writes .zqk/logs/reports/latest_object_count_snapshot.json for dashboards and aggregators.
func WriteDashboardSnapshot(reportDir string, report *operational.CongruenceReport, logger logging.Logger, fsProjectSnap *operational.FilesystemProjectSnapshot) {
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

// WriteReportJSON writes the congruence report along with cache status, process integrity, and snapshot to JSON.
func WriteReportJSON(path string, report *operational.CongruenceReport, cacheStatus CacheStatus, integrity ProcessIntegrity, fsSnap *operational.FilesystemProjectSnapshot) error {
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
