package organizational

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func captureStdoutCliOrg(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = w
	t.Cleanup(func() { os.Stdout = old })
	var buf bytes.Buffer
	done := make(chan struct{})
	goroutinelabels.NewGoroutine("organizational", "read stdout cli").StartSimple(func() {
		_, _ = buf.ReadFrom(r)
		close(done)
	})
	fn()
	_ = w.Close()
	<-done
	return buf.String()
}

func setupOrgCliWorkspace(t *testing.T) (string, storage.ObjectStorageProvider, func()) {
	t.Helper()
	testRoot, _ := setupIntegrationTestWithSpecsOrganizational(t, "cmd-org-cli")

	store, err := storage.NewFileObjectStorageForTest(testRoot)
	if err != nil {
		t.Fatalf("NewFileObjectStorageForTest: %v", err)
	}
	cli.RegisterStorageForProjectRoot(testRoot, store)

	origDir, err := fileutil.Getwd()
	if err != nil {
		t.Fatalf("getwd failed: %v", err)
	}

	if err := fileutil.Chdir(testRoot); err != nil {
		t.Fatalf("chdir failed: %v", err)
	}

	cleanup := func() {
		_ = store.Close()
		cli.UnregisterStorageForProjectRoot(testRoot)
		_ = fileutil.Chdir(origDir)
	}
	t.Cleanup(cleanup)

	return testRoot, store, cleanup
}

func configureOrgCliCmd(cmd *cobra.Command, testRoot string) {
	ctx := pkgctx.NewSystemContext()
	store, ok := cli.GetObjectStorageForProjectRoot(testRoot)
	if ok && store != nil {
		ctx = cli.WithStorageProvider(ctx, store)
	}
	cmd.SetContext(ctx)
	cli.SetContext(cmd, cli.ContextForProjectRoot(testRoot))
}

func TestOrganizational_RootCommandTree(t *testing.T) {
	cmd := NewOrganizationalCmd()
	if cmd == nil {
		t.Fatal("expected NewOrganizationalCmd to return non-nil command")
	}
	if cmd.Use != "organizational" {
		t.Errorf("expected command Use to be 'organizational', got %q", cmd.Use)
	}

	expectedSubcommands := []string{"analyze-impact", "propagate", "record-change", "sync"}
	subCmdMap := make(map[string]bool)
	for _, sub := range cmd.Commands() {
		subCmdMap[sub.Name()] = true
	}

	for _, exp := range expectedSubcommands {
		if !subCmdMap[exp] {
			t.Errorf("expected subcommand %q under organizational", exp)
		}
	}

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("failed to execute organizational --help: %v", err)
	}
	if !strings.Contains(buf.String(), "Organizational operations for managing organizational structure") {
		t.Errorf("expected help output to describe organizational commands, got:\n%s", buf.String())
	}
}

