package operational

import (
	"context"
	"path/filepath"

	"github.com/zqk-os/zqk/pkg/errfmt"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqktime"
)

// skipDirTarget is the Rust/Cargo build output directory. Same string as objects.FieldKeyTarget;
// kept local to avoid importing pkg/objects into this package.
const skipDirTarget = "target"

// DefaultFilesystemSnapshotSkipDirs are directory names skipped during a full-tree walk (heavy or non-actionable subtrees).
var DefaultFilesystemSnapshotSkipDirs = map[string]struct{}{
	".git":         {},
	"node_modules": {},
	"vendor":       {},
	".ide":         {},
	"dist":         {},
	"out":          {},
	skipDirTarget:  {},
	".idea":        {},
	"__pycache__":  {},
	".venv":        {},
	"venv":         {},
}

// FilesystemBucketStats aggregates file count, total bytes, and counts by file extension for one bucket.
type FilesystemBucketStats struct {
	Files       int64            `json:"files"`
	Bytes       int64            `json:"bytes"`
	ByExtension map[string]int64 `json:"by_extension"`
}

// FilesystemVolumeStats is the containing filesystem for projectRoot, from statfs(2) on Unix
// (same information df(1) reports for that mount). It is cheap (no tree walk) and complements
// TotalBytes/ByTopLevel, which count regular files under the project only.
type FilesystemVolumeStats struct {
	Source string `json:"source"` // e.g. "statfs"; empty when unavailable (e.g. Windows stub)

	BlockSizeBytes  uint64 `json:"block_size_bytes"`
	BlocksTotal     uint64 `json:"blocks_total"`
	BlocksFree      uint64 `json:"blocks_free"`
	BlocksAvailable uint64 `json:"blocks_available"`
	TotalBytes      uint64 `json:"total_bytes"`
	FreeBytes       uint64 `json:"free_bytes"`
	AvailableBytes  uint64 `json:"available_bytes"`
}

// FilesystemProjectSnapshot is a point-in-time view of file counts and types across the project tree.
// Used to spot unbounded growth ("landfill") under .zqk or elsewhere when sampled on a schedule.
type FilesystemProjectSnapshot struct {
	GeneratedAt string `json:"generated_at"`
	ProjectRoot string `json:"project_root"`
	TotalFiles  int64  `json:"total_files"`
	TotalBytes  int64  `json:"total_bytes"`
	// Volume: filesystem containing projectRoot (Unix: statfs; same underlying data as df). O(1); optional.
	Volume *FilesystemVolumeStats `json:"volume,omitempty"`
	// ByTopLevel: first path segment relative to project root (e.g. paths.ProjectDataDir, "docs", "pkg", "go.mod" for root files).
	ByTopLevel map[string]*FilesystemBucketStats `json:"by_top_level"`
	// ZqkByChild: immediate children under ".zqk/" only (logs, state, scheduler, metrics, …).
	ZqkByChild map[string]*FilesystemBucketStats `json:"zqk_by_child,omitempty"`
	// SnapshotScope is which walk strategy was used (full, zqk, docs, zqk-and-docs).
	SnapshotScope string `json:"snapshot_scope,omitempty"`
}

// RunFilesystemProjectSnapshot walks projectRoot (excluding DefaultFilesystemSnapshotSkipDirs) and builds a snapshot.
// scope controls breadth: FSSnapshotScopeFull walks the whole repo; narrow scopes only walk .zqk and/or docs/ with parallel fan-out.
// Pass empty scope or FSSnapshotScopeFull for the full tree.
// Honors ctx cancellation. Symlinks are not followed for directories (WalkDir does not descend into symlink dirs by default on most platforms when SkipDir is not used for them — we skip symlink dirs explicitly to avoid cycles).
func RunFilesystemProjectSnapshot(ctx context.Context, projectRoot string, scope FilesystemSnapshotScope) (*FilesystemProjectSnapshot, error) {
	if projectRoot == emptyValue {
		return nil, errfmt.Errorf("project root required")
	}
	rootAbs, err := filepath.Abs(projectRoot)
	if err != nil {
		return nil, errfmt.Newf("abs project root").Wrap(err)
	}
	fi, err := fileutil.Stat(rootAbs)
	if err != nil {
		return nil, errfmt.Newf("stat project root").Wrap(err)
	}
	if !fi.IsDir() {
		return nil, errfmt.Errorf("project root is not a directory: %s", rootAbs)
	}
	if scope == "" {
		scope = FSSnapshotScopeFull
	}

	snap := &FilesystemProjectSnapshot{
		GeneratedAt:   zqktime.NowRFC3339NanoUTC(),
		ProjectRoot:   rootAbs,
		SnapshotScope: string(scope),
		ByTopLevel:    make(map[string]*FilesystemBucketStats),
		ZqkByChild:    make(map[string]*FilesystemBucketStats),
	}
	// Fast: whole-filesystem capacity/free space (kernel statfs — same family of data as df).
	fillVolumeStats(snap, rootAbs)

	if err := runFilesystemProjectSnapshotParallel(ctx, rootAbs, snap, scope); err != nil {
		return nil, err
	}
	return snap, nil
}

func (s *FilesystemProjectSnapshot) ensureTopLevel(name string) *FilesystemBucketStats {
	if b, ok := s.ByTopLevel[name]; ok {
		return b
	}
	b := &FilesystemBucketStats{ByExtension: make(map[string]int64)}
	s.ByTopLevel[name] = b
	return b
}

func (s *FilesystemProjectSnapshot) ensureZqkChild(name string) *FilesystemBucketStats {
	if b, ok := s.ZqkByChild[name]; ok {
		return b
	}
	b := &FilesystemBucketStats{ByExtension: make(map[string]int64)}
	s.ZqkByChild[name] = b
	return b
}
