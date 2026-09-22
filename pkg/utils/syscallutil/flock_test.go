//go:build !windows

package syscallutil

import (
	"syscall"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestFileFlock(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := tmpDir + "/test_flock.txt"

	f, err := fileutil.OpenFile(filePath, fileutil.O_CREATE|fileutil.O_RDWR, paths.FilePerm644)
	if err != nil {
		t.Fatalf("failed to open file: %v", err)
	}
	defer f.Close()

	if err := FileFlock(f, syscall.LOCK_EX); err != nil {
		t.Fatalf("failed to acquire exclusive lock: %v", err)
	}

	if err := FileFlock(f, syscall.LOCK_UN); err != nil {
		t.Fatalf("failed to unlock: %v", err)
	}
}
