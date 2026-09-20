// Package operational provides operational congruence reporting: disk vs index vs
// internal counts, disparity detection, and hooks for metrics and alerts.
package operational

import (
	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/execwrap"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"

	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/appledouble"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
)

const (
	// DefaultInternalCountTimeout is the timeout for the internal count subprocess.
	DefaultInternalCountTimeout = 30 * time.Second
	// DisparityAlertThreshold: when disk - object count exceeds this for a dir, we alert.
	DisparityAlertThreshold = 50

	yamlExt                   = ".yaml"
	internalDirName           = "_internal"
	hiddenPrefix              = "."
	internalCountTimeoutError = "internal count timed out"

	errProjectRootRequired         = "project root required"
	errWalkDirFmt                  = "walk %s: %w"
	errLoadFieldRegistryFmt        = "load field registry: %w"
	errGetAllKindsFmt              = "get all kinds: %w"
	errDisparityAlertFmt           = "dir %s: disk=%d object=%d disparity=%d (scheduler maintenance keeps counts in policy when running; if daemon not started, run 'zqk system ensure-retention-jobs' and start scheduler)"
	errStrayProcessDirFmt          = "stray process dir %s: disk=%d YAML, no kind mapping (use GetDirectoryFromKind; kind decision lives in decisions/)"
	errInternalCountExecFmt        = "internal count: %w (output: %s)"
	errParseInternalCountOutputFmt = "parse internal count output: %w"
	internalCountCmdSubcommand     = "internal"
	internalCountCmdAction         = "count"
	internalCountCmdFormatFlag     = "--format"
	internalCountCmdFormatJSON     = "json"
	emptyValue                     = ""
)

// CongruenceReport holds disk, object, and internal counts and derived disparity/alert data.
type CongruenceReport struct {
	GeneratedAt         time.Time      `json:"generated_at"`
	ProjectRoot         string         `json:"project_root"`
	DiskCountByDir      map[string]int `json:"disk_count_by_dir"`                // top-level dir under .zqk/process -> YAML file count (0 for stream-backed dirs; authoritative count from stream)
	ObjectCountByKind   map[string]int `json:"object_count_by_kind"`             // kind -> object count (index)
	InternalCountByKind map[string]int `json:"internal_count_by_kind,omitempty"` // kind -> internal count (subprocess)
	ObjectCountByDir    map[string]int `json:"object_count_by_dir"`              // dir -> sum of object counts for kinds in that dir
	DisparityByDir      map[string]int `json:"disparity_by_dir"`                 // dir -> disk - object (positive = more files than index)
	TotalDisk           int            `json:"total_disk"`
	TotalObject         int            `json:"total_object"`
	TotalInternal       int            `json:"total_internal,omitempty"`
	Alerts              []string       `json:"alerts,omitempty"`
	KindsWithDisparity  []string       `json:"kinds_with_disparity,omitempty"` // dirs with disparity > threshold
	// StrayDirs: top-level process dirs that have YAML but do not map to a kind
	// (e.g. .zqk/process/decision/ vs canonical decisions/). Alert at count >= 1.
	StrayDirs []string `json:"stray_dirs,omitempty"`
	// StreamBackedDirs: dirs where object count comes from stream storage; disk count is legacy YAML in .zqk/process/<dir>. Negative disparity there is expected.
	StreamBackedDirs []string `json:"stream_backed_dirs,omitempty"`
	// LegacyStreamBackedByDir: count of legacy YAML files on disk in each stream-backed dir.
	// These files predate stream-storage migration. DiskCountByDir for these dirs is 0 (disk is
	// not authoritative); this field gives operators visibility into how many files need cleanup.
	// Use 'zqk system migrate-legacy-to-stream --kind <kind> --remove-legacy' to merge and remove,
	// or 'scripts/delete_unmanaged_audit_yaml.py --execute' for audit/metrics dirs.
	LegacyStreamBackedByDir map[string]int `json:"legacy_stream_backed_by_dir,omitempty"`
}

// RunOptions configures Run.
type RunOptions struct {
	ProjectRoot        string
	IncludeInternal    bool   // run internal count subprocess
	ZQKBin             string // path to zqk binary for internal count; empty = auto-detect
	InternalTimeout    time.Duration
	DisparityThreshold int // alert when disk - object > this; 0 = use DisparityAlertThreshold
	// ObjectCountByKindFromCache: when non-nil, use these counts instead of calling storage.Count per kind (avoids 80+ slow Count calls on large repos).
	ObjectCountByKindFromCache map[string]int
	// StreamBackedDirs: when set, dirs in this set have object count from stream storage; disk is legacy. Used to annotate report (negative disparity expected).
	StreamBackedDirs []string
}