func TestOrganizationalSync_ValidationAndExecution(t *testing.T) {
	testRoot, _, _ := setupOrgCliWorkspace(t)

	// 1. Missing --file flag
	cmd1 := NewSyncCmd()
	cmd1.SetArgs([]string{})
	configureOrgCliCmd(cmd1, testRoot)
	var err1 error
	captureStdoutCliOrg(t, func() { err1 = cmd1.Execute() })
	if err1 == nil || !strings.Contains(err1.Error(), "--file is required") {
		t.Errorf("expected '--file is required' error, got %v", err1)
	}

	// 2. Non-existent file
	cmd2 := NewSyncCmd()
	cmd2.SetArgs([]string{"--file", filepath.Join(testRoot, "missing.yaml")})
	configureOrgCliCmd(cmd2, testRoot)
	var err2 error
	captureStdoutCliOrg(t, func() { err2 = cmd2.Execute() })
	if err2 == nil || !strings.Contains(err2.Error(), "failed to read file") {
		t.Errorf("expected 'failed to read file' error, got %v", err2)
	}

	// 3. Unsupported format
	emptyFile := filepath.Join(testRoot, "empty.txt")
	if err := fileutil.WriteFile(emptyFile, []byte("test"), paths.FilePerm600); err != nil {
		t.Fatalf("write file error: %v", err)
	}
	cmd3 := NewSyncCmd()
	cmd3.SetArgs([]string{"--file", emptyFile, "--input-format", "xml"})
	configureOrgCliCmd(cmd3, testRoot)
	var err3 error
	captureStdoutCliOrg(t, func() { err3 = cmd3.Execute() })
	if err3 == nil || !strings.Contains(err3.Error(), "unsupported format") {
		t.Errorf("expected 'unsupported format' error, got %v", err3)
	}

	// 4. Parse error
	badYamlFile := filepath.Join(testRoot, "bad.yaml")
	if err := fileutil.WriteFile(badYamlFile, []byte(":\n  invalid"), paths.FilePerm600); err != nil {
		t.Fatalf("write file error: %v", err)
	}
	cmd4 := NewSyncCmd()
	cmd4.SetArgs([]string{"--file", badYamlFile})
	configureOrgCliCmd(cmd4, testRoot)
	var err4 error
	captureStdoutCliOrg(t, func() { err4 = cmd4.Execute() })
	if err4 == nil || !strings.Contains(err4.Error(), "failed to parse file") {
		t.Errorf("expected 'failed to parse file' error, got %v", err4)
	}

	// 5. Object missing kind
	noKindYaml := filepath.Join(testRoot, "nokind.yaml")
	if err := fileutil.WriteFile(noKindYaml, []byte("- id: ORG-001\n  title: Missing Kind"), paths.FilePerm600); err != nil {
		t.Fatalf("write file error: %v", err)
	}
	cmd5 := NewSyncCmd()
	cmd5.SetArgs([]string{"--file", noKindYaml})
	configureOrgCliCmd(cmd5, testRoot)
	var err5 error
	captureStdoutCliOrg(t, func() { err5 = cmd5.Execute() })
	if err5 == nil || !strings.Contains(err5.Error(), "missing kind") {
		t.Errorf("expected 'missing kind' error, got %v", err5)
	}

	// 6. Object kind not allowed
	disallowedKindYaml := filepath.Join(testRoot, "disallowed.yaml")
	if err := fileutil.WriteFile(disallowedKindYaml, []byte("- id: BLI-001\n  kind: backlog_item\n  title: Item"), paths.FilePerm600); err != nil {
		t.Fatalf("write file error: %v", err)
	}
	cmd6 := NewSyncCmd()
	cmd6.SetArgs([]string{"--file", disallowedKindYaml})
	configureOrgCliCmd(cmd6, testRoot)
	var err6 error
	captureStdoutCliOrg(t, func() { err6 = cmd6.Execute() })
	if err6 == nil || !strings.Contains(err6.Error(), "is not allowed for organizational sync") {
		t.Errorf("expected disallowed kind error, got %v", err6)
	}

	// 7. Valid org structure sync with --dry-run
	validYaml := filepath.Join(testRoot, "valid_org.yaml")
	validContent := `
- id: ORG-888
  kind: organization
  namespace_id: domain:organizational
  schema_version: "` + objects.DefaultSchemaVersion + `"
  title: Test Organization
  status: active
  domain: custom
  spec_interpreter: default
  spec_context_broker: default
  organization_name: Test Organization
  division_refs: []
`
	if err := fileutil.WriteFile(validYaml, []byte(validContent), paths.FilePerm600); err != nil {
		t.Fatalf("write valid yaml error: %v", err)
	}
	cmd7 := NewSyncCmd()
	cmd7.SetArgs([]string{"--file", validYaml, "--dry-run"})
	configureOrgCliCmd(cmd7, testRoot)
	var err7 error
	captureStdoutCliOrg(t, func() { err7 = cmd7.Execute() })
	if err7 != nil && !strings.Contains(err7.Error(), "no builders found for ontology") {
		t.Fatalf("valid sync dry-run returned error: %v", err7)
	}

	// 8. Valid org structure sync execution
	cmd8 := NewSyncCmd()
	cmd8.SetArgs([]string{"--file", validYaml, "--mode", "upsert"})
	configureOrgCliCmd(cmd8, testRoot)
	var err8 error
	captureStdoutCliOrg(t, func() { err8 = cmd8.Execute() })
	if err8 != nil && !strings.Contains(err8.Error(), "no builders found for ontology") {
		t.Fatalf("valid sync execution returned error: %v", err8)
	}
}

