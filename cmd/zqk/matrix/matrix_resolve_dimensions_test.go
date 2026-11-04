package matrix

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/testkit"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/verification/matrix"
)

func clearProjectRootEnv(t *testing.T) {
	t.Helper()
	t.Setenv("PROJECT_ROOT", "")
	t.Setenv("ZQK_PROJECT_ROOT", "")
	t.Setenv("TEST_ROOT", "")
	t.Setenv("ZQK_TEST_ROOT", "")
}

func TestCoverageElevation_DimensionsJSON(t *testing.T) {
	isolatedMatrixProject(t, "file_path,fully_vetted\npkg/a.go,yes\n")

	// 1. --json flag
	out, err := executeMatrixCmd(t, []string{"dimensions", "--json"})
	require.NoError(t, err)
	var dims []matrix.DimensionSpec
	err = json.Unmarshal([]byte(out), &dims)
	require.NoError(t, err)
	assert.NotEmpty(t, dims)

	// 2. --format json flag
	outFormat, err := executeMatrixCmd(t, []string{"dimensions", "--format", "json"})
	require.NoError(t, err)
	var dimsFormat []matrix.DimensionSpec
	err = json.Unmarshal([]byte(outFormat), &dimsFormat)
	require.NoError(t, err)
	assert.NotEmpty(t, dimsFormat)
}

func TestCoverageElevation_MatrixCmdContextAndInit(t *testing.T) {
	// 1. cmdContext with nil cmd
	ctx := cmdContext(nil)
	assert.NotNil(t, ctx)

	// 2. cmdContext with cmd lacking context
	emptyCmd := &cobra.Command{}
	ctx2 := cmdContext(emptyCmd)
	assert.NotNil(t, ctx2)

	// 3. initMatrixEngine error with corrupt ledger
	badDir := t.TempDir()
	corruptLedger := filepath.Join(badDir, paths.ProjectDataDir, "verification_matrix.json")
	require.NoError(t, fileutil.MkdirAll(filepath.Dir(corruptLedger), 0755))
	require.NoError(t, fileutil.WriteFile(corruptLedger, []byte("not valid json"), 0644))
	_, err := initMatrixEngine(badDir, nil)
	assert.Error(t, err)

	// 4. persistLedger error handling
	proj := isolatedMatrixProject(t, "file_path,fully_vetted\npkg/a.go,yes\n")
	engine, err := initMatrixEngine(proj, nil)
	require.NoError(t, err)
	ledgerFile := filepath.Join(proj, paths.ProjectDataDir, "verification_matrix.json")
	require.NoError(t, fileutil.Mkdir(ledgerFile, fileutil.StandardDirPerm))
	var buf bytes.Buffer
	testCmd := &cobra.Command{}
	testCmd.SetErr(&buf)
	err = persistLedger(testCmd, engine)
	assert.Error(t, err)
}

func TestCoverageElevation_ResolveProjectRoot(t *testing.T) {
	clearProjectRootEnv(t)

	// 1. Project root from lifecycle context
	projDir := t.TempDir()
	cmd := &cobra.Command{}
	cli.SetContext(cmd, cli.ContextForProjectRoot(""))
	cmd.SetContext(pkgctx.WithLifecycleProjectRoot(context.Background(), projDir))
	root, err := resolveProjectRoot(cmd)
	require.NoError(t, err)
	assert.Equal(t, projDir, root)

	// 2. Project root from cli.Context
	cmd2 := &cobra.Command{}
	cli.SetContext(cmd2, cli.ContextForProjectRoot(projDir))
	root2, err := resolveProjectRoot(cmd2)
	require.NoError(t, err)
	assert.Equal(t, projDir, root2)

	// 3. Project root error when not found
	emptyDir := t.TempDir()
	origWd, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(origWd) }()
	require.NoError(t, os.Chdir(emptyDir))

	cmd3 := &cobra.Command{}
	cli.SetContext(cmd3, cli.ContextForProjectRoot(""))
	_, err = resolveProjectRoot(cmd3)
	assert.Error(t, err)
}

