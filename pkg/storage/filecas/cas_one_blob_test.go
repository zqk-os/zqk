package filecas

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestEnsureExactlyOneLiveCASBlob_RemovesExtraAndKeepsHash(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	keeper := "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	extra := "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
	keeperPath := filepath.Join(dir, keeper+".yaml")
	extraPath := filepath.Join(dir, extra+".yaml")
	body := []byte("id: REQ-ONE-BLOB\nkind: requirement\n")
	if err := fileutil.WriteStandardFile(keeperPath, body); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteStandardFile(extraPath, body); err != nil {
		t.Fatal(err)
	}
	if err := ensureExactlyOneLiveCASBlob("REQ-ONE-BLOB", keeper, dir); err != nil {
		t.Fatal(err)
	}
	if !fileutil.Exists(keeperPath) {
		t.Fatal("keeper blob must remain")
	}
	if fileutil.Exists(extraPath) {
		t.Fatal("extra blob must be removed before Update returns")
	}
}

func TestWithSweepLock_CleansUpLockFileOnSuccess(t *testing.T) {
	dir := t.TempDir()
	cas := &ContentAddressableStorage{kindDir: dir, kind: "test_kind"}
	lockPath := filepath.Join(dir, ".cas_sweep.lock")

	ran := false
	err := cas.WithSweepLock(func() error {
		ran = true
		if !fileutil.Exists(lockPath) {
			t.Error("expected .cas_sweep.lock to exist while lock is held")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected WithSweepLock error: %v", err)
	}
	if !ran {
		t.Fatal("expected fn to execute")
	}
	if fileutil.Exists(lockPath) {
		t.Error("expected .cas_sweep.lock to be unlinked after WithSweepLock completes")
	}
}

func TestWithSweepLock_CleansUpLockFileOnFailure(t *testing.T) {
	dir := t.TempDir()
	cas := &ContentAddressableStorage{kindDir: dir, kind: "test_kind"}
	lockPath := filepath.Join(dir, ".cas_sweep.lock")

	expectedErr := filepath.ErrBadPattern
	err := cas.WithSweepLock(func() error {
		return expectedErr
	})
	if err != expectedErr {
		t.Fatalf("expected error %v, got %v", expectedErr, err)
	}
	if fileutil.Exists(lockPath) {
		t.Error("expected .cas_sweep.lock to be unlinked after WithSweepLock fails")
	}
}

func TestWithSweepLock_ReconcilesStaleLockOlderThanTTL(t *testing.T) {
	dir := t.TempDir()
	cas := &ContentAddressableStorage{kindDir: dir, kind: "test_kind"}
	lockPath := filepath.Join(dir, ".cas_sweep.lock")

	// Create abandoned lock file with older modtime (>15s)
	if err := fileutil.WriteStandardFile(lockPath, []byte("999999")); err != nil {
		t.Fatal(err)
	}

	staleTime := time.Now().Add(-30 * time.Second)
	if err := os.Chtimes(lockPath, staleTime, staleTime); err != nil {
		t.Fatal(err)
	}

	err := cas.WithSweepLock(func() error {
		return nil
	})
	if err != nil {
		t.Fatalf("WithSweepLock failed over stale lock: %v", err)
	}
	if fileutil.Exists(lockPath) {
		t.Error("expected stale .cas_sweep.lock to be cleaned up after completion")
	}
}

func TestWithSweepLock_ReconcilesOrphanedPIDLock(t *testing.T) {
	dir := t.TempDir()
	cas := &ContentAddressableStorage{kindDir: dir, kind: "test_kind"}
	lockPath := filepath.Join(dir, ".cas_sweep.lock")

	// PID 99999999 is nonexistent/orphaned
	if err := fileutil.WriteStandardFile(lockPath, []byte("99999999")); err != nil {
		t.Fatal(err)
	}

	err := cas.WithSweepLock(func() error {
		return nil
	})
	if err != nil {
		t.Fatalf("WithSweepLock failed over orphaned PID lock: %v", err)
	}
	if fileutil.Exists(lockPath) {
		t.Error("expected orphaned PID lock to be cleaned up")
	}
}
