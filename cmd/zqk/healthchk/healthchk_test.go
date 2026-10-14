package healthchk

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/healthcheck"
	_ "github.com/zqk-os/zqk/pkg/healthcheck/monitors"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func createHealthchkTestCmd(t *testing.T, projectRoot string, storageProvider storage.ObjectStorageProvider, args ...string) (*cobra.Command, *bytes.Buffer) {
	t.Helper()
	cmd := NewHealthchkCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	baseCtx := pkgctx.NewSystemContext()
	baseCtx = pkgctx.WithCommandOutputWriter(baseCtx, &buf)
	baseCtx = pkgctx.WithSecurityContext(baseCtx, pkgctx.NewSystemSecurityContext())
	if storageProvider != nil {
		baseCtx = cli.WithStorageProvider(baseCtx, storageProvider)
	}
	cmd.SetContext(baseCtx)

	initCtx := pkgctx.NewCliInitializationContext(cli.ResolveProjectRoot, ".")
	cctx, err := cli.GetContextFromCommand(cmd, initCtx)
	require.NoError(t, err)
	cctx.ProjectRoot = projectRoot
	cli.SetContext(cmd, cctx)
	cmd.SetArgs(args)

	return cmd, &buf
}

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

func TestHealthchkCmd_Structure(t *testing.T) {
	cmd := NewHealthchkCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "healthchk", cmd.Use)
	assert.Contains(t, cmd.Short, "Health check registry")

	subcommands := make(map[string]*cobra.Command)
	for _, sub := range cmd.Commands() {
		subcommands[sub.Name()] = sub
	}

	assert.Contains(t, subcommands, "list")
	assert.Contains(t, subcommands, "update")
	assert.Contains(t, subcommands, "run")
	assert.Contains(t, subcommands, "bulk")
}

func TestHealthchkList(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, nil)

	cmd, buf := createHealthchkTestCmd(t, proj.Root, proj.FileStorage, "list")
	err := cmd.Execute()
	require.NoError(t, err)

	out := buf.String()
	assert.Contains(t, out, "object_volume")
	assert.Contains(t, out, "wal_backlog")
}

func TestHealthchkUpdate(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, nil)

	// 1. Disable a monitor
	cmdDisable, bufDisable := createHealthchkTestCmd(t, proj.Root, proj.FileStorage, "update", "object_volume", "--enabled=false")
	err := cmdDisable.Execute()
	require.NoError(t, err)
	assert.Contains(t, bufDisable.String(), "Monitor object_volume enabled=false")

	// 2. Re-enable the monitor
	cmdEnable, bufEnable := createHealthchkTestCmd(t, proj.Root, proj.FileStorage, "update", "object_volume", "--enabled=true")
	err = cmdEnable.Execute()
	require.NoError(t, err)
	assert.Contains(t, bufEnable.String(), "Monitor object_volume enabled=true")

	// 3. Error on non-existent monitor
	cmdInvalid, bufInvalid := createHealthchkTestCmd(t, proj.Root, proj.FileStorage, "update", "nonexistent_monitor_xyz")
	assert.NotNil(t, bufInvalid)
	err = cmdInvalid.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestHealthchkRun(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, nil)

	// 1. Run single monitor in table/text format
	cmdSingle, bufSingle := createHealthchkTestCmd(t, proj.Root, proj.FileStorage, "run", "object_volume")
	err := cmdSingle.Execute()
	require.NoError(t, err)
	assert.Contains(t, bufSingle.String(), "status=")

	// 2. Run single monitor in JSON format
	cmdJSON, bufJSON := createHealthchkTestCmd(t, proj.Root, proj.FileStorage, "run", "object_volume", "--format", "json")
	err = cmdJSON.Execute()
	require.NoError(t, err)
	assert.Contains(t, bufJSON.String(), `"status"`)

	// 3. Run all monitors
	cmdAll, bufAll := createHealthchkTestCmd(t, proj.Root, proj.FileStorage, "run")
	err = cmdAll.Execute()
	require.NoError(t, err)
	assert.Contains(t, bufAll.String(), "status=")
}

func TestHealthchkBulkDisable(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, nil)

	// 1. Disable via --ids flag
	cmdIDs, bufIDs := createHealthchkTestCmd(t, proj.Root, proj.FileStorage, "bulk", "disable", "--ids", "object_volume,wal_backlog")
	err := cmdIDs.Execute()
	require.NoError(t, err)
	assert.Contains(t, bufIDs.String(), "Disabled 2 monitor(s)")

	// 2. Disable via --ids-file with YAML list
	yamlFile := filepath.Join(proj.Root, "monitors.yaml")
	require.NoError(t, fileutil.WriteFile(yamlFile, []byte("- object_volume\n- stream_volume\n"), 0600))

	cmdYAML, bufYAML := createHealthchkTestCmd(t, proj.Root, proj.FileStorage, "bulk", "disable", "--ids-file", yamlFile)
	err = cmdYAML.Execute()
	require.NoError(t, err)
	assert.Contains(t, bufYAML.String(), "Disabled 2 monitor(s)")

	// 3. Disable via --ids-file with plain list
	txtFile := filepath.Join(proj.Root, "monitors.txt")
	require.NoError(t, fileutil.WriteFile(txtFile, []byte("object_volume\nwal_backlog\n"), 0600))

	cmdTxt, bufTxt := createHealthchkTestCmd(t, proj.Root, proj.FileStorage, "bulk", "disable", "--ids-file", txtFile)
	err = cmdTxt.Execute()
	require.NoError(t, err)
	assert.Contains(t, bufTxt.String(), "Disabled 2 monitor(s)")

	// 4. Missing both flags returns error
	cmdNoArgs, bufNoArgs := createHealthchkTestCmd(t, proj.Root, proj.FileStorage, "bulk", "disable")
	assert.NotNil(t, bufNoArgs)
	err = cmdNoArgs.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "provide --ids or --ids-file")

	// 5. Invalid file path returns error
	cmdMissingFile, bufMissingFile := createHealthchkTestCmd(t, proj.Root, proj.FileStorage, "bulk", "disable", "--ids-file", "nonexistent.yaml")
	assert.NotNil(t, bufMissingFile)
	err = cmdMissingFile.Execute()
	require.Error(t, err)
}

func TestResolveHealthcheckProjectRoot_ErrorCases(t *testing.T) {
	t.Chdir(t.TempDir())
	cmd := NewHealthchkCmd()
	initCtx := pkgctx.NewCliInitializationContext(cli.ResolveProjectRoot, ".")
	cctx, getErr := cli.GetContextFromCommand(cmd, initCtx)
	require.NoError(t, getErr)
	cctx.ProjectRoot = ""
	cli.SetContext(cmd, cctx)

	root, err := resolveHealthcheckProjectRoot(cmd)
	assert.Empty(t, root)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "project root not found")

	boundRoot, reg, regErr := resolveBoundRegistry(cmd)
	assert.Empty(t, boundRoot)
	assert.Nil(t, reg)
	require.Error(t, regErr)
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
