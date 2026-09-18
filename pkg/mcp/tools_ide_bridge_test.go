package mcp

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/idebridge"
	"github.com/zqk-os/zqk/pkg/objects"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestHandleIdeBridgeRequest_queuesEvent(t *testing.T) {
	root := t.TempDir()
	res, err := HandleIdeBridgeRequest(context.Background(), nil, map[string]any{
		objects.FieldKeyCommand: "zqk.mcp.reloadClient",
		"request_id":            "t1",
	}, root)
	if err != nil {
		t.Fatal(err)
	}
	m, ok := res.(map[string]any)
	if !ok || m[objects.FieldKeyStatus] != "queued" {
		t.Fatalf("unexpected result: %#v", res)
	}
	path := idebridge.ControlJSONLPath(root)
	data, err := fileutil.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var ev idebridge.ControlEventV1
	if err := json.Unmarshal(data[:len(data)-1], &ev); err != nil {
		t.Fatal(err)
	}
	if ev.Command != "zqk.mcp.reloadClient" {
		t.Fatalf("got %+v", ev)
	}
}

func TestHandleIdeBridgeRequest_requiresCommand(t *testing.T) {
	_, err := HandleIdeBridgeRequest(context.Background(), nil, map[string]any{}, t.TempDir())
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRegisterIdeBridgeTool_name(t *testing.T) {
	// Ensure GetToolName stays stable for clients / allowlists.
	name := GetToolName("ide_bridge_request")
	if filepath.Base(name) == "" || name == "" {
		t.Fatal("empty tool name")
	}
	if got := GetToolName("ide_bridge_request"); got != name {
		t.Fatalf("unstable name %q vs %q", got, name)
	}
}