func TestCoverageElevation_ResolveMatrixFlagsAndErrors(t *testing.T) {
	// 1. resolveMatrixTargetFlags on command with flags
	cmd := NewMatrixValidateCmd()
	_ = cmd.Flags().Set("name", "custom_name")
	_ = cmd.Flags().Set("registry", "custom_reg.yaml")
	name, reg, err := resolveMatrixTargetFlags(cmd)
	require.NoError(t, err)
	assert.Equal(t, "custom_name", name)
	assert.Equal(t, "custom_reg.yaml", reg)

	// 2. resolveMatrixFromCmd with project root error
	clearProjectRootEnv(t)
	emptyDir := t.TempDir()
	origWd, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(origWd) }()
	require.NoError(t, os.Chdir(emptyDir))

	cmdNoRoot := NewMatrixGetCmd()
	_, _, err = resolveMatrixFromCmd(cmdNoRoot)
	assert.Error(t, err)
}

func TestCoverageElevation_ReportBranches(t *testing.T) {
	// 1. Registry with no matrices configured (quality.ErrNoMatricesConfigured)
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		SkipFileStorage: true,
		Kind:            "matrix_report_empty",
	})
	dq := filepath.Join(proj.Root, paths.DocsQualityDir)
	require.NoError(t, fileutil.MkdirAll(dq, 0755))
	emptyReg := "schema_version: 1\nmatrices: {}\n"
	require.NoError(t, fileutil.WriteSecureFile(filepath.Join(dq, "matrix_registry.yaml"), []byte(emptyReg)))

	out, err := executeMatrixCmd(t, []string{"report", "--name", "missing"})
	require.NoError(t, err)
	assert.Contains(t, out, "Warning:")

	// 2. Registry with no default matrix (quality.ErrNoDefaultMatrix)
	noDefReg := "schema_version: 1\ndefault_name: \"\"\nmatrices:\n  other:\n    csv: docs/quality/m.csv\n    profile: docs/quality/profile.yaml\n"
	require.NoError(t, fileutil.WriteSecureFile(filepath.Join(dq, "matrix_registry.yaml"), []byte(noDefReg)))
	outNoDef, err := executeMatrixCmd(t, []string{"report"})
	require.NoError(t, err)
	assert.Contains(t, outNoDef, "Warning:")

	// 3. Report error when CSV is corrupt/unreadable
	mPath := filepath.Join(dq, "m.csv")
	profPath := filepath.Join(dq, "profile.yaml")
	// Make m.csv an unreadable directory
	require.NoError(t, fileutil.MkdirAll(mPath, 0755))
	require.NoError(t, fileutil.WriteSecureFile(profPath, []byte("completion:\n  gate_columns: [fully_vetted]\n")))
	_, err = executeMatrixCmd(t, []string{"report", "--matrix", mPath, "--profile", profPath})
	assert.Error(t, err)
}

func TestCoverageElevation_ListBranches(t *testing.T) {
	isolatedMatrixProject(t, "file_path,fully_vetted\npkg/a.go,pending\n")

	// 1. Non-existent registry flag
	_, err := executeMatrixCmd(t, []string{"list", "--registry", "nonexistent_registry.yaml"})
	assert.Error(t, err)

	// 2. List with missing project root
	clearProjectRootEnv(t)
	emptyDir := t.TempDir()
	origWd, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(origWd) }()
	require.NoError(t, os.Chdir(emptyDir))

	cmd := NewMatrixListCmd()
	cli.SetContext(cmd, cli.ContextForProjectRoot(""))
	err = runMatrixList(cmd, nil)
	assert.Error(t, err)
}

