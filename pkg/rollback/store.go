package rollback

import (
	"bufio"
	"encoding/json"
	"errors"
	"path/filepath"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/concurrency"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

const (
	rollbackWALFileName      = "rollback_points"
	maxRollbackLineSize      = 2 * 1024 * 1024 // 2 MiB per line
	errStoreNeedsProjectRoot = "rollback store requires non-empty project root"
	errCreateStoreDirFmt     = "create rollback store dir: %w"
	errMarshalPointFmt       = "marshal rollback point: %w"
	errPointTooLargeFmt      = "rollback point too large: %d bytes"
	storeDirPerm             = 0o755
	storeFilePerm            = 0o600
	scannerInitialBufSize    = 65536
	tmpFileSuffix            = ".tmp"
)

// Store is an append-only store for rollback points (JSONL file). Thread-safe; Append/Retain use exclusive lock, List/Get use RLock.
type Store struct {
	mu          sync.RWMutex
	path        string
	projectRoot string
}

// NewStore creates or opens the rollback store under projectRoot/.zqk/wal/rollback_points.
func NewStore(projectRoot string) (*Store, error) {
	if projectRoot == emptyValue {
		return nil, errors.New(errStoreNeedsProjectRoot)
	}
	dir := filepath.Join(projectRoot, paths.ProjectDataDir, paths.WalDir)
	if err := fileutil.MkdirAll(dir, storeDirPerm); err != nil {
		return nil, errfmt.Errorf(errCreateStoreDirFmt, err)
	}
	path := filepath.Join(dir, rollbackWALFileName)
	return &Store{path: path, projectRoot: projectRoot}, nil
}

// Append appends a rollback point and syncs to disk.
func (s *Store) Append(p *RollbackPoint) error {
	return concurrency.RunInLockWithLogger(&s.mu, LockNameRollbackStoreAppend, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		f, err := fileutil.OpenFile(s.path, fileutil.O_CREATE|fileutil.O_WRONLY|fileutil.O_APPEND, storeFilePerm)
		if err != nil {
			return err
		}
		defer f.Close() //nolint:gosec
		line, err := json.Marshal(p)
		if err != nil {
			return errfmt.Errorf(errMarshalPointFmt, err)
		}
		if len(line) > maxRollbackLineSize {
			return errfmt.Errorf(errPointTooLargeFmt, len(line))
		}
		if _, err := f.Write(line); err != nil {
			return err
		}
		if _, err := f.Write([]byte{'\n'}); err != nil {
			return err
		}
		return f.Sync()
	})
}

// List reads all points and returns meta for each, most recent last. Caller can filter by time/count.
func (s *Store) List() ([]Meta, error) {
	var metas []Meta
	err := concurrency.RunInRLockWithLogger(&s.mu, LockNameRollbackStoreList, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		f, err := fileutil.Open(s.path)
		if err != nil {
			if fileutil.IsNotExist(err) {
				return nil
			}
			return err
		}
		defer f.Close() //nolint:gosec
		sc := bufio.NewScanner(f)
		buf := make([]byte, 0, scannerInitialBufSize)
		sc.Buffer(buf, maxRollbackLineSize)
		for sc.Scan() {
			var p RollbackPoint
			if err := json.Unmarshal(sc.Bytes(), &p); err != nil {
				continue
			}
			metas = append(metas, Meta{
				ID:          p.ID,
				Timestamp:   p.Timestamp,
				ScopeType:   p.ScopeType,
				ScopeID:     p.ScopeID,
				ObjectCount: len(p.ObjectStates),
			})
		}
		return sc.Err()
	})
	if err != nil {
		return nil, err
	}
	return metas, nil
}

// Get finds a rollback point by ID by scanning the file.
func (s *Store) Get(id string) (*RollbackPoint, error) {
	var out *RollbackPoint
	err := concurrency.RunInRLockWithLogger(&s.mu, LockNameRollbackStoreGet, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		f, err := fileutil.Open(s.path)
		if err != nil {
			if fileutil.IsNotExist(err) {
				return nil
			}
			return err
		}
		defer f.Close() //nolint:gosec
		sc := bufio.NewScanner(f)
		buf := make([]byte, 0, scannerInitialBufSize)
		sc.Buffer(buf, maxRollbackLineSize)
		for sc.Scan() {
			var p RollbackPoint
			if err := json.Unmarshal(sc.Bytes(), &p); err != nil {
				continue
			}
			if p.ID == id {
				out = &p
				return nil
			}
		}
		return sc.Err()
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Retain rewrites the store to keep only the last keepCount points that are also within keepDuration of now.
func (s *Store) Retain(keepCount int, keepDuration time.Duration) error {
	return concurrency.RunInLockWithLogger(&s.mu, LockNameRollbackStoreRetain, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		f, err := fileutil.Open(s.path)
		if err != nil {
			if fileutil.IsNotExist(err) {
				return nil
			}
			return err
		}
		var all []RollbackPoint
		sc := bufio.NewScanner(f)
		buf := make([]byte, 0, scannerInitialBufSize)
		sc.Buffer(buf, maxRollbackLineSize)
		for sc.Scan() {
			var p RollbackPoint
			if err := json.Unmarshal(sc.Bytes(), &p); err != nil {
				continue
			}
			all = append(all, p)
		}
		f.Close() //nolint:gosec
		if err := sc.Err(); err != nil {
			return err
		}
		now := time.Now().UTC()
		cutoff := now.Add(-keepDuration)
		var filtered []RollbackPoint
		for _, p := range all {
			if !p.Timestamp.Before(cutoff) {
				filtered = append(filtered, p)
			}
		}
		start := 0
		if len(filtered) > keepCount {
			start = len(filtered) - keepCount
		}
		keep := filtered[start:]
		if len(keep) >= len(all) {
			return nil
		}
		// Rewrite file
		tmpPath := s.path + tmpFileSuffix
		wt, err := fileutil.Create(tmpPath)
		if err != nil {
			return err
		}
		for _, p := range keep {
			line, _ := json.Marshal(p)
			line = append(line, '\n')
			if _, err := wt.Write(line); err != nil {
				wt.Close() //nolint:gosec
				_ = fileutil.Remove(tmpPath)
				return err
			}
		}
		if err := wt.Sync(); err != nil {
			wt.Close() //nolint:gosec
			_ = fileutil.Remove(tmpPath)
			return err
		}
		if err := wt.Close(); err != nil { //nolint:gosec
			_ = fileutil.Remove(tmpPath)
			return err
		}
		return fileutil.Rename(tmpPath, s.path)
	})
}
