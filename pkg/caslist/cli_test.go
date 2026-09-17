package caslist

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"
)

const testGoalID = "GOAL-1"
const testGoalHash = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func TestParseInventory(t *testing.T) {
	req, ok := parseInventory([]string{"object", "list", "goal", "--format", "json"})
	if !ok {
		t.Fatal("expected inventory parse")
	}
	if req.verb != verbList || req.kind != "goal" || req.format != formatJSON {
		t.Fatalf("got %+v", req)
	}
	if _, ok := parseInventory([]string{"object", "create", "goal"}); ok {
		t.Fatal("create must not take the index path")
	}
	if _, ok := parseInventory([]string{"object", "list", "goal", "--filter", "status=originated"}); ok {
		t.Fatal("filtered list must fall through to the full CLI")
	}
}

func TestListKindFromIndex(t *testing.T) {
	root := t.TempDir()
	kindDir := filepath.Join(root, processDirName, "goals")
	if err := os.MkdirAll(kindDir, 0o755); err != nil {
		t.Fatal(err)
	}
	idx := []byte(`{"version":"1","kind":"goal","mappings":{"` + testGoalID + `":"` + testGoalHash + `"}}`)
	if err := os.WriteFile(filepath.Join(kindDir, ".goal.index"), idx, 0o644); err != nil {
		t.Fatal(err)
	}
	blob := []byte("id: " + testGoalID + "\nkind: goal\nstatus: originated\ntitle: launch\n")
	if err := os.WriteFile(filepath.Join(kindDir, testGoalHash+".yaml"), blob, 0o644); err != nil {
		t.Fatal(err)
	}

	stdout, code := runTry(t, []string{"--project-root", root, "object", "list", "goal", "--format", "json"})
	if code != 0 {
		t.Fatalf("exit %d stdout %s", code, stdout)
	}
	var payload struct {
		Meta    map[string]any   `json:"meta"`
		Objects []map[string]any `json:"objects"`
	}
	if err := json.Unmarshal(stdout, &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Objects) != 1 {
		t.Fatalf("objects=%d", len(payload.Objects))
	}
	if payload.Objects[0]["id"] != testGoalID {
		t.Fatalf("id=%v", payload.Objects[0]["id"])
	}
	if payload.Meta[metaKeyCASIndex] != true {
		t.Fatalf("meta=%v", payload.Meta)
	}
}

func TestGetUsesIndexMapping(t *testing.T) {
	root := t.TempDir()
	kindDir := filepath.Join(root, processDirName, "goals")
	if err := os.MkdirAll(kindDir, 0o755); err != nil {
		t.Fatal(err)
	}
	idx := []byte(`{"version":"1","kind":"goal","mappings":{"` + testGoalID + `":"` + testGoalHash + `"}}`)
	if err := os.WriteFile(filepath.Join(kindDir, ".goal.index"), idx, 0o644); err != nil {
		t.Fatal(err)
	}
	blob := []byte("id: " + testGoalID + "\nkind: goal\ntitle: launch\n")
	if err := os.WriteFile(filepath.Join(kindDir, testGoalHash+".yaml"), blob, 0o644); err != nil {
		t.Fatal(err)
	}
	stdout, code := runTry(t, []string{"--project-root", root, "object", "get", testGoalID, "--format", "json"})
	if code != 0 {
		t.Fatalf("exit %d stdout %s", code, stdout)
	}
	var obj map[string]any
	if err := json.Unmarshal(stdout, &obj); err != nil {
		t.Fatal(err)
	}
	if obj["id"] != testGoalID {
		t.Fatalf("got %v", obj)
	}
}

func runTry(t *testing.T, args []string) ([]byte, int) {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	handled, code := Try(args)
	_ = w.Close()
	os.Stdout = old
	if !handled {
		t.Fatal("Try did not handle inventory argv")
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	_ = r.Close()
	return bytes.TrimSpace(out), code
}
