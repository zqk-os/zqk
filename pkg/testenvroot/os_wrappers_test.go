package testenvroot

import (
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
)

func TestOSWrappersPanicOnRepoWrite(t *testing.T) {
	repoRoot := paths.FindWorkspaceRoot(".")
	processDir := filepath.Join(repoRoot, paths.ProcessDir)
	targetFile := filepath.Join(processDir, "violation.txt")

	func() {
		defer func() {
			if r := recover(); r == nil {
				t.Errorf("WriteFile to repo process dir did not panic")
			}
		}()
		_ = WriteFile(targetFile, []byte("should panic"), 0644)
	}()

	func() {
		defer func() {
			if r := recover(); r == nil {
				t.Errorf("MkdirAll to repo process dir did not panic")
			}
		}()
		_ = MkdirAll(filepath.Join(processDir, "new_violation"), 0755)
	}()
}