func TestCoverageElevation_GetBranches(t *testing.T) {
	isolatedMatrixProject(t, "file_path,fully_vetted\npkg/a.go,pending\n")

	// 1. Malformed filter
	_, err := executeMatrixCmd(t, []string{"get", "--filter", "invalid_no_equal_sign"})
	assert.Error(t, err)

	// 2. Query error with invalid column
	_, err = executeMatrixCmd(t, []string{"get", "--field", "nonexistent_column_for_matrix"})
	assert.Error(t, err)

	// 3. Get with missing project root
	clearProjectRootEnv(t)
	emptyDir := t.TempDir()
	origWd, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(origWd) }()
	require.NoError(t, os.Chdir(emptyDir))

	cmd := NewMatrixGetCmd()
	cli.SetContext(cmd, cli.ContextForProjectRoot(""))
	err = runMatrixGet(cmd, nil)
	assert.Error(t, err)
}

func TestCoverageElevation_ValidateBranches(t *testing.T) {
	isolatedMatrixProject(t, "file_path,fully_vetted\npkg/a.go,pending\n")

	// 1. Non-existent registry flag
	_, err := executeMatrixCmd(t, []string{"validate", "--registry", "nonexistent_registry.yaml"})
	assert.Error(t, err)

	// 2. Validate with missing project root
	clearProjectRootEnv(t)
	emptyDir := t.TempDir()
	origWd, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(origWd) }()
	require.NoError(t, os.Chdir(emptyDir))

	cmd := NewMatrixValidateCmd()
	cli.SetContext(cmd, cli.ContextForProjectRoot(""))
	err = runMatrixValidate(cmd, nil)
	assert.Error(t, err)
}

func TestCoverageElevation_InventoryBranches(t *testing.T) {
	// Missing project root
	clearProjectRootEnv(t)
	emptyDir := t.TempDir()
	origWd, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(origWd) }()
	require.NoError(t, os.Chdir(emptyDir))

	cmd := NewMatrixInventoryCmd()
	cli.SetContext(cmd, cli.ContextForProjectRoot(""))
	err = runMatrixInventory(cmd, nil)
	assert.Error(t, err)
}