// Run builds a congruence report: counts YAML files per dir, object count per kind (then by dir),
// optionally internal count via subprocess, computes disparity and alerts.
func Run(ctx context.Context, storageProvider storage.ObjectStorageProvider, secCtx *pkgctx.SecurityContext, opts RunOptions) (*CongruenceReport, error) {
	if opts.ProjectRoot == emptyValue {
		return nil, errors.New(errProjectRootRequired)
	}
	processDir := datacell.ProcessPrimaryDir(opts.ProjectRoot)
	if _, err := fileutil.Stat(processDir); err != nil {
		return nil, errfmt.Errorf("%s not found: %w", paths.ProcessDir, err)
	}

	report := &CongruenceReport{
		GeneratedAt:        time.Now().UTC(),
		ProjectRoot:        opts.ProjectRoot,
		DiskCountByDir:     make(map[string]int),
		ObjectCountByKind:  make(map[string]int),
		ObjectCountByDir:   make(map[string]int),
		DisparityByDir:     make(map[string]int),
		Alerts:             nil,
		KindsWithDisparity: nil,
	}

	// 1) Disk count per top-level dir.
	// Stream-backed dirs (audit/, metrics/, etc.) can hold tens of thousands of YAML files
	// across nested monthly subdirs. Walking them is expensive (10-30s) and not actionable
	// because their authoritative count comes from stream storage, not disk. Skip the walk
	// for those dirs; disparity for them is annotated as expected in the report.
	streamBackedDirSet := make(map[string]bool, len(opts.StreamBackedDirs))
	for _, d := range opts.StreamBackedDirs {
		streamBackedDirSet[d] = true
	}

	entries, err := fileutil.ReadDir(processDir)
	if err != nil {
		return nil, errfmt.Errorf("read %s: %w", paths.ProcessDir, err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dirName := e.Name()
		if dirName == internalDirName {
			// Schema/config plane is not instance inventory. Hermetic tests seed
			// specs/lifecycles here; counting them as TotalDisk invents disparity.
			continue
		}
		if streamBackedDirSet[dirName] {
			// DiskCountByDir stays 0: stream storage is authoritative; the disk walk would be
			// expensive (10–30 s for tens-of-thousands of monthly-bucketed files) and the count
			// is not used for disparity. A fast YAML count is stored separately in
			// LegacyStreamBackedByDir so operators can see how many files remain for cleanup.
			report.DiskCountByDir[dirName] = 0
			dirPath := filepath.Join(processDir, dirName)
			if n := countYAMLFilesInDir(dirPath); n > 0 {
				if report.LegacyStreamBackedByDir == nil {
					report.LegacyStreamBackedByDir = make(map[string]int)
				}
				report.LegacyStreamBackedByDir[dirName] = n
			}
			continue
		}
		dirPath := filepath.Join(processDir, dirName)
		var count int
		err := filepath.Walk(dirPath, func(path string, info fileutil.FileInfo, err error) error {
			if err != nil {
				if fileutil.IsNotExist(err) {
					return nil // skip deleted/missing file (TOCTOU or concurrent delete)
				}
				return err
			}
			if info.Mode().IsRegular() && strings.HasSuffix(info.Name(), yamlExt) {
				if appledouble.SkipPathInTreeWalk(path) {
					return nil
				}
				count++
			}
			return nil
		})
		if err != nil {
			return nil, errfmt.Errorf(errWalkDirFmt, dirPath, err)
		}
		report.DiskCountByDir[dirName] = count
		report.TotalDisk += count
	}

	// 2) Object count per kind: use cache when provided (fast); otherwise storage.Count per kind (slow on large repos).
	if len(opts.ObjectCountByKindFromCache) > 0 {
		for kind, n := range opts.ObjectCountByKindFromCache {
			report.ObjectCountByKind[kind] = n
			report.TotalObject += n
			dir := objects.GetDirectoryFromKind(kind)
			if dir != emptyValue {
				report.ObjectCountByDir[dir] += n
			}
		}
	} else {
		registry := objects.GetGlobalFieldRegistry()
		if err := registry.LoadFields(); err != nil {
			return nil, errfmt.Errorf(errLoadFieldRegistryFmt, err)
		}
		allKinds, err := registry.GetAllKinds()
		if err != nil {
			return nil, errfmt.Errorf(errGetAllKindsFmt, err)
		}
		for _, kind := range allKinds {
			n, err := storageProvider.Count(ctx, secCtx, storage.ListFilter{Kind: kind})
			if err != nil {
				continue
			}
			report.ObjectCountByKind[kind] = n
			report.TotalObject += n
			dir := objects.GetDirectoryFromKind(kind)
			if dir != emptyValue {
				report.ObjectCountByDir[dir] += n
			}
		}
	}

	// 3) Optional internal count (subprocess)
	if opts.IncludeInternal && opts.ZQKBin != emptyValue {
		internal, total, err := getInternalCountByKind(ctx, opts.ProjectRoot, opts.ZQKBin, opts.InternalTimeout)
		if err == nil {
			report.InternalCountByKind = internal
			report.TotalInternal = total
		}
	}

	// 4) Disparity by dir: disk - object (for dirs we have both)
	threshold := opts.DisparityThreshold
	if threshold <= 0 {
		threshold = DisparityAlertThreshold
	}
	for dir, diskCount := range report.DiskCountByDir {
		objectCount := report.ObjectCountByDir[dir]
		disp := diskCount - objectCount
		report.DisparityByDir[dir] = disp

		isStray := false
		if diskCount > 0 && dir != internalDirName && !strings.HasPrefix(dir, hiddenPrefix) && !streamBackedDirSet[dir] {
			if objects.GetKindFromDirectory(dir) == emptyValue {
				report.StrayDirs = append(report.StrayDirs, dir)
				report.Alerts = append(report.Alerts, fmt.Sprintf(errStrayProcessDirFmt, dir, diskCount))
				isStray = true
			}
		}

		// Alert only when disparity is positive and above threshold. Negative disparity for stream-backed dirs is expected (object from stream > legacy disk).
		if disp > threshold && !isStray {
			report.KindsWithDisparity = append(report.KindsWithDisparity, dir)
			report.Alerts = append(report.Alerts, fmt.Sprintf(errDisparityAlertFmt, dir, diskCount, objectCount, disp))
		}
	}
	if len(opts.StreamBackedDirs) > 0 {
		report.StreamBackedDirs = append([]string(nil), opts.StreamBackedDirs...)
		sort.Strings(report.StreamBackedDirs)
	}
	sort.Strings(report.KindsWithDisparity)
	sort.Strings(report.StrayDirs)

	return report, nil
}

// countYAMLFilesInDir recursively counts .yaml files under dir without reading their content.
// It is intentionally lightweight: only os.ReadDir calls, no stat on individual files, no YAML parsing.
// Used for stream-backed dirs where we need a legacy-file count for visibility without a full expensive walk.
func countYAMLFilesInDir(dir string) int {
	entries, err := fileutil.ReadDir(dir)
	if err != nil {
		return 0
	}
	var count int
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() {
			if name != internalDirName && !strings.HasPrefix(name, hiddenPrefix) {
				count += countYAMLFilesInDir(filepath.Join(dir, name))
			}
			continue
		}
		if strings.HasSuffix(name, yamlExt) && !strings.HasPrefix(name, hiddenPrefix) {
			count++
		}
	}
	return count
}

