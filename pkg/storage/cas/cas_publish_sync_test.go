package cas_test

import (
	"os"

	"github.com/zqk-os/zqk/pkg/storage"
	caspkg "github.com/zqk-os/zqk/pkg/storage/cas"

	"errors"
	"os/exec"
	"path/filepath"

	"github.com/zqk-os/zqk/pkg/storage/filecas"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"

	"testing"
	"time"
)

const (
	envCASCrashKillChild = "CAS_CRASH_KILL_CHILD"
	envCASCrashKillDir   = "CAS_CRASH_KILL_DIR"
)

var casCrashKillPayload = []byte("kind: scheduler_job\nid: SCH-cas-crash-kill\nstatus: active\n")

// TRACK: BLI-CEF-R16-CAS-FSYNC-001 / CRIT-CEF-R2-REL-CAS-FSYNC-A / REQ-CEF-R2-REL-CAS-FSYNC
func TestCAS_WriteFileWithSync_CallsPublishSyncHooks(t *testing.T) {
	var fileN, dirN int
	origFile, origDir := filecas.CasPublishSyncFile, filecas.CasPublishSyncDir
	t.Cleanup(func() {
		filecas.CasPublishSyncFile = origFile
		filecas.CasPublishSyncDir = origDir
	})
	filecas.CasPublishSyncFile = func(f *fileutil.File) error {
		fileN++
		return origFile(f)
	}
	filecas.CasPublishSyncDir = func(dir string) error {
		dirN++
		return origDir(dir)
	}

	tmpDir := t.TempDir()
	casQueue := caspkg.NewListingIndexWriteQueueForTest()
	defer casQueue.Shutdown()
	cas := filecas.NewContentAddressableStorage(tmpDir, "scheduler_job", casQueue)

	data := []byte("kind: scheduler_job\nid: SCH-hook\n")
	dest := filepath.Join(tmpDir, storage.CalculateSHA256Hash(data)+".yaml")
	if err := cas.WriteFileWithSync(dest, data); err != nil {
		t.Fatalf("WriteFileWithSync: %v", err)
	}
	if fileN != 1 || dirN != 1 {
		t.Fatalf("publish sync hooks file=%d dir=%d; want 1 and 1", fileN, dirN)
	}
}

func TestCAS_WriteFileWithSync_FileSyncErrorFailsClosed(t *testing.T) {
	origFile, origDir := filecas.CasPublishSyncFile, filecas.CasPublishSyncDir
	t.Cleanup(func() {
		filecas.CasPublishSyncFile = origFile
		filecas.CasPublishSyncDir = origDir
	})
	filecas.CasPublishSyncFile = func(*fileutil.File) error {
		return errors.New("inject file sync")
	}

	tmpDir := t.TempDir()
	casQueue := caspkg.NewListingIndexWriteQueueForTest()
	defer casQueue.Shutdown()
	cas := filecas.NewContentAddressableStorage(tmpDir, "scheduler_job", casQueue)

	data := []byte("kind: scheduler_job\nid: SCH-sync-fail\n")
	dest := filepath.Join(tmpDir, storage.CalculateSHA256Hash(data)+".yaml")
	if err := cas.WriteFileWithSync(dest, data); err == nil {
		t.Fatal("expected file-sync error")
	}
	if _, err := fileutil.Stat(dest); !fileutil.IsNotExist(err) {
		t.Fatalf("CAS object must not publish when temp fsync fails; stat=%v", err)
	}
}

func TestCAS_WriteFileWithSync_DirSyncErrorFailsClosed(t *testing.T) {
	origFile, origDir := filecas.CasPublishSyncFile, filecas.CasPublishSyncDir
	t.Cleanup(func() {
		filecas.CasPublishSyncFile = origFile
		filecas.CasPublishSyncDir = origDir
	})
	filecas.CasPublishSyncDir = func(string) error {
		return errors.New("inject dir sync")
	}

	tmpDir := t.TempDir()
	casQueue := caspkg.NewListingIndexWriteQueueForTest()
	defer casQueue.Shutdown()
	cas := filecas.NewContentAddressableStorage(tmpDir, "scheduler_job", casQueue)

	data := []byte("kind: scheduler_job\nid: SCH-dir-sync-fail\n")
	dest := filepath.Join(tmpDir, storage.CalculateSHA256Hash(data)+".yaml")
	if err := cas.WriteFileWithSync(dest, data); err == nil {
		t.Fatal("expected dir-sync error")
	}
}

func TestCAS_WriteFileWithSync_VisibleAfterChildKill(t *testing.T) {
	if os.Getenv(envCASCrashKillChild) == "1" {
		dir := os.Getenv(envCASCrashKillDir)
		casQueue := caspkg.NewListingIndexWriteQueueForTest()
		cas := filecas.NewContentAddressableStorage(dir, "scheduler_job", casQueue)
		dest := filepath.Join(dir, storage.CalculateSHA256Hash(casCrashKillPayload)+".yaml")
		if err := cas.WriteFileWithSync(dest, casCrashKillPayload); err != nil {
			os.Exit(2)
		}
		if err := fileutil.WriteFile(filepath.Join(dir, "ready"), []byte("ok\n"), 0o644); err != nil {
			os.Exit(3)
		}
		select {}
	}

	dir := t.TempDir()
	cmd := exec.Command(os.Args[0], "-test.run=^TestCAS_WriteFileWithSync_VisibleAfterChildKill$", "-test.count=1") //nolint:gosec
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), envCASCrashKillChild+"=1", envCASCrashKillDir+"="+dir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start crash-kill child: %v", err)
	}

	ready := filepath.Join(dir, "ready")
	deadline := time.Now().Add(15 * time.Second)
	for {
		if _, err := fileutil.Stat(ready); err == nil {
			break
		}
		if time.Now().After(deadline) {
			_ = cmd.Process.Kill()
			_, _ = cmd.Process.Wait()
			t.Fatal("crash-kill child never published CAS object")
		}
		time.Sleep(10 * time.Millisecond)
	}

	if err := cmd.Process.Kill(); err != nil {
		t.Fatalf("SIGKILL child: %v", err)
	}
	_, _ = cmd.Process.Wait()

	dest := filepath.Join(dir, storage.CalculateSHA256Hash(casCrashKillPayload)+".yaml")
	got, err := fileutil.ReadFile(dest)
	if err != nil {
		t.Fatalf("CAS object missing after child kill: %v", err)
	}
	if string(got) != string(casCrashKillPayload) {
		t.Fatalf("CAS bytes after child kill: %q", got)
	}
	if hash := storage.CalculateSHA256Hash(got); hash != storage.CalculateSHA256Hash(casCrashKillPayload) {
		t.Fatalf("hash mismatch after child kill: %s", hash)
	}
}
