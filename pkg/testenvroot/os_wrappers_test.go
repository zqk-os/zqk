package testenvroot

import (
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/paths"
)

func TestOSWrappersPanicOnRepoWrite(t *testing.T) {
	repoRoot := paths.FindWorkspaceRoot(".")
	processDir := filepath.Join(repoRoot, "docs", "process")
	targetFile := filepath.Join(processDir, "violation.txt")

	func() {
		defer func() {
			if r := recover(); r == nil {
				t.Errorf("WriteFile to repo docs/process did not panic")
			}
		}()
		WriteFile(targetFile, []byte("should panic"), 0644)
	}()

	func() {
		defer func() {
			if r := recover(); r == nil {
				t.Errorf("MkdirAll to repo docs/process did not panic")
			}
		}()
		MkdirAll(filepath.Join(processDir, "new_violation"), 0755)
	}()
}
