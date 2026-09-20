package system

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestPrepareCompactCheckOutputData_CASDuplicateIDsTier1(t *testing.T) {
	root := t.TempDir()
	kindDir := filepath.Join(root, paths.ProcessBacklogDir)
	if err := fileutil.MkdirAll(kindDir, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	a := filepath.Join(kindDir, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa.yaml")
	b := filepath.Join(kindDir, "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb.yaml")
	body := "id: BLI-CHECK-DUAL-001\nkind: backlog_item\n"
	if err := fileutil.WriteFile(a, []byte(body), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond)
	if err := fileutil.WriteFile(b, []byte(body), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}

	cmd := &cobra.Command{Use: "check"}
	raw, err := json.Marshal(PrepareCompactCheckOutputData(cmd, nil, 0, nil, root))
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	summary, _ := out[objects.FieldKeySummary].(map[string]any)
	if summary == nil {
		t.Fatalf("missing summary: %s", raw)
	}
	blocking, _ := summary["blocking_issues"].(float64)
	if blocking < 1 {
		t.Fatalf("expected Tier-1 blocking for dual CAS, got summary=%v", summary)
	}
	casDup, ok := out["cas_duplicate_ids"].(map[string]any)
	if !ok || casDup == nil {
		t.Fatalf("expected cas_duplicate_ids in output: %s", raw)
	}
	count, _ := casDup["duplicate_count"].(float64)
	if count != 1 {
		t.Fatalf("duplicate_count=%v want 1", count)
	}
}
