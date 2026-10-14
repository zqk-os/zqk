package healthchk

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/healthcheck"
	_ "github.com/zqk-os/zqk/pkg/healthcheck/monitors"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func executeHealthchk(t *testing.T, projRoot string, args ...string) (string, error) {
	t.Helper()
	cmd := NewHealthchkCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs(args)

	// Inject Cobra command context with command output writer
	ctx := pkgctx.WithCommandOutputWriter(context.Background(), &buf)
	cmd.SetContext(ctx)

	err := cmd.Execute()
	return buf.String(), err
}

func TestNewHealthchkCmd_Hierarchy(t *testing.T) {
	cmd := NewHealthchkCmd()
	if cmd == nil {
		t.Fatal("expected NewHealthchkCmd to return non-nil command")
	}
	if cmd.Use != "healthchk" {
		t.Fatalf("expected Use='healthchk', got %q", cmd.Use)
	}

	subCommands := map[string]*cobra.Command{}
	for _, c := range cmd.Commands() {
		subCommands[c.Name()] = c
	}

	for _, expected := range []string{"list", "update", "run", "bulk"} {
		if _, ok := subCommands[expected]; !ok {
			t.Errorf("missing subcommand %q under healthchk", expected)
		}
	}

	bulk := subCommands["bulk"]
	var hasDisable bool
	for _, c := range bulk.Commands() {
		if c.Name() == "disable" {
			hasDisable = true
		}
	}
	if !hasDisable {
		t.Error("missing 'disable' subcommand under 'bulk'")
	}
}

func TestHealthchk_List(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, nil)

	// List table format
	out, err := executeHealthchk(t, proj.Root, "list")
	if err != nil {
		t.Fatalf("healthchk list failed: %v, out: %s", err, out)
	}
	if !strings.Contains(out, "object_volume") && !strings.Contains(out, "autonomy_inbox") {
		t.Fatalf("expected monitor IDs in list output, got: %s", out)
	}

	// List JSON format
	jsonOut, err := executeHealthchk(t, proj.Root, "list", "--format", "json")
	if err != nil {
		t.Fatalf("healthchk list --format json failed: %v, out: %s", err, jsonOut)
	}
	if !strings.Contains(jsonOut, "object_volume") {
		t.Fatalf("expected object_volume in json list output, got: %s", jsonOut)
	}
}

func getFirstMonitorID(t *testing.T) string {
	t.Helper()
	monitors := healthcheck.DefaultRegistry.List()
	if len(monitors) == 0 {
		t.Fatal("no health monitors registered in DefaultRegistry")
	}
	return monitors[0].ID()
}

func TestHealthchk_Update(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, nil)
	targetID := getFirstMonitorID(t)

	// Disable monitor
	out, err := executeHealthchk(t, proj.Root, "update", targetID, "--enabled=false")
	if err != nil {
		t.Fatalf("healthchk update --enabled=false failed: %v, out: %s", err, out)
	}
	if !strings.Contains(out, "enabled=false") {
		t.Fatalf("expected output to mention enabled=false, got: %s", out)
	}

	// Re-enable monitor
	out, err = executeHealthchk(t, proj.Root, "update", targetID, "--enabled=true")
	if err != nil {
		t.Fatalf("healthchk update --enabled=true failed: %v, out: %s", err, out)
	}
	if !strings.Contains(out, "enabled=true") {
		t.Fatalf("expected output to mention enabled=true, got: %s", out)
	}

	// Update non-existent monitor
	_, err = executeHealthchk(t, proj.Root, "update", "non_existent_monitor_id", "--enabled=false")
	if err == nil {
		t.Fatal("expected error updating non-existent monitor")
	}

	// Missing ID arg
	_, err = executeHealthchk(t, proj.Root, "update")
	if err == nil {
		t.Fatal("expected error updating without args")
	}
}

