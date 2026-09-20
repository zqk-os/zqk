package idebridge

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestAppendRequest_writesV1Line(t *testing.T) {
	root := t.TempDir()
	path, err := AppendRequest(root, "zqk.mcp.reloadClient", "req-1", nil)
	if err != nil {
		t.Fatal(err)
	}
	want := ControlJSONLPath(root)
	if path != want {
		t.Fatalf("path=%q want %q", path, want)
	}
	data, err := fileutil.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var ev ControlEventV1
	if err := json.Unmarshal(data[:len(data)-1], &ev); err != nil {
		t.Fatal(err)
	}
	if ev.Schema != SchemaV1 || ev.Command != "zqk.mcp.reloadClient" || ev.RequestID != "req-1" {
		t.Fatalf("unexpected event: %+v", ev)
	}
}

func TestAppendRequest_rejectsPrivateIDEIDs(t *testing.T) {
	_, err := AppendRequest(t.TempDir(), "mcp.reloadClient", "", nil)
	if err == nil || !strings.Contains(err.Error(), StableCommandPrefix) {
		t.Fatalf("expected prefix rejection, got %v", err)
	}
}

func TestControlJSONLPath(t *testing.T) {
	got := ControlJSONLPath("/proj")
	if !strings.HasSuffix(got, filepath.Join(paths.ProjectDataDir, paths.LogsDir, "ide-hooks", "ide_bridge_control.jsonl")) {
		t.Fatalf("unexpected path: %s", got)
	}
}
