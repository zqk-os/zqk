package matrix

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	clitool "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/quality"
	"github.com/zqk-os/zqk/pkg/testkit"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/verification/matrix"
)

func executeMatrixCmd(t *testing.T, args []string) (string, error) {
	t.Helper()
	var buf bytes.Buffer
	root := clitool.NewCommandBuilder("zqk").Build()
	root.PersistentFlags().StringP("format", "f", "", "Output format")
	root.AddCommand(NewMatrixCmd())
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &buf))
	root.SetArgs(append([]string{"matrix"}, args...))
	err := root.Execute()
	return buf.String(), err
}

func TestMatrixListCommand(t *testing.T) {
	isolatedMatrixProject(t, "file_path,fully_vetted\npkg/a.go,pending\n")

	out, err := executeMatrixCmd(t, []string{"list"})
	require.NoError(t, err)
	assert.Contains(t, out, "t")
}

func TestMatrixReportCommand(t *testing.T) {
	isolatedMatrixProject(t, "file_path,fully_vetted\npkg/a.go,pending\npkg/b.go,yes\n")

	// 1. Default report
	out, err := executeMatrixCmd(t, []string{"report"})
	require.NoError(t, err)
	assert.Contains(t, out, "t")

	// 2. Go-only flag
	out, err = executeMatrixCmd(t, []string{"report", "--go-only"})
	require.NoError(t, err)
	assert.Contains(t, out, "t")
}

func TestMatrixReportMissingFile(t *testing.T) {
	projRoot := isolatedMatrixProject(t, "file_path,fully_vetted\npkg/a.go,pending\n")

	// Delete the CSV file to test missing file fallback
	csvPath := filepath.Join(projRoot, "docs", "quality", "m.csv")
	require.NoError(t, fileutil.Remove(csvPath))

	out, err := executeMatrixCmd(t, []string{"report"})
	require.NoError(t, err)
	assert.Contains(t, out, "matrix file missing")
}

func TestMatrixValidateCommand(t *testing.T) {
	isolatedMatrixProject(t, "file_path,fully_vetted\npkg/a.go,pending\n")
	out, err := executeMatrixCmd(t, []string{"validate"})
	require.NoError(t, err)
	assert.Contains(t, out, "t")
}

func TestMatrixValidateFailure(t *testing.T) {
	projRoot := isolatedMatrixProject(t, "invalid_header,random_column\n1,2\n")

	// Delete profile to cause validation failure
	profilePath := filepath.Join(projRoot, "docs", "quality", "profile.yaml")
	_ = fileutil.Remove(profilePath)

	_, err := executeMatrixCmd(t, []string{"validate"})
	assert.Error(t, err)
}

func TestMatrixInventoryCommand(t *testing.T) {
	projRoot := isolatedMatrixProject(t, "file_path,fully_vetted\npkg/a.go,yes\npkg/b.go,pending\n")

	// 1. Default inventory (zero dups)
	out, err := executeMatrixCmd(t, []string{"inventory"})
	require.NoError(t, err)
	assert.NotEmpty(t, out)

	// 2. Inventory with duplicate records and --duplicates-only
	invPath := filepath.Join(projRoot, paths.ProjectDataDir, "literal_inventory.json")
	require.NoError(t, fileutil.MkdirAll(filepath.Dir(invPath), 0755))
	dupJSON := `{"schema_version":"1.0.0","literals":{"duplicate_string":{"literal":"duplicate_string","count":2,"locations":[{"path":"pkg/a.go","line":10},{"path":"pkg/b.go","line":20}]}}}`
	require.NoError(t, fileutil.WriteFile(invPath, []byte(dupJSON), 0644))

	out, err = executeMatrixCmd(t, []string{"inventory", "--duplicates-only"})
	require.NoError(t, err)
	assert.Contains(t, out, "duplicate_string")
	assert.Contains(t, out, "Violating Literals")

	// 3. Inventory with --all
	out, err = executeMatrixCmd(t, []string{"inventory", "--all"})
	require.NoError(t, err)
	assert.Contains(t, out, "Global String Literal Inventory")

	// 4. Invalid inventory JSON returns error
	require.NoError(t, fileutil.WriteFile(invPath, []byte("invalid json"), 0644))
	_, err = executeMatrixCmd(t, []string{"inventory"})
	assert.Error(t, err)
}