func TestHealthchk_Run(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, nil)
	targetID := getFirstMonitorID(t)

	// Run single monitor with table format
	out, err := executeHealthchk(t, proj.Root, "run", targetID)
	if err != nil {
		t.Fatalf("healthchk run %s failed: %v, out: %s", targetID, err, out)
	}
	if !strings.Contains(out, "status=") {
		t.Fatalf("expected status= in output, got: %s", out)
	}

	// Run single monitor with json format
	jsonOut, err := executeHealthchk(t, proj.Root, "run", targetID, "--format", "json")
	if err != nil {
		t.Fatalf("healthchk run %s --format json failed: %v, out: %s", targetID, err, jsonOut)
	}
	if !strings.Contains(jsonOut, targetID) && !strings.Contains(jsonOut, "status") {
		t.Fatalf("expected json result with status, got: %s", jsonOut)
	}

	// Run all monitors
	out, err = executeHealthchk(t, proj.Root, "run")
	if err != nil {
		t.Fatalf("healthchk run (all) failed: %v, out: %s", err, out)
	}
	if !strings.Contains(out, "status=") {
		t.Fatalf("expected status= in output for all monitors run, got: %s", out)
	}

	// Run non-existent monitor
	_, err = executeHealthchk(t, proj.Root, "run", "non_existent_id")
	if err == nil {
		t.Fatal("expected error running non-existent monitor")
	}
}

func TestHealthchk_BulkDisable(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, nil)

	monitors := healthcheck.DefaultRegistry.List()
	if len(monitors) < 2 {
		t.Skip("need at least 2 monitors for bulk test")
	}
	id1 := monitors[0].ID()
	id2 := monitors[1].ID()

	// Bulk disable via --ids flag
	out, err := executeHealthchk(t, proj.Root, "bulk", "disable", "--ids", id1+","+id2)
	if err != nil {
		t.Fatalf("bulk disable --ids failed: %v, out: %s", err, out)
	}
	if !strings.Contains(out, "Disabled 2 monitor(s)") {
		t.Fatalf("expected 'Disabled 2 monitor(s)', got: %s", out)
	}

	// Bulk disable via YAML --ids-file
	yamlFile := filepath.Join(proj.Root, "disable_monitors.yaml")
	yamlContent := "- " + id1 + "\n- " + id2 + "\n"
	if err := fileutil.WriteFile(yamlFile, []byte(yamlContent), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	out, err = executeHealthchk(t, proj.Root, "bulk", "disable", "--ids-file", yamlFile)
	if err != nil {
		t.Fatalf("bulk disable --ids-file (yaml) failed: %v, out: %s", err, out)
	}
	if !strings.Contains(out, "Disabled 2 monitor(s)") {
		t.Fatalf("expected 'Disabled 2 monitor(s)', got: %s", out)
	}

	// Bulk disable via plain text --ids-file
	txtFile := filepath.Join(proj.Root, "disable_monitors.txt")
	txtContent := id1 + "\n\n" + id2 + "\n"
	if err := fileutil.WriteFile(txtFile, []byte(txtContent), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	out, err = executeHealthchk(t, proj.Root, "bulk", "disable", "--ids-file", txtFile)
	if err != nil {
		t.Fatalf("bulk disable --ids-file (txt) failed: %v, out: %s", err, out)
	}
	if !strings.Contains(out, "Disabled 2 monitor(s)") {
		t.Fatalf("expected 'Disabled 2 monitor(s)', got: %s", out)
	}

	// Missing flags
	_, err = executeHealthchk(t, proj.Root, "bulk", "disable")
	if err == nil {
		t.Fatal("expected error for bulk disable without flags")
	}

	// Non-existent ids file
	_, err = executeHealthchk(t, proj.Root, "bulk", "disable", "--ids-file", "/non/existent/file.txt")
	if err == nil {
		t.Fatal("expected error for non-existent ids file")
	}
}

func TestSplitIDs(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{"", []string{""}},
		{"a", []string{"a"}},
		{"a,b", []string{"a", "b"}},
		{" a , b , c ", []string{"a", "b", "c"}},
	}
	for _, tt := range tests {
		got := splitIDs(tt.input)
		if len(got) != len(tt.expected) {
			t.Fatalf("splitIDs(%q) length = %d, want %d", tt.input, len(got), len(tt.expected))
		}
		for i := range got {
			if got[i] != tt.expected[i] {
				t.Errorf("splitIDs(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.expected[i])
			}
		}
	}
}

func TestReadIDsFromFile(t *testing.T) {
	dir := t.TempDir()

	// Empty file
	emptyFile := filepath.Join(dir, "empty.txt")
	if err := fileutil.WriteFile(emptyFile, []byte(""), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	ids, err := readIDsFromFile(emptyFile)
	if err != nil || len(ids) != 0 {
		t.Fatalf("expected empty ids, got %v, err=%v", ids, err)
	}

	// Non-existent file
	_, err = readIDsFromFile(filepath.Join(dir, "nonexistent.txt"))
	if err == nil {
		t.Fatal("expected error for non-existent file")
	}
}
