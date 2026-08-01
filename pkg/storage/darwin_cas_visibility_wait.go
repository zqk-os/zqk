package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/lanceman/zqk/pkg/appledouble"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/datacell"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
)

// Cross-process CLI durability on macOS: after WAL checkpoint and CAS index flush, another
// reader process can still miss hash files briefly (directory/metadata visibility). We replace a
// fixed sleep in waitForWALProcessingEventDriven with a bounded poll that verifies indexed CAS
// files are stat-able on disk (or, for very large indexes, that the kind directory view stabilizes).
const (
	darwinCASVisibilityPollInterval = 5 * time.Millisecond
	// Worst-case prior barrier was ~450ms fixed sleep + flush; keep headroom for slow visibility.
	darwinCASVisibilityMaxWait = 1200 * time.Millisecond
	// Beyond this, full per-ID stat passes are avoided; use directory fingerprint stability instead.
	darwinCASVisibilityMaxFullStatMappings = 4096
)

func postFlushDarwinCASVisibilityIfNeeded(ctx context.Context, projectRoot string, flushKinds []string) error {
	if runtime.GOOS != "darwin" || projectRoot == emptyValue {
		return nil
	}
	deadline := time.Now().Add(darwinCASVisibilityMaxWait)
	if ctx != nil {
		if dl, ok := ctx.Deadline(); ok && dl.Before(deadline) {
			deadline = dl
		}
	}
	for _, kind := range flushKinds {
		if kind == emptyValue {
			continue
		}
		if StreamStorageEnabledForKind(kind) {
			continue
		}
		if err := darwinWaitCASKindVisible(ctx, projectRoot, kind, deadline); err != nil {
			return err
		}
	}
	return nil
}

func darwinWaitCASKindVisible(ctx context.Context, projectRoot, kind string, deadline time.Time) error {
	dirName := objects.GetDirectoryFromKind(kind)
	if dirName == emptyValue {
		return nil
	}
	kindDir := datacell.CellCASPrimaryDir(projectRoot, dirName)
	q := GetListingIndexWriteQueueForProjectRoot(projectRoot)
	indexPath := filepath.Join(kindDir, fmt.Sprintf(".%s.index", kind))

	for time.Now().Before(deadline) {
		if ctx != nil && ctx.Err() != nil {
			return errfmt.Errorf(ConstMiscDarwinCasVisibilityWaitKindSW, kind, context.Cause(ctx))
		}

		if _, err := os.Stat(kindDir); err != nil {
			time.Sleep(darwinCASVisibilityPollInterval)
			continue
		}

		if _, err := os.Stat(indexPath); err != nil {
			if os.IsNotExist(err) {
				time.Sleep(darwinCASVisibilityPollInterval)
				continue
			}
			return errfmt.Errorf(ConstMiscStatCasIndexKindSW, kind, err)
		}

		cas := NewContentAddressableStorage(kindDir, kind, q)
		all, err := cas.GetAllMappings()
		if err != nil {
			return errfmt.Errorf(ConstMiscCasMappingsKindSW, kind, err)
		}
		if len(all) == 0 {
			data, rerr := os.ReadFile(indexPath)
			if rerr != nil {
				time.Sleep(darwinCASVisibilityPollInterval)
				continue
			}
			var probe struct {
				Mappings map[string]string `json:"mappings"`
			}
			if jerr := json.Unmarshal(data, &probe); jerr == nil && len(probe.Mappings) > 0 {
				// Index on disk has entries but in-memory load disagrees — keep polling.
				if err := darwinSleepPoll(ctx, darwinCASVisibilityPollInterval); err != nil {
					return errfmt.Errorf(ConstMiscDarwinCasVisibilityWaitKindSW, kind, err)
				}
				continue
			}
			return nil
		}

		if len(all) > darwinCASVisibilityMaxFullStatMappings {
			if err := darwinWaitKindDirYAMLFingerprintStable(ctx, kindDir, deadline); err != nil {
				return errfmt.Errorf(ConstMiscDarwinCasVisibilityKindSLargeIndexW, kind, err)
			}
			return nil
		}

		ids := make([]string, 0, len(all))
		for id := range all {
			ids = append(ids, id)
		}
		sort.Strings(ids)

		ok := true
		for _, id := range ids {
			path, err := cas.GetFilePathForID(id)
			if err != nil {
				ok = false
				break
			}
			if segmentPath, _, isStream := StreamPathAndOffset(path); isStream {
				if _, err := os.Stat(segmentPath); err != nil {
					ok = false
					break
				}
				continue
			}
			if _, err := os.Stat(path); err != nil {
				ok = false
				break
			}
		}
		if ok {
			return nil
		}

		if err := darwinSleepPoll(ctx, darwinCASVisibilityPollInterval); err != nil {
			return errfmt.Errorf(ConstMiscDarwinCasVisibilityWaitKindSW, kind, err)
		}
	}

	return errfmt.Errorf(ConstMiscTimeoutWaitingForCasHashFilesVisibleKind, kind)
}

func darwinSleepPoll(ctx context.Context, d time.Duration) error {
	if ctx == nil {
		time.Sleep(d)
		return nil
	}
	select {
	case <-ctx.Done():
		return context.Cause(ctx)
	case <-time.After(d):
		return nil
	}
}

// darwinWaitKindDirYAMLFingerprintStable waits until three consecutive WalkDir fingerprints match
// (non-hidden *.yaml under kindDir, depth <= 2). Used when the index is too large for full stat.
func darwinWaitKindDirYAMLFingerprintStable(ctx context.Context, kindDir string, deadline time.Time) error {
	var prev string
	consecutive := 0
	for time.Now().Before(deadline) {
		if ctx != nil && ctx.Err() != nil {
			return context.Cause(ctx)
		}
		fp, err := darwinKindDirYAMLFingerprint(kindDir)
		if err != nil {
			consecutive = 0
			prev = ""
			time.Sleep(darwinCASVisibilityPollInterval)
			continue
		}
		if fp == prev && fp != "" {
			consecutive++
			if consecutive >= 2 {
				return nil
			}
		} else {
			consecutive = 0
		}
		prev = fp
		if err := darwinSleepPoll(ctx, darwinCASVisibilityPollInterval); err != nil {
			return err
		}
	}
	return errfmt.Errorf(ConstMiscTimeoutWaitingForKindDirectoryListingToS, kindDir)
}

func darwinKindDirYAMLFingerprint(kindDir string) (string, error) {
	var b strings.Builder
	err := filepath.WalkDir(kindDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(kindDir, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		if strings.Count(rel, string(filepath.Separator)) > 2 {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil // skip deep files (beyond bucket/hash layout)
		}
		if d.IsDir() {
			return nil
		}
		if appledouble.SkipPathInTreeWalk(path) {
			return nil
		}
		name := d.Name()
		if strings.HasPrefix(name, ".") {
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(name), ".yaml") {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		var _err_82755351 error
		_, _err_82755351 = fmt.Fprintf(&b, "%s:%d|", rel, info.Size())
		if _err_82755351 != nil {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, _err_82755351).Log()
		}
		return nil
	})
	return b.String(), err
}
