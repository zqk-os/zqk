package scheduler

import (
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestReadCVSRollupLatestSummary(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	sid := "CVS-1776080007703030000-a726d525"
	body := `{"schema_version":"rollup_v1","parent_convergence_session_id":"` + sid + `","rollup_status":"satisfied","ready_for_parent_completion":true}` + "\n"
	path := ResolveCVSRollupLatestJSONPath(tmp, "")
	if err := fileutil.EnsureDir(filepath.Dir(path)); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteSecureFile(path, []byte(body)); err != nil {
		t.Fatal(err)
	}
	status, ready, matched, err := ReadCVSRollupLatestSummary(sid, path)
	if err != nil {
		t.Fatal(err)
	}
	if !matched || status != "satisfied" || !ready {
		t.Fatalf("got matched=%v status=%q ready=%v", matched, status, ready)
	}
	other, _, matchedOther, err := ReadCVSRollupLatestSummary("CVS-1776080007703030000-a726d526", path)
	if err != nil {
		t.Fatal(err)
	}
	if matchedOther {
		t.Fatal("expected no match for different session id")
	}
	if other != "" {
		t.Fatalf("want empty status, got %q", other)
	}
}
