package operational

import (
	"context"
	"io/fs"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"

	"github.com/lanceman/zqk/pkg/appledouble"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
)

// DefaultFSSnapshotParallelism is how many top-level subtrees we walk concurrently (I/O bound).
// Tuning: raise on fast NVMe; lower if file descriptor or CPU scheduling becomes noisy.
const DefaultFSSnapshotParallelism = 8

// fsSegAgg holds partial counts for one top-level subtree walk (merged under a mutex).
type fsSegAgg struct {
	files   int64
	bytes   int64
	byTop   map[string]*FilesystemBucketStats
	zqkKids map[string]*FilesystemBucketStats
}

func newFsSegAgg() *fsSegAgg {
	return &fsSegAgg{
		byTop:   make(map[string]*FilesystemBucketStats),
		zqkKids: make(map[string]*FilesystemBucketStats),
	}
}

func (a *fsSegAgg) ensureTop(name string) *FilesystemBucketStats {
	if b, ok := a.byTop[name]; ok {
		return b
	}
	b := &FilesystemBucketStats{ByExtension: make(map[string]int64)}
	a.byTop[name] = b
	return b
}

func (a *fsSegAgg) ensureZqkChild(name string) *FilesystemBucketStats {
	if b, ok := a.zqkKids[name]; ok {
		return b
	}
	b := &FilesystemBucketStats{ByExtension: make(map[string]int64)}
	a.zqkKids[name] = b
	return b
}

func mergeSegIntoSnapshot(dst *FilesystemProjectSnapshot, src *fsSegAgg) {
	if dst == nil || src == nil {
		return
	}
	dst.TotalFiles += src.files
	dst.TotalBytes += src.bytes
	for name, b := range src.byTop {
		d := dst.ensureTopLevel(name)
		d.Files += b.Files
		d.Bytes += b.Bytes
		for ext, n := range b.ByExtension {
			d.ByExtension[ext] += n
		}
	}
	for name, b := range src.zqkKids {
		z := dst.ensureZqkChild(name)
		z.Files += b.Files
		z.Bytes += b.Bytes
		for ext, n := range b.ByExtension {
			z.ByExtension[ext] += n
		}
	}
}

func addRegularFileToSeg(rootAbs string, seg *fsSegAgg, path string, d fs.DirEntry, info fs.FileInfo) {
	rel, err := filepath.Rel(rootAbs, path)
	if err != nil || rel == "." || rel == emptyValue {
		return
	}
	rel = filepath.ToSlash(rel)
	parts := strings.Split(rel, "/")
	if len(parts) == 0 {
		return
	}
	top := parts[0]
	ext := strings.ToLower(filepath.Ext(d.Name()))
	if ext == emptyValue {
		ext = "(no_ext)"
	}
	size := info.Size()

	seg.files++
	seg.bytes += size

	b := seg.ensureTop(top)
	b.Files++
	b.Bytes += size
	b.ByExtension[ext]++

	if len(parts) >= 2 && top == paths.ProjectDataDir {
		child := parts[1]
		z := seg.ensureZqkChild(child)
		z.Files++
		z.Bytes += size
		z.ByExtension[ext]++
	}
}

func parallelFSSnapshotWorkers() int {
	n := DefaultFSSnapshotParallelism
	if g := runtime.GOMAXPROCS(0); g > 0 && g < n {
		n = g
	}
	if n < 1 {
		n = 1
	}
	return n
}

// runFilesystemProjectSnapshotParallel walks using one WalkDir goroutine per planned root (see planParallelWalkRoots).
// Full scope: one worker per top-level directory under the project root. Narrow scopes: fan-out under .zqk/ and/or docs/.
func runFilesystemProjectSnapshotParallel(ctx context.Context, rootAbs string, snap *FilesystemProjectSnapshot, scope FilesystemSnapshotScope) error {
	walkRoots, includeRootFiles, err := planParallelWalkRoots(rootAbs, scope)
	if err != nil {
		return err
	}

	workers := parallelFSSnapshotWorkers()
	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup
	var mergeMu sync.Mutex
	var firstErr error
	var errMu sync.Mutex
	setErr := func(e error) {
		if e == nil {
			return
		}
		errMu.Lock()
		if firstErr == nil {
			firstErr = e
		}
		errMu.Unlock()
	}

	if includeRootFiles {
		entries, err := fileutil.ReadDir(rootAbs)
		if err != nil {
			return errfmt.Newf("read project root").Wrap(err)
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			li, err := fileutil.Lstat(filepath.Join(rootAbs, e.Name()))
			if err != nil {
				continue
			}
			if li.Mode()&fileutil.ModeSymlink != 0 || !li.Mode().IsRegular() {
				continue
			}
			path := filepath.Join(rootAbs, e.Name())
			seg := newFsSegAgg()
			addRegularFileToSeg(rootAbs, seg, path, e, li)
			mergeSegIntoSnapshot(snap, seg)
		}
	}

	for _, topPath := range walkRoots {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		wg.Add(1)
		goroutinelabels.NewGoroutine("operational", "snapshot parallel subtree").StartSimple(func() {
			func(dirPath string) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()

				seg := newFsSegAgg()
				walkErr := filepath.WalkDir(dirPath, func(path string, d fs.DirEntry, walkErr error) error {
					if walkErr != nil {
						return walkErr
					}
					select {
					case <-ctx.Done():
						return ctx.Err()
					default:
					}

					if d.IsDir() {
						if path == dirPath {
							return nil
						}
						if li, err := fileutil.Lstat(path); err == nil && li.Mode()&fileutil.ModeSymlink != 0 {
							return filepath.SkipDir
						}
						if _, skip := DefaultFilesystemSnapshotSkipDirs[d.Name()]; skip {
							return filepath.SkipDir
						}
						return nil
					}

					if appledouble.SkipPathInTreeWalk(path) {
						return nil
					}

					li, err := fileutil.Lstat(path)
					if err != nil {
						return nil
					}
					if li.Mode()&fileutil.ModeSymlink != 0 || !li.Mode().IsRegular() {
						return nil
					}
					addRegularFileToSeg(rootAbs, seg, path, d, li)
					return nil
				})
				if walkErr != nil {
					setErr(walkErr)
					return
				}

				mergeMu.Lock()
				mergeSegIntoSnapshot(snap, seg)
				mergeMu.Unlock()
			}(topPath)
		})
	}

	wg.Wait()
	return firstErr
}