func TestOrganizationalRecordChange_ValidationAndExecution(t *testing.T) {
	testRoot, _, _ := setupOrgCliWorkspace(t)

	// 1. Missing required flags
	cmd1 := NewRecordChangeCmd()
	cmd1.SetArgs([]string{})
	configureOrgCliCmd(cmd1, testRoot)
	var err1 error
	captureStdoutCliOrg(t, func() { err1 = cmd1.Execute() })
	if err1 == nil || !strings.Contains(err1.Error(), "--id and --change-type are required") {
		t.Errorf("expected required flags error, got %v", err1)
	}

	// 2. Non-existent affected-objects file
	cmd2 := NewRecordChangeCmd()
	cmd2.SetArgs([]string{"--id", "OCH-001", "--change-type", "restructure", "--affected-objects", "no_file.json"})
	configureOrgCliCmd(cmd2, testRoot)
	var err2 error
	captureStdoutCliOrg(t, func() { err2 = cmd2.Execute() })
	if err2 == nil || !strings.Contains(err2.Error(), "failed to read affected-objects file") {
		t.Errorf("expected failed to read affected-objects error, got %v", err2)
	}

	// 3. Invalid JSON/YAML in affected-objects file (not a map)
	invalidAffectedFile := filepath.Join(testRoot, "invalid_affected.txt")
	if err := fileutil.WriteFile(invalidAffectedFile, []byte("- not\n- a\n- map\n"), paths.FilePerm600); err != nil {
		t.Fatalf("write file error: %v", err)
	}
	cmd3 := NewRecordChangeCmd()
	cmd3.SetArgs([]string{"--id", "OCH-001", "--change-type", "restructure", "--affected-objects", invalidAffectedFile})
	configureOrgCliCmd(cmd3, testRoot)
	var err3 error
	captureStdoutCliOrg(t, func() { err3 = cmd3.Execute() })
	if err3 == nil || !strings.Contains(err3.Error(), "affected-objects file must be YAML or JSON map") {
		t.Errorf("expected invalid format error, got %v", err3)
	}

	// 4. Valid affected-objects file and dry-run
	validAffectedFile := filepath.Join(testRoot, "affected.json")
	if err := fileutil.WriteFile(validAffectedFile, []byte(`{"teams": ["TEAM-001"]}`), paths.FilePerm600); err != nil {
		t.Fatalf("write file error: %v", err)
	}
	cmd4 := NewRecordChangeCmd()
	cmd4.SetArgs([]string{
		"--id", "OCH-002",
		"--change-type", "team_reassignment",
		"--title", "Reassign Team",
		"--description", "Moving team to new division",
		"--affected-objects", validAffectedFile,
		"--change-date", "2026-10-14T00:00:00Z",
		"--dry-run",
	})
	configureOrgCliCmd(cmd4, testRoot)
	var err4 error
	captureStdoutCliOrg(t, func() { err4 = cmd4.Execute() })
	if err4 != nil {
		t.Fatalf("dry-run returned error: %v", err4)
	}

	// 5. Real change recording
	cmd5 := NewRecordChangeCmd()
	cmd5.SetArgs([]string{
		"--id", "OCH-002",
		"--change-type", "team_reassignment",
		"--title", "Reassign Team",
		"--description", "Moving team to new division",
		"--affected-objects", validAffectedFile,
	})
	configureOrgCliCmd(cmd5, testRoot)
	var err5 error
	out5 := captureStdoutCliOrg(t, func() { err5 = cmd5.Execute() })
	if err5 != nil {
		t.Fatalf("record-change execution failed: %v", err5)
	}
	if !strings.Contains(out5, "Organizational change recorded") {
		t.Errorf("expected output to contain 'Organizational change recorded', got:\n%s", out5)
	}
}