// getInternalCountByKind runs `zqk internal count --format json` and parses counts_by_kind and total_objects.
func getInternalCountByKind(ctx context.Context, projectRoot, zqkBin string, timeout time.Duration) (byKind map[string]int, total int, err error) {
	if timeout <= 0 {
		timeout = DefaultInternalCountTimeout
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := execwrap.CommandContext(
		runCtx,
		zqkBin,
		internalCountCmdSubcommand,
		internalCountCmdAction,
		internalCountCmdFormatFlag,
		internalCountCmdFormatJSON,
	)
	cmd.Dir = projectRoot
	out, execErr := cmd.CombinedOutput()
	if runCtx.Err() == context.DeadlineExceeded {
		return nil, 0, errors.New(internalCountTimeoutError)
	}
	if execErr != nil {
		return nil, 0, errfmt.Errorf(errInternalCountExecFmt, execErr, string(out))
	}
	var payload struct {
		CountsByKind map[string]int `json:"counts_by_kind"`
		TotalObjects int            `json:"total_objects"`
	}
	if err := json.Unmarshal(out, &payload); err != nil {
		return nil, 0, errfmt.Errorf(errParseInternalCountOutputFmt, err)
	}
	if payload.CountsByKind == nil {
		payload.CountsByKind = make(map[string]int)
	}
	return payload.CountsByKind, payload.TotalObjects, nil
}