func TestMatrixDimensionsCommand(t *testing.T) {
	isolatedMatrixProject(t, "file_path,fully_vetted\npkg/a.go,yes\n")

	out, err := executeMatrixCmd(t, []string{"dimensions"})
	require.NoError(t, err)
	assert.Contains(t, out, "HCODE")
}

func TestMatrixStampCommand(t *testing.T) {
	projRoot := isolatedMatrixProject(t, "file_path,fully_vetted\npkg/a.go,pending\n")
	testFile := filepath.Join(projRoot, "pkg", "a.go")
	require.NoError(t, fileutil.MkdirAll(filepath.Dir(testFile), 0755))
	require.NoError(t, fileutil.WriteFile(testFile, []byte("package pkg\n"), 0644))

	out, err := executeMatrixCmd(t, []string{
		"stamp",
		"--file", "pkg/a.go",
		"--dimension", "HCODE",
		"--status", "passed",
	})
	require.NoError(t, err)
	assert.Contains(t, out, "Stamped")
}

func TestMatrixUpdateHelpers(t *testing.T) {
	// 1. dedupeSortedStrings
	input := []string{"  beta  ", "alpha", "beta", "", "  ", "gamma"}
	deduped := dedupeSortedStrings(input)
	assert.Equal(t, []string{"alpha", "beta", "gamma"}, deduped)

	// 2. resolveMatrixCvsIDsForAppend with explicit flag
	ids, err := resolveMatrixCvsIDsForAppend([]string{"CVS-001", "CVS-002", "CVS-001"}, "", nil, nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"CVS-001", "CVS-002"}, ids)

	// 3. resolveMatrixCvsIDsForAppend with empty session column error
	_, err = resolveMatrixCvsIDsForAppend(nil, "", nil, nil)
	assert.Error(t, err)

	// 4. resolveMatrixCvsIDsForAppend with single update result
	single := &quality.MatrixUpdateResult{
		RowAfter: map[string]string{
			"session_id": "CVS-999",
		},
	}
	ids, err = resolveMatrixCvsIDsForAppend(nil, "session_id", single, nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"CVS-999"}, ids)

	// 5. resolveMatrixCvsIDsForAppend with empty row value error
	singleEmpty := &quality.MatrixUpdateResult{
		RowAfter: map[string]string{
			"session_id": "",
		},
	}
	_, err = resolveMatrixCvsIDsForAppend(nil, "session_id", singleEmpty, nil)
	assert.Error(t, err)

	// 6. resolveMatrixCvsIDsForAppend with bulk result
	bulk := &quality.MatrixBulkUpdateResult{
		CvsIDs: []string{"CVS-100", "CVS-200"},
	}
	ids, err = resolveMatrixCvsIDsForAppend(nil, "session_id", nil, bulk)
	require.NoError(t, err)
	assert.Equal(t, []string{"CVS-100", "CVS-200"}, ids)
}

func TestExtractObjectIDFromOutput(t *testing.T) {
	assert.Equal(t, "TDE-12345", extractObjectIDFromOutput("Created technical debt TDE-12345."))
	assert.Equal(t, "BLI-98765", extractObjectIDFromOutput("Minted item: BLI-98765, successfully."))
	assert.Equal(t, "", extractObjectIDFromOutput("Nothing to see here."))
}