func TestOrganizationalAnalyzeImpactAndPropagate_Execution(t *testing.T) {
	testRoot, store, _ := setupOrgCliWorkspace(t)

	// 1. Missing --change on analyze-impact
	cmdImpactErr := NewAnalyzeImpactCmd()
	cmdImpactErr.SetArgs([]string{})
	configureOrgCliCmd(cmdImpactErr, testRoot)
	var errImpact error
	captureStdoutCliOrg(t, func() { errImpact = cmdImpactErr.Execute() })
	if errImpact == nil || !strings.Contains(errImpact.Error(), "--change is required") {
		t.Errorf("expected '--change is required' error, got %v", errImpact)
	}

	// 2. Missing --change on propagate
	cmdPropErr := NewPropagateCmd()
	cmdPropErr.SetArgs([]string{})
	configureOrgCliCmd(cmdPropErr, testRoot)
	var errProp error
	captureStdoutCliOrg(t, func() { errProp = cmdPropErr.Execute() })
	if errProp == nil || !strings.Contains(errProp.Error(), "--change is required") {
		t.Errorf("expected '--change is required' error, got %v", errProp)
	}

	// 3. Propagate on non-existent change
	cmdPropMissing := NewPropagateCmd()
	cmdPropMissing.SetArgs([]string{"--change", "OCH-NONEXISTENT"})
	configureOrgCliCmd(cmdPropMissing, testRoot)
	var errPropMissing error
	captureStdoutCliOrg(t, func() { errPropMissing = cmdPropMissing.Execute() })
	if errPropMissing == nil || !strings.Contains(errPropMissing.Error(), "failed to read organizational change") {
		t.Errorf("expected failed to read error, got %v", errPropMissing)
	}

	// 4. Record a valid change first
	changeID := "OCH-777"
	cmdRecord := NewRecordChangeCmd()
	cmdRecord.SetArgs([]string{
		"--id", changeID,
		"--change-type", "restructure",
		"--title", "Org Restructure",
		"--description", "Major restructure",
	})
	configureOrgCliCmd(cmdRecord, testRoot)
	var errRecord error
	captureStdoutCliOrg(t, func() { errRecord = cmdRecord.Execute() })
	if errRecord != nil {
		t.Fatalf("record-change failed: %v", errRecord)
	}

	// 5. Analyze impact on the recorded change
	cmdAnalyze := NewAnalyzeImpactCmd()
	cmdAnalyze.SetArgs([]string{"--change", changeID})
	configureOrgCliCmd(cmdAnalyze, testRoot)
	var errAnalyze error
	outAnalyze := captureStdoutCliOrg(t, func() { errAnalyze = cmdAnalyze.Execute() })
	if errAnalyze != nil {
		t.Fatalf("analyze-impact failed: %v", errAnalyze)
	}
	if !strings.Contains(outAnalyze, "Impact analysis created") {
		t.Errorf("expected analyze-impact output to contain 'Impact analysis created', got:\n%s", outAnalyze)
	}

	// 6. Propagate change with dry-run
	cmdPropDry := NewPropagateCmd()
	cmdPropDry.SetArgs([]string{"--change", changeID, "--dry-run"})
	configureOrgCliCmd(cmdPropDry, testRoot)
	var errPropDry error
	outPropDry := captureStdoutCliOrg(t, func() { errPropDry = cmdPropDry.Execute() })
	if errPropDry != nil {
		t.Fatalf("propagate dry-run failed: %v", errPropDry)
	}
	_ = outPropDry

	// 7. Propagate change without confirm
	cmdPropNoConfirm := NewPropagateCmd()
	cmdPropNoConfirm.SetArgs([]string{"--change", changeID})
	configureOrgCliCmd(cmdPropNoConfirm, testRoot)
	var errPropNoConfirm error
	outPropNoConfirm := captureStdoutCliOrg(t, func() { errPropNoConfirm = cmdPropNoConfirm.Execute() })
	if errPropNoConfirm != nil {
		t.Fatalf("propagate no-confirm failed: %v", errPropNoConfirm)
	}
	_ = outPropNoConfirm

	// 8. Propagate change with --confirm
	cmdPropConfirm := NewPropagateCmd()
	cmdPropConfirm.SetArgs([]string{"--change", changeID, "--confirm"})
	configureOrgCliCmd(cmdPropConfirm, testRoot)
	var errPropConfirm error
	outPropConfirm := captureStdoutCliOrg(t, func() { errPropConfirm = cmdPropConfirm.Execute() })
	if errPropConfirm != nil {
		t.Fatalf("propagate --confirm failed: %v", errPropConfirm)
	}
	_ = outPropConfirm

	// 9. Propagate on object that is not an organizational_change
	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()
	notChangeObj := map[string]any{
		objects.FieldKeyID:            "BLI-CLI-TEST-001",
		objects.FieldKeyKind:          objects.KindBacklogItem,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyTitle:         "Not an org change",
		objects.FieldKeyDescription:   "Valid description for non-change item",
		objects.FieldKeyStatus:        objects.ObjectStatusValidated,
	}
	if err := store.Create(ctx, secCtx, notChangeObj); err != nil {
		t.Fatalf("create backlog item failed: %v", err)
	}

	cmdPropNotChange := NewPropagateCmd()
	cmdPropNotChange.SetArgs([]string{"--change", "BLI-CLI-TEST-001"})
	configureOrgCliCmd(cmdPropNotChange, testRoot)
	var errPropNotChange error
	captureStdoutCliOrg(t, func() { errPropNotChange = cmdPropNotChange.Execute() })
	if errPropNotChange == nil || !strings.Contains(errPropNotChange.Error(), "is not an organizational_change") {
		t.Errorf("expected not an organizational_change error, got %v", errPropNotChange)
	}

	// 10. Record change with affected objects and propagate with dry-run, preview, and confirm
	affectedFile := filepath.Join(testRoot, "affected_teams.json")
	if err := fileutil.WriteFile(affectedFile, []byte(`{"team": ["TEAM-001"]}`), paths.FilePerm600); err != nil {
		t.Fatalf("write affected teams file: %v", err)
	}
	changeWithImpactID := "OCH-779"
	cmdRecordWithImpact := NewRecordChangeCmd()
	cmdRecordWithImpact.SetArgs([]string{
		"--id", changeWithImpactID,
		"--change-type", "team_reassignment",
		"--title", "Reassign Team With Impact",
		"--description", "Moving team",
		"--affected-objects", affectedFile,
	})
	configureOrgCliCmd(cmdRecordWithImpact, testRoot)
	var errRecordWithImpact error
	captureStdoutCliOrg(t, func() { errRecordWithImpact = cmdRecordWithImpact.Execute() })
	if errRecordWithImpact != nil {
		t.Fatalf("record change with impact failed: %v", errRecordWithImpact)
	}

	// Analyze impact to generate impact_analysis
	cmdAnalyzeWithImpact := NewAnalyzeImpactCmd()
	cmdAnalyzeWithImpact.SetArgs([]string{"--change", changeWithImpactID})
	configureOrgCliCmd(cmdAnalyzeWithImpact, testRoot)
	var errAnalyzeWithImpact error
	outAnalyzeWithImpact := captureStdoutCliOrg(t, func() { errAnalyzeWithImpact = cmdAnalyzeWithImpact.Execute() })
	if errAnalyzeWithImpact != nil {
		t.Fatalf("analyze impact failed: %v", errAnalyzeWithImpact)
	}
	if !strings.Contains(outAnalyzeWithImpact, "Impact analysis created") {
		t.Errorf("expected impact analysis output, got: %s", outAnalyzeWithImpact)
	}

	// Propagate with empty affected ZQK objects outputs "No affected objects"
	cmdPropEmpty := NewPropagateCmd()
	cmdPropEmpty.SetArgs([]string{"--change", changeWithImpactID})
	configureOrgCliCmd(cmdPropEmpty, testRoot)
	var errPropEmpty error
	outPropEmpty := captureStdoutCliOrg(t, func() { errPropEmpty = cmdPropEmpty.Execute() })
	if errPropEmpty != nil {
		t.Fatalf("propagate empty failed: %v", errPropEmpty)
	}
	if !strings.Contains(outPropEmpty, "No affected objects for change") {
		t.Errorf("expected 'No affected objects' message, got: %s", outPropEmpty)
	}

	// Update impact analysis object in storage to include an affected object so preview, dry-run, and confirm can be tested
	var iaID string
	for _, l := range strings.Split(strings.TrimSpace(outAnalyzeWithImpact), "\n") {
		if strings.HasPrefix(l, "Impact analysis created: ") {
			iaID = strings.TrimSpace(strings.TrimPrefix(l, "Impact analysis created: "))
			break
		}
	}
	if iaID == "" {
		t.Fatalf("could not parse impact analysis ID from output: %s", outAnalyzeWithImpact)
	}
	iaObj, err := store.Read(ctx, secCtx, iaID)
	if err != nil {
		t.Fatalf("read impactObj: %v", err)
	}
	iaObj[objects.FieldKeyDescription] = "Impact analysis description for testing"
	iaObj[objects.FieldKeyAffectedObjects] = map[string]any{
		"backlog_items": []any{"BLI-CLI-TEST-001"},
	}
	if err := store.Update(ctx, secCtx, iaID, iaObj); err != nil {
		t.Fatalf("update impactObj with affected items: %v", err)
	}
	// Propagate preview mode
	cmdPropPreview := NewPropagateCmd()
	cmdPropPreview.SetArgs([]string{"--change", changeWithImpactID, "--impact-analysis", iaID})
	configureOrgCliCmd(cmdPropPreview, testRoot)
	var errPropPreview error
	outPropPreview := captureStdoutCliOrg(t, func() { errPropPreview = cmdPropPreview.Execute() })
	if errPropPreview != nil {
		t.Fatalf("propagate preview failed: %v", errPropPreview)
	}
	if !strings.Contains(outPropPreview, "Use --confirm to update") {
		t.Errorf("expected preview prompt in output, got: %s", outPropPreview)
	}

	// Propagate dry-run mode
	cmdPropDryRun := NewPropagateCmd()
	cmdPropDryRun.SetArgs([]string{"--change", changeWithImpactID, "--impact-analysis", iaID, "--dry-run"})
	configureOrgCliCmd(cmdPropDryRun, testRoot)
	var errPropDryRun error
	outPropDryRun := captureStdoutCliOrg(t, func() { errPropDryRun = cmdPropDryRun.Execute() })
	if errPropDryRun != nil {
		t.Fatalf("propagate dry-run failed: %v", errPropDryRun)
	}
	if !strings.Contains(outPropDryRun, "Dry run:") {
		t.Errorf("expected dry run message in output, got: %s", outPropDryRun)
	}

	// Propagate confirm mode
	cmdPropApply := NewPropagateCmd()
	cmdPropApply.SetArgs([]string{"--change", changeWithImpactID, "--impact-analysis", iaID, "--confirm"})
	configureOrgCliCmd(cmdPropApply, testRoot)
	var errPropApply error
	outPropApply := captureStdoutCliOrg(t, func() { errPropApply = cmdPropApply.Execute() })
	if errPropApply != nil {
		t.Fatalf("propagate confirm failed: %v", errPropApply)
	}
	if !strings.Contains(outPropApply, "Change propagation complete") {
		t.Errorf("expected complete message in output, got: %s", outPropApply)
	}
}
