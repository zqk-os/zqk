package testkit

import (
	"github.com/lanceman/zqk/pkg/datacell"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"

	"testing"

	"github.com/lanceman/zqk/pkg/paths"
)

func TestPrepareIsolatedTempProject_DefaultLayout(t *testing.T) {
	p := PrepareIsolatedTempProject(t, nil)
	if p.Root == "" {
		t.Fatal("empty Root")
	}
	if p.FileStorage == nil {
		t.Fatal("nil FileStorage")
	}
	processDir := datacell.ProcessPrimaryDir(p.Root)
	if st, err := fileutil.Stat(processDir); err != nil || !st.IsDir() {
		t.Fatalf("expected process dir %s: stat=%v err=%v", paths.ProcessDir, st, err)
	}
}
