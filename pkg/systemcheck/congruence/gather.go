package congruence

import (
	"context"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/objectidcache"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// GatherCacheStatus returns object ID cache and reverse reference index status for the report.
func GatherCacheStatus(projectRoot string) CacheStatus {
	var status CacheStatus
	cacheDir := filepath.Join(projectRoot, paths.ProjectDataDir, paths.CacheDir)
	status.ObjectIDCache.Path = filepath.Join(cacheDir, paths.ObjectIDCacheFile)
	objCache := objectidcache.GetGlobalObjectIDCache()
	if loaded := objectidcache.TryLoadObjectIDCacheOnly(projectRoot); loaded {
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

// GatherProcessIntegrity scans the process directory for misplaced, unmanaged, or stray files.
func GatherProcessIntegrity(projectRoot string, streamBackedDirs []string) ProcessIntegrity {
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
	objCache := objectidcache.GetGlobalObjectIDCache()
	_ = objectidcache.TryLoadObjectIDCacheOnly(projectRoot)

	// Stream-backed dirs hold files managed by stream storage, not the object-id cache.
	streamBackedSet := make(map[string]bool, len(streamBackedDirs))
	for _, d := range streamBackedDirs {
		streamBackedSet[d] = true
	}

	var unmanaged []string
	_ = filepath.Walk(processDir, func(path string, info fileutil.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			if info.Name() == OCRProcessInternalDir {
				return filepath.SkipDir
			}
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
		if id == "" {
			return nil
		}
		if _, ok := objCache.Get(id); !ok {
			unmanaged = append(unmanaged, path)
		}
		return nil
	})
	if len(unmanaged) > 0 {
		out.UnmanagedFiles = unmanaged
	}

	entries, err := fileutil.ReadDir(processDir)
	if err != nil {
		return out
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() {
			if name == OCRProcessInternalDir || strings.HasPrefix(name, ".") {
				continue
			}
			if objects.GetKindFromDirectory(name) == "" {
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

// GatherObjectCountByKindForReport returns a single source of truth for object counts by kind.
func GatherObjectCountByKindForReport(ctx context.Context, projectRoot string, storageProvider storage.ObjectStorageProvider, noCache bool) map[string]int {
	if projectRoot == "" {
		return nil
	}
	if noCache {
		return nil
	}
	ensureCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	_ = storage.EnsureHighVolumeEventCacheReady(ensureCtx, projectRoot, storageProvider, false)
	cancel()

	objCache := objectidcache.GetGlobalObjectIDCache()
	if objCache == nil || !objectidcache.TryLoadObjectIDCacheOnly(projectRoot) {
		return nil
	}
	raw := objCache.CountByKind()
	if raw == nil {
		return nil
	}
	out := make(map[string]int, len(raw))
	for k, n := range raw {
		if n < 0 {
			n = 0
		}
		out[k] = n
	}
	hvCache := storage.GetGlobalHighVolumeEventCache()
	if hvCache != nil && hvCache.IsPopulatedForProject(projectRoot) {
		for _, kind := range storage.StreamStorageEnabledKindsList() {
			out[kind] = hvCache.CountByKind(kind)
		}
	}
	return out
}

// StreamBackedKindsForReport returns the list of kinds that use stream storage.
func StreamBackedKindsForReport() []string {
	return storage.StreamStorageEnabledKindsList()
}

// StreamBackedDirsForReport returns dir names under .zqk/process where object count is from stream storage.
func StreamBackedDirsForReport() []string {
	dirs := make(map[string]bool)
	for _, kind := range storage.StreamStorageEnabledKindsList() {
		if d := objects.GetDirectoryFromKind(kind); d != "" {
			dirs[d] = true
		}
	}
	out := make([]string, 0, len(dirs))
	for d := range dirs {
		out = append(out, d)
	}
	return out
}
