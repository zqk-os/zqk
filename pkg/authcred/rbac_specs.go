package authcred

import (
	"strings"
	"sync"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

type rbacSpecSnap struct {
	mu       sync.Mutex
	dirMtime int64
	loaded   bool
	err      error
}

var rbacSpecSnaps sync.Map

func rbacSpecCache(projectRoot string) *rbacSpecSnap {
	if existing, ok := rbacSpecSnaps.Load(projectRoot); ok {
		return existing.(*rbacSpecSnap)
	}
	fresh := &rbacSpecSnap{}
	actual, _ := rbacSpecSnaps.LoadOrStore(projectRoot, fresh)
	return actual.(*rbacSpecSnap)
}

// RequireRBACSpecs reports whether account and role object specs exist (flat or domain bucket).
// Result is retained until the specs directory mtime changes.
func RequireRBACSpecs(projectRoot string) error {
	if strings.TrimSpace(projectRoot) == "" {
		return errfmt.Errorf("object specs directory missing")
	}
	specsDir := paths.ObjectSpecsDir(projectRoot)
	var dirMtime int64
	if info, err := fileutil.Stat(specsDir); err == nil {
		dirMtime = info.ModTime().UnixNano()
	}
	snap := rbacSpecCache(projectRoot)
	snap.mu.Lock()
	defer snap.mu.Unlock()
	if snap.loaded && snap.dirMtime == dirMtime {
		return snap.err
	}
	if _, err := paths.FindObjectSpecFile(specsDir, objects.KindAccount); err != nil {
		snap.dirMtime = dirMtime
		snap.loaded = true
		snap.err = err
		return err
	}
	if _, err := paths.FindObjectSpecFile(specsDir, objects.KindRole); err != nil {
		snap.dirMtime = dirMtime
		snap.loaded = true
		snap.err = err
		return err
	}
	snap.dirMtime = dirMtime
	snap.loaded = true
	snap.err = nil
	return nil
}
