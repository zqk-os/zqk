package fileutil

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

func TestOSWrappersPanicOnRepoWrite(t *testing.T) {
	repo, _ := fileutilEnv()
	processDir := filepath.Join(repo, defaultProcessDir)
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
		uniqueDir := fmt.Sprintf("new_violation_%d", time.Now().UnixNano())
		_ = MkdirAll(filepath.Join(processDir, uniqueDir), 0755)
	}()
}
