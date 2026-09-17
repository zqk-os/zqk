package object

// BLI-177483 inventory: SetupTestEnvironment → testkit.RunStandardTeardown (TempProjectTeardown) in test_helpers.go.

import (
	"github.com/lanceman/zqk/pkg/execwrap"
	"github.com/lanceman/zqk/pkg/utils/fileutil"

	"encoding/json"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
)

// TestCLI_MigrateVettingMatrix_AppendCVSActivity_Integration runs scripts/migrate-vetting-matrix-complete.py
// against an isolated ZQK_TEST_ROOT: creates a convergence_session, archives one fully-done row, and asserts
// zqk object update appended activity_log (vetting_matrix_rows_archived). Requires python3 + PyYAML.
func TestCLI_MigrateVettingMatrix_AppendCVSActivity_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skip CLI integration in -short mode")
	}
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not on PATH")
	}
	if err := execwrap.Command("python3", "-c", "import yaml").Run(); err != nil {
		t.Skip("PyYAML not installed for python3 (pip install pyyaml)")
	}

	te := SetupTestEnvironment(t)
	root := te.GetTestRoot()
	projectRoot := te.ProjectRoot

	const cvsID = "CVS-VET-MIG-001"
	cvsYAML := filepath.Join(t.TempDir(), "cvs-vet-mig.yaml")
	cvsContent := `id: ` + cvsID + `
kind: convergence_session
title: Vetting migrate integration test session
status: draft
current_phase: c1_scope
outcome_character: pending
delta_assessment: unknown
schema_version: "` + objects.DefaultSchemaVersion + `"
namespace_id: zqk:kernel
hypothesis: |
  integration test for vetting matrix migrate.
desired_end_state: |
  activity_log receives boundary sensor entry.
next_action: |
  run migrate script.
`
	if err := fileutil.WriteSecureFile(cvsYAML, []byte(cvsContent)); err != nil {
		t.Fatalf("write cvs yaml: %v", err)
	}

	runVettingMigrateCLI(t, te, "object", "create", "convergence_session", "--file", cvsYAML)
	// Prove the CVS exists in isolated storage before Python subprocess runs zqk (cwd=repo).
	runVettingMigrateCLI(t, te, "object", "get", cvsID, "--format", "json")

	vetDir := filepath.Join(root, "vetting_migrate_int")
	if err := fileutil.EnsureDir(vetDir); err != nil {
		t.Fatalf("mkdir vetting dir: %v", err)
	}
	activeCSV := filepath.Join(vetDir, "active.csv")
	completeCSV := filepath.Join(vetDir, "complete.csv")
	profileSrc := filepath.Join(projectRoot, paths.DocsQualityDir, "vetting_matrix_profile.yaml")
	profileDst := filepath.Join(vetDir, "vetting_matrix_profile.yaml")
	b, err := fileutil.ReadFile(profileSrc)
	if err != nil {
		t.Fatalf("read profile from repo: %v", err)
	}
	if err := fileutil.WriteSecureFile(profileDst, b); err != nil {
		t.Fatalf("write profile copy: %v", err)
	}

	header := "logical_group,file_path,fully_vetted,fully_refactored_dry,fully_working,cvs_id,last_reviewed_git_sha,notes\n"
	row := "(root),vetting_migrate_int_example.go,yes,yes,yes," + cvsID + ",abc,\n"
	if err := fileutil.WriteSecureFile(activeCSV, []byte(header+row)); err != nil {
		t.Fatalf("write active csv: %v", err)
	}
	if err := fileutil.WriteSecureFile(completeCSV, []byte(header)); err != nil {
		t.Fatalf("write complete csv: %v", err)
	}

	migrateScript := filepath.Join(projectRoot, "scripts", "migrate-vetting-matrix-complete.py")
	pyArgs := []string{
		migrateScript,
		"--active", activeCSV,
		"--complete", completeCSV,
		"--profile", profileDst,
		"--append-cvs-activity",
		"--zqk", te.CLIBinary,
		"--no-log",
	}
	pyCmd := execwrap.Command("python3", pyArgs...)
	wireExecForTest(pyCmd, projectRoot)
	pyCmd.Env = EnvWithTestRoot(root)
	out, err := pyCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("python3 migrate: %v\n%s", err, out)
	}
	t.Logf("migrate stderr+stdout:\n%s", out)

	activeAfter, err := fileutil.ReadFile(activeCSV)
	if err != nil {
		t.Fatalf("read active after migrate: %v", err)
	}
	if strings.Contains(string(activeAfter), "vetting_migrate_int_example.go") {
		t.Fatalf("expected archived row removed from active csv, got:\n%s", activeAfter)
	}

	getOut := runVettingMigrateCLI(t, te, "object", "get", cvsID, "--format", "json")
	obj := parseVettingMigrateJSONObject(t, getOut)
	al, ok := obj[objects.FieldKeyActivityLog].([]any)
	if !ok || len(al) == 0 {
		t.Fatalf("activity_log missing or empty: %#v", obj[objects.FieldKeyActivityLog])
	}
	last, ok := al[len(al)-1].(map[string]any)
	if !ok {
		t.Fatalf("last activity_log entry type %T", al[len(al)-1])
	}
	if last["action"] != "vetting_matrix_rows_archived" {
		t.Fatalf("last activity action: got %v want vetting_matrix_rows_archived (full entry %#v)", last["action"], last)
	}
	notes, _ := last[objects.FieldKeyNotes].(string)
	if !strings.Contains(notes, "profile_id=codebase_vetting_v1") || !strings.Contains(notes, "archived_in_batch=1") {
		t.Fatalf("notes should mention profile and batch: %q", notes)
	}
}

func runVettingMigrateCLI(t *testing.T, te *TestEnvironment, args ...string) []byte {
	t.Helper()
	cmd := te.CreateCLICommand(args...)
	wireExecForTest(cmd, te.GetTestRoot())
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("zqk %v: %v\n%s", args, err, out)
	}
	return out
}

func parseVettingMigrateJSONObject(t *testing.T, data []byte) map[string]any {
	t.Helper()
	s := strings.TrimSpace(string(data))
	start := strings.Index(s, "{")
	if start < 0 {
		t.Fatalf("no JSON object in output:\n%s", data)
	}
	dec := json.NewDecoder(strings.NewReader(s[start:]))
	dec.UseNumber()
	var m map[string]any
	if err := dec.Decode(&m); err != nil {
		t.Fatalf("json decode: %v\noutput:\n%s", err, data)
	}
	return m
}