func TestZQKRemediationHook(t *testing.T) {
	tmpDir := t.TempDir()
	require.NoError(t, fileutil.MkdirAll(filepath.Join(tmpDir, paths.ProjectDataDir), 0755))
	require.NoError(t, fileutil.MkdirAll(filepath.Join(tmpDir, "bin"), 0755))
	mockBin := filepath.Join(tmpDir, "bin", "zqk")
	script := `#!/bin/sh
if [ "$3" = "technical_debt" ]; then
  echo "Created technical_debt TDE-99999 successfully"
elif [ "$3" = "backlog_item" ]; then
  echo "Created backlog_item BLI-88888 successfully"
fi
exit 0
`
	require.NoError(t, fileutil.WriteFile(mockBin, []byte(script), 0755))

	hook := NewZQKRemediationHook(tmpDir)
	require.NotNil(t, hook)

	sc := &matrix.FileScorecard{
		FilePath:     "pkg/sample/sample.go",
		Dimension:    "HCODE",
		ContentHash:  "abc1234",
		Evaluator:    "ast-scanner",
		PolicyID:     "POL-SEC-001",
		DiamondLabel: "DIAMOND-V1",
		Findings: []matrix.Finding{
			{
				Line:     15,
				Severity: "high",
				Message:  "Hardcoded credential pattern",
				RuleID:   "POL-SEC-001",
			},
		},
	}
	failures := []string{"Score 40 below threshold 80"}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rem, err := hook.OnPolicyFailure(ctx, sc, failures)
	require.NoError(t, err)
	require.NotNil(t, rem)
	assert.Equal(t, "TDE-99999", rem.TechnicalDebtID)
	assert.Equal(t, "BLI-88888", rem.BacklogItemID)

	// Error branch: write failure when .zqk dir is missing
	missingDir := filepath.Join(tmpDir, "does-not-exist")
	badHook := NewZQKRemediationHook(missingDir)
	_, err = badHook.OnPolicyFailure(ctx, sc, failures)
	assert.Error(t, err)
}

func TestMatrixUpdateExecution(t *testing.T) {
	isolatedMatrixProject(t, "file_path,fully_vetted\npkg/a.go,pending\npkg/b.go,pending\n")

	// 1. Single row update
	out, err := executeMatrixCmd(t, []string{"update", "--name", "t", "--file-path", "pkg/a.go", "--set", "fully_vetted=yes"})
	require.NoError(t, err)
	assert.Contains(t, out, "wrote")

	// 2. Bulk row update
	out, err = executeMatrixCmd(t, []string{"update", "--name", "t", "--filter", "fully_vetted=pending", "--set", "fully_vetted=yes"})
	require.NoError(t, err)
	assert.Contains(t, out, "wrote")

	// 3. Error: missing --file-path and --bundle-label without filter
	_, err = executeMatrixCmd(t, []string{"update", "--name", "t", "--set", "fully_vetted=yes"})
	assert.Error(t, err)

	// 4. Error: combining --filter with --file-path
	_, err = executeMatrixCmd(t, []string{"update", "--name", "t", "--filter", "fully_vetted=pending", "--file-path", "pkg/a.go", "--set", "fully_vetted=yes"})
	assert.Error(t, err)
}

func TestValidateCvsIDsReadable(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, nil)
	cmd := NewMatrixUpdateCmd()
	cmd.SetContext(context.Background())
	cli.SetContext(cmd, cli.ContextForProjectRoot(proj.Root))

	// Empty IDs returns nil
	err := validateCvsIDsReadable(cmd, nil)
	assert.NoError(t, err)

	// Non-existent ID returns error
	err = validateCvsIDsReadable(cmd, []string{"CVS-NONEXISTENT"})
	assert.Error(t, err)
}

