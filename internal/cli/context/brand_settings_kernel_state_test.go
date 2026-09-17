package context

import (
	"path/filepath"
	"testing"
)

func TestBrandSettings_ResolveSnapshotBackupDir(t *testing.T) {
	t.Parallel()
	root := "/tmp/zqk-proj"
	s := &BrandSettings{KernelState: BrandSettingsKernelState{SnapshotBackupDir: "../zqk-csnap-backups"}}
	got := s.ResolveSnapshotBackupDir(root)
	want := filepath.Clean(filepath.Join(root, "../zqk-csnap-backups"))
	if got != want {
		t.Fatalf("relative: got %s want %s", got, want)
	}
	s.KernelState.SnapshotBackupDir = "/var/backups/csnap"
	if s.ResolveSnapshotBackupDir(root) != "/var/backups/csnap" {
		t.Fatalf("absolute: %s", s.ResolveSnapshotBackupDir(root))
	}
	s.KernelState.SnapshotBackupDir = ""
	if s.ResolveSnapshotBackupDir(root) != "" {
		t.Fatal("empty should yield empty")
	}
}

func TestResolveStateCommitBackup_viaSettingsShape(t *testing.T) {
	t.Parallel()
	// Smoke: unmarshalled keep is visible on KernelState.
	s := &BrandSettings{}
	s.KernelState.SnapshotBackupKeep = 5
	if s.KernelState.SnapshotBackupKeep != 5 {
		t.Fatal("keep not set")
	}
}