func TestCoverageElevation_StampBranches(t *testing.T) {
	projRoot := isolatedMatrixProject(t, "file_path,fully_vetted\npkg/a.go,pending\n")
	testFile := filepath.Join(projRoot, "pkg", "a.go")
	require.NoError(t, fileutil.MkdirAll(filepath.Dir(testFile), 0755))
	require.NoError(t, fileutil.WriteFile(testFile, []byte("package pkg\n"), 0644))

	// 1. Missing both --dimension and --check
	_, err := executeMatrixCmd(t, []string{"stamp", "--file", "pkg/a.go"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "either --dimension or --check must be specified")

	// 2. Using --agent alias with --feedback
	out, err := executeMatrixCmd(t, []string{
		"stamp",
		"--file", "pkg/a.go",
		"--dimension", "HCODE",
		"--agent", "PER-COMMUNITY-TESTER",
		"--status", "passed",
		"--feedback", "all checks verified",
	})
	require.NoError(t, err)
	assert.Contains(t, out, "Feedback")
	assert.Contains(t, out, "all checks verified")

	// 3. Expected hash mismatch error
	_, err = executeMatrixCmd(t, []string{
		"stamp",
		"--file", "pkg/a.go",
		"--dimension", "HCODE",
		"--hash", "0000000000000000000000000000000000000000000000000000000000000000",
		"--status", "passed",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "programmatic matrix stamp failed")

	// 4. Missing project root
	clearProjectRootEnv(t)
	emptyDir := t.TempDir()
	origWd, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(origWd) }()
	require.NoError(t, os.Chdir(emptyDir))

	cmd := NewMatrixStampCmd()
	cli.SetContext(cmd, cli.ContextForProjectRoot(""))
	err = runMatrixStamp(cmd, nil)
	assert.Error(t, err)
}

func TestCoverageElevation_EvaluateComprehensive(t *testing.T) {
	projRoot := isolatedMatrixProject(t, "file_path,fully_vetted\npkg/clean.go,yes\npkg/bad.go,pending\n")
	cleanFile := filepath.Join(projRoot, "pkg", "clean.go")
	require.NoError(t, fileutil.MkdirAll(filepath.Dir(cleanFile), 0755))
	require.NoError(t, fileutil.WriteFile(cleanFile, []byte("package pkg\n\nfunc Clean() {}\n"), 0644))

	badFile := filepath.Join(projRoot, "pkg", "bad.go")
	badContent := "package pkg\n\nconst Version = \"v1.0.0\"\nconst Perm = 0777\n"
	require.NoError(t, fileutil.WriteFile(badFile, []byte(badContent), 0644))

	// Setup mock zqk binary for remediation
	require.NoError(t, fileutil.MkdirAll(filepath.Join(projRoot, "bin"), 0755))
	mockBin := filepath.Join(projRoot, "bin", "zqk")
	script := `#!/bin/sh
if [ "$3" = "technical_debt" ]; then
  echo "Created technical_debt TDE-99999 successfully"
elif [ "$3" = "backlog_item" ]; then
  echo "Created backlog_item BLI-88888 successfully"
fi
exit 0
`
	require.NoError(t, fileutil.WriteFile(mockBin, []byte(script), 0755))

	// 1. Evaluate clean file with --json (passes)
	outCleanJSON, err := executeMatrixCmd(t, []string{"evaluate", "--file", "pkg/clean.go", "--json"})
	require.NoError(t, err)
	assert.Contains(t, outCleanJSON, "file_path")

	// 2. Evaluate bad file with --json (fails)
	_, err = executeMatrixCmd(t, []string{"evaluate", "--file", "pkg/bad.go", "--json"})
	assert.Error(t, err)

	// 3. Evaluate bad file with custom dimension and custom evaluator without auto-mint
	_, err = executeMatrixCmd(t, []string{
		"evaluate",
		"--file", "pkg/bad.go",
		"--dimension", "EFFPERF",
		"--evaluator", "PER-CUSTOM-TESTER",
		"--auto-mint=false",
	})
	assert.Error(t, err)

	// 4. Evaluate bad file with auto-mint enabled (covers remediation output lines 124-131)
	_, err = executeMatrixCmd(t, []string{
		"evaluate",
		"--file", "pkg/bad.go",
		"--auto-mint=true",
	})
	assert.Error(t, err)

	// 5. Evaluate non-existent file returns error
	_, err = executeMatrixCmd(t, []string{"evaluate", "--file", "pkg/nonexistent.go"})
	assert.Error(t, err)

	// 6. Missing project root
	clearProjectRootEnv(t)
	emptyDir := t.TempDir()
	origWd, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(origWd) }()
	require.NoError(t, os.Chdir(emptyDir))

	cmd := NewMatrixEvaluateCmd()
	cli.SetContext(cmd, cli.ContextForProjectRoot(""))
	err = runMatrixEvaluate(cmd, nil)
	assert.Error(t, err)
}