func TestAppendMatrixCvsActivityErrors(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, nil)
	cmd := NewMatrixUpdateCmd()
	cmd.SetContext(context.Background())
	cli.SetContext(cmd, cli.ContextForProjectRoot(proj.Root))

	// Empty session IDs
	_, err := appendMatrixCvsActivity(cmd, "t", "m.csv", false, 0, nil, nil, "", nil, nil)
	assert.Error(t, err)

	// Non-existent session IDs
	_, err = appendMatrixCvsActivity(cmd, "t", "m.csv", false, 1, map[string]string{"k": "v"}, []string{"CVS-NONEXISTENT"}, "", nil, nil)
	assert.Error(t, err)
}

func TestMatrixInventoryWithDuplicates(t *testing.T) {
	proj := isolatedMatrixProject(t, "file_path,fully_vetted\npkg/a.go,yes\n")
	invPath := filepath.Join(proj, paths.ProjectDataDir, "literal_inventory.json")
	require.NoError(t, fileutil.MkdirAll(filepath.Dir(invPath), 0755))
	invData := []byte(`{"schema_version":"1.0.0","literals":{"dup_str":{"literal":"dup_str","count":2,"locations":[{"path":"pkg/a.go","line":1},{"path":"pkg/b.go","line":2}]}}}`)
	require.NoError(t, fileutil.WriteFile(invPath, invData, 0644))

	out, err := executeMatrixCmd(t, []string{"inventory", "--duplicates-only"})
	require.NoError(t, err)
	assert.Contains(t, out, "Violating Literals")
	assert.Contains(t, out, "dup_str")

	outAll, err := executeMatrixCmd(t, []string{"inventory", "--all"})
	require.NoError(t, err)
	assert.NotEmpty(t, outAll)

	// Corrupt file
	require.NoError(t, fileutil.WriteFile(invPath, []byte("bad-json"), 0644))
	_, err = executeMatrixCmd(t, []string{"inventory"})
	assert.Error(t, err)
}

func TestMatrixReportDetailed(t *testing.T) {
	projRoot := isolatedMatrixProject(t, "file_path,fully_vetted\npkg/a.go,yes\n")
	csvPath := filepath.Join(projRoot, "docs", "quality", "m.csv")
	profPath := filepath.Join(projRoot, "docs", "quality", "profile.yaml")

	// Direct matrix and profile flags
	out, err := executeMatrixCmd(t, []string{"report", "--matrix", csvPath, "--profile", profPath})
	require.NoError(t, err)
	assert.Contains(t, out, "rowtotal")

	// Go only flag
	outGo, err := executeMatrixCmd(t, []string{"report", "--matrix", csvPath, "--profile", profPath, "--go-only"})
	require.NoError(t, err)
	assert.Contains(t, outGo, "rowtotal")
}

func TestMatrixValidateFailingMatrix(t *testing.T) {
	projRoot := isolatedMatrixProject(t, "file_path,fully_vetted\npkg/a.go,pending\n")
	// Corrupt profile YAML so validation fails
	profPath := filepath.Join(projRoot, "docs", "quality", "profile.yaml")
	require.NoError(t, fileutil.WriteFile(profPath, []byte("corrupt: [unclosed"), 0644))

	_, err := executeMatrixCmd(t, []string{"validate", "--name", "t"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "one or more matrices failed validation")
}

func TestMatrixUpdateDetailedBranches(t *testing.T) {
	isolatedMatrixProject(t, "file_path,bundle_label,fully_vetted\npkg/a.go,bundle1,pending\npkg/b.go,bundle2,pending\n")

	// 1. Success: bundle-label update
	out, err := executeMatrixCmd(t, []string{"update", "--name", "t", "--bundle-label", "bundle1", "--set", "fully_vetted=yes", "--dry-run"})
	require.NoError(t, err)
	assert.NotEmpty(t, out)

	// 2. Error: both file-path and bundle-label
	_, err = executeMatrixCmd(t, []string{"update", "--name", "t", "--file-path", "pkg/a.go", "--bundle-label", "bundle1", "--set", "fully_vetted=yes"})
	assert.Error(t, err)
}

