package system

import (
	"path/filepath"
	"testing"

	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestRejectThinStateCommit(t *testing.T) {
	t.Parallel()
	if err := rejectThinStateCommit(5487, 219, false); err == nil {
		t.Fatal("expected refuse on thin shrink")
	}
	if err := rejectThinStateCommit(5487, 219, true); err != nil {
		t.Fatalf("allow-shrink should pass: %v", err)
	}
	if err := rejectThinStateCommit(5487, 5000, false); err != nil {
		t.Fatalf("modest shrink should pass: %v", err)
	}
	if err := rejectThinStateCommit(0, 10, false); err != nil {
		t.Fatalf("no prior should pass: %v", err)
	}
}

func TestRotateCSnapBackups(t *testing.T) {
	t.Parallel()
	backup := t.TempDir()
	for _, name := range []string{
		"prior_100_1_aaaaaaaa.csnap",
		"prior_200_1_bbbbbbbb.csnap",
		"prior_300_1_cccccccc.csnap",
		"prior_400_1_dddddddd.csnap",
		"prior_500_1_eeeeeeee.csnap",
	} {
		if err := fileutil.WriteFile(filepath.Join(backup, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := rotateCSnapBackups(backup, 3); err != nil {
		t.Fatal(err)
	}
	entries, err := fileutil.ReadDir(backup)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 files, got %d", len(entries))
	}
	for _, want := range []string{"prior_300_1_cccccccc.csnap", "prior_400_1_dddddddd.csnap", "prior_500_1_eeeeeeee.csnap"} {
		if _, err := fileutil.Stat(filepath.Join(backup, want)); err != nil {
			t.Fatalf("missing kept file %s: %v", want, err)
		}
	}
}

func TestArchivePriorCSnapExternal(t *testing.T) {
	t.Parallel()
	backup := t.TempDir()
	tip := filepath.Join(t.TempDir(), "system-state.csnap")
	payload := []byte("header:\n    object_count: 2\n")
	if err := fileutil.WriteFile(tip, payload, 0o644); err != nil {
		t.Fatal(err)
	}
	dest, err := archivePriorCSnapExternal(backup, tip, 2, "abcdef0123456789", 3)
	if err != nil {
		t.Fatal(err)
	}
	got, err := fileutil.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(payload) {
		t.Fatal("archive mismatch")
	}
}

func TestDefaultExternalCSnapBackupDir(t *testing.T) {
	t.Parallel()
	got := defaultExternalCSnapBackupDir("/Users/me/ai-projects/zqk")
	want := "/Users/me/ai-projects/zqk-csnap-backups"
	if got != want {
		t.Fatalf("got %s want %s", got, want)
	}
}

func TestResolveStateCommitBackup_defaultsWithoutSettings(t *testing.T) {
	t.Parallel()
	// Temp root without brand settings → sibling default + keep 3
	root := t.TempDir()
	dir, keep := resolveStateCommitBackup(root, "", 0)
	wantDir := defaultExternalCSnapBackupDir(root)
	if dir != wantDir {
		t.Fatalf("dir got %s want %s", dir, wantDir)
	}
	if keep != defaultExternalCSnapBackupKeep {
		t.Fatalf("keep got %d want %d", keep, defaultExternalCSnapBackupKeep)
	}
	dir2, keep2 := resolveStateCommitBackup(root, "/custom/backups", 7)
	if dir2 != "/custom/backups" || keep2 != 7 {
		t.Fatalf("flag override: %s %d", dir2, keep2)
	}
}

func TestNewStateCommitCmd_flags(t *testing.T) {
	t.Parallel()
	cmd := NewStateCommitCmd()
	for _, name := range []string{"snapshot-file", "allow-shrink", "skip-archive", "backup-dir", "backup-keep"} {
		if cmd.Flags().Lookup(name) == nil {
			t.Errorf("missing flag --%s", name)
		}
	}
}