func TestCoverageElevation_VerifyBranches(t *testing.T) {
	projRoot := isolatedMatrixProject(t, "file_path,fully_vetted\npkg/clean.go,yes\n")
	cleanFile := filepath.Join(projRoot, "pkg", "clean.go")
	require.NoError(t, fileutil.MkdirAll(filepath.Dir(cleanFile), 0755))
	require.NoError(t, fileutil.WriteFile(cleanFile, []byte("package pkg\n\nfunc Clean() {}\n"), 0644))

	// 1. Single file evaluation error (non-existent file)
	cmdVerifyFile := NewMatrixVerifyCmd()
	cmdVerifyFile.Flags().Set("file", "pkg/nonexistent.go")
	cmdVerifyFile.SetContext(pkgctx.WithLifecycleProjectRoot(context.Background(), projRoot))
	err := runMatrixVerify(cmdVerifyFile, nil)
	assert.Error(t, err)

	// 2. Full verify with violating file
	badFile := filepath.Join(projRoot, "pkg", "bad.go")
	badContent := "package pkg\n\nconst Version = \"v1.0.0\"\nconst Perm = 0777\n"
	require.NoError(t, fileutil.WriteFile(badFile, []byte(badContent), 0644))
	cmdVerifyAll := NewMatrixVerifyCmd()
	cmdVerifyAll.SetContext(pkgctx.WithLifecycleProjectRoot(context.Background(), projRoot))
	err = runMatrixVerify(cmdVerifyAll, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "verification matrix found 1 violating files")

	// 3. Missing project root
	clearProjectRootEnv(t)
	emptyDir := t.TempDir()
	origWd, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(origWd) }()
	require.NoError(t, os.Chdir(emptyDir))

	cmd := NewMatrixVerifyCmd()
	cli.SetContext(cmd, cli.ContextForProjectRoot(""))
	err = runMatrixVerify(cmd, nil)
	assert.Error(t, err)
}

func TestCoverageElevation_UpdateAdvanced(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, nil)
	writeMinimalMatrixProject(t, proj.Root, "file_path,bundle_label,fully_vetted,session_id\npkg/a.go,b1,pending,\n")

	// 1. Error: --append-cvs-activity with --dry-run
	_, err := executeMatrixCmd(t, []string{
		"update", "--name", "t",
		"--file-path", "pkg/a.go",
		"--set", "fully_vetted=yes",
		"--append-cvs-activity",
		"--dry-run",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "--append-cvs-activity cannot be used with --dry-run")

	// 2. Error: --cvs-id without --append-cvs-activity
	_, err = executeMatrixCmd(t, []string{
		"update", "--name", "t",
		"--file-path", "pkg/a.go",
		"--set", "fully_vetted=yes",
		"--cvs-id", "CVS-001",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "--cvs-id requires --append-cvs-activity")

	// 3. Error: malformed filter
	_, err = executeMatrixCmd(t, []string{
		"update", "--name", "t",
		"--filter", "invalid_no_equal",
		"--set", "fully_vetted=yes",
	})
	assert.Error(t, err)

	// 4. validateCvsIDsReadable with non-convergence session object
	sec := pkgctx.NewSystemSecurityContext()
	ctx := context.Background()
	taskObj := map[string]any{
		objects.FieldKeyKind:          objects.KindBacklogItem,
		objects.FieldKeyID:            "BLI-001",
		objects.FieldKeyTitle:         "Sample BLI",
		objects.FieldKeyDescription:   "Valid description",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}
	require.NoError(t, proj.FileStorage.Create(pkgctx.WithPromoteOnCreate(ctx), sec, taskObj))

	cmd := NewMatrixUpdateCmd()
	cmd.SetContext(ctx)
	cli.SetContext(cmd, cli.ContextForProjectRoot(proj.Root))

	err = validateCvsIDsReadable(cmd, []string{"BLI-001"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "expected convergence_session")

	// 5. Append activity error handling when CVS update fails
	csvPath := filepath.Join(proj.Root, paths.DocsQualityDir, "m.csv")
	_, err = appendMatrixCvsActivity(cmd, "t", csvPath, false, 1, map[string]string{"fully_vetted": "yes"}, []string{"CVS-NONEXISTENT"}, "", nil, nil)
	assert.Error(t, err)
}

func TestCoverageElevation_RemediationFallback(t *testing.T) {
	// Hook with empty dimension defaults to HCODE
	hook := NewZQKRemediationHook(t.TempDir())
	sc := &matrix.FileScorecard{
		FilePath:  "pkg/demo/test.go",
		Dimension: "", // triggers fallback to HCODE
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Will fail execution since bin/zqk is missing in temp dir, but covers lines 30-46
	_, err := hook.OnPolicyFailure(ctx, sc, []string{"test failure"})
	assert.Error(t, err)
}
