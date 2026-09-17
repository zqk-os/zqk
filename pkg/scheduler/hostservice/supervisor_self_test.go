package hostservice

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

func TestRefuseIfCallerDescendsFromUnit_liveAncestor(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	dir := filepath.Join(root, paths.ProjectDataDir, paths.SchedulerDir)
	if err := fileutil.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	pid := os.Getpid()
	if err := fileutil.WriteFile(filepath.Join(dir, paths.SchedulerPIDFile), []byte(strconv.Itoa(pid)+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := refuseIfCallerDescendsFromUnit(Entry{AbsRoot: root, UnitLabel: "com.zqk.scheduler.test"})
	if err == nil {
		t.Fatal("expected refuse when pid file is the caller")
	}
}

func TestRefuseIfCallerDescendsFromUnit_missingPID(t *testing.T) {
	t.Parallel()
	if err := refuseIfCallerDescendsFromUnit(Entry{AbsRoot: t.TempDir(), UnitLabel: "com.zqk.scheduler.test"}); err != nil {
		t.Fatalf("missing pid file should not refuse: %v", err)
	}
}
