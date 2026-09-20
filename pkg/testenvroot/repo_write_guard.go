package testenvroot

import (
	"path/filepath"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// ValidateNoRepoStateMutation re-exports the fileutil predicate so snapshot tests
// and existing call sites keep a testenvroot import without duplicating the rule.
func ValidateNoRepoStateMutation(targetPath, repoRoot, testRoot string) error {
	return fileutil.ValidateNoRepoStateMutation(targetPath, repoRoot, testRoot)
}

// GuardStateSnapshot captures modtimes of files in protected repo directories.
type GuardStateSnapshot struct {
	repoRoot string
	files    map[string]int64
}

// SnapshotRepoState records the state of .zqk/process under repoRoot.
func SnapshotRepoState(repoRoot string) (*GuardStateSnapshot, error) {
	absRepo, err := filepath.Abs(repoRoot)
	if err != nil {
		return nil, errfmt.Errorf("failed to resolve repo root: %w", err)
	}
	snap := &GuardStateSnapshot{
		repoRoot: absRepo,
		files:    make(map[string]int64),
	}

	dirsToWatch := []string{
		filepath.Join(absRepo, paths.ProcessDir),
	}

	for _, dir := range dirsToWatch {
		if _, err := fileutil.Stat(dir); fileutil.IsNotExist(err) {
			continue
		}
		err := filepath.Walk(dir, func(p string, info fileutil.FileInfo, err error) error {
			if err != nil || info == nil {
				return nil
			}
			if !info.IsDir() {
				snap.files[p] = info.ModTime().UnixNano()
			}
			return nil
		})
		if err != nil {
			return nil, errfmt.Errorf("failed to snapshot repo state: %w", err)
		}
	}

	return snap, nil
}

// VerifyNoMutations asserts that no files in the snapshot were modified or added.
func (s *GuardStateSnapshot) VerifyNoMutations() error {
	if s == nil || s.repoRoot == "" {
		return nil
	}

	dirsToWatch := []string{
		filepath.Join(s.repoRoot, paths.ProcessDir),
	}

	for _, dir := range dirsToWatch {
		if _, err := fileutil.Stat(dir); fileutil.IsNotExist(err) {
			continue
		}
		var violated string
		err := filepath.Walk(dir, func(p string, info fileutil.FileInfo, err error) error {
			if err != nil || info == nil || violated != "" {
				return nil
			}
			if !info.IsDir() {
				origMod, existed := s.files[p]
				if !existed {
					violated = "new file created in repo process dir: " + p
					return nil
				}
				if info.ModTime().UnixNano() != origMod {
					violated = "file modified in repo process dir: " + p
					return nil
				}
			}
			return nil
		})
		if err != nil {
			return err
		}
		if violated != "" {
			return errfmt.Errorf("test isolation gate violation: %s", violated)
		}
	}

	return nil
}
