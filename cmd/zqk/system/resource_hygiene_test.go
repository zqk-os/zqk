package system

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestNewResourceHygieneCmd(t *testing.T) {
	cmd := NewResourceHygieneCmd()
	if cmd.Use != "resource-hygiene" {
		t.Fatalf("expected Use=resource-hygiene, got %s", cmd.Use)
	}

	dryRunFlag := cmd.Flag("dry-run")
	if dryRunFlag == nil {
		t.Fatal("missing --dry-run flag")
	}

	reapLocksFlag := cmd.Flag("reap-locks")
	if reapLocksFlag == nil {
		t.Fatal("missing --reap-locks flag")
	}

	reapTempFlag := cmd.Flag("reap-temp")
	if reapTempFlag == nil {
		t.Fatal("missing --reap-temp flag")
	}
}

func TestRunResourceHygiene_Execution(t *testing.T) {
	dir := t.TempDir()
	cacheDir := filepath.Join(dir, paths.ProjectDataDir, "cache")
	if err := fileutil.MkdirAll(cacheDir, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}

	oldTmp := filepath.Join(cacheDir, ".tmp-durable-abc")
	_ = fileutil.WriteFile(oldTmp, []byte("temporary"), paths.FilePerm600)
	oldTime := time.Now().Add(-2 * time.Hour)
	_ = os.Chtimes(oldTmp, oldTime, oldTime)

	cmd := NewResourceHygieneCmd()
	cmd.SetArgs([]string{"--dry-run", "--project-root", dir, "--temp-threshold", "1h"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("cmd.Execute() dry-run failed: %v", err)
	}

	// In dry-run mode, file should still exist
	if _, err := fileutil.Stat(oldTmp); err != nil {
		t.Errorf("file should exist in dry-run mode")
	}

	// Live run with 1h threshold to reap
	cmdLive := NewResourceHygieneCmd()
	cmdLive.SetArgs([]string{"--project-root", dir, "--temp-threshold", "1h"})
	err = cmdLive.Execute()
	if err != nil {
		t.Fatalf("cmdLive.Execute() failed: %v", err)
	}

	if _, err := fileutil.Stat(oldTmp); !fileutil.IsNotExist(err) {
		t.Errorf("expected temp file to be reaped in live run")
	}
}
