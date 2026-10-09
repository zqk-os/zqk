package main

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/vet"
)

func TestRunMain_Help(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := runMain([]string{"--help"}, &stdout, &stderr)
	assert.Equal(t, 0, code)
	assert.Contains(t, stderr.String(), "Usage of zqk-vet")
}

func TestRunMain_UnknownFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := runMain([]string{"--unknown-flag-xyz"}, &stdout, &stderr)
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr.String(), "flag provided but not defined")
}

func TestRunMain_InvalidConfig(t *testing.T) {
	tempDir := t.TempDir()
	badConfigPath := filepath.Join(tempDir, "bad_gates.yaml")
	require.NoError(t, fileutil.WriteFile(badConfigPath, []byte("hygiene:\n  [bad yaml"), fileutil.StandardFilePerm))

	var stdout, stderr bytes.Buffer
	code := runMain([]string{"--root", tempDir, "--config", badConfigPath}, &stdout, &stderr)
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr.String(), "zqk-vet: load config:")
}

func TestRunMain_UnknownSuite(t *testing.T) {
	tempDir := t.TempDir()
	var stdout, stderr bytes.Buffer
	code := runMain([]string{"--root", tempDir, "--suite", "nonexistent_suite"}, &stdout, &stderr)
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr.String(), "error running checks: unknown vet suite: nonexistent_suite")
}

func TestRunMain_FailFastAndReportFail(t *testing.T) {
	// Empty temp dir with default config will fail payload & tree checks
	tempDir := t.TempDir()
	var stdout, stderr bytes.Buffer
	code := runMain([]string{"--root", tempDir, "--suite", "payload", "--fail-fast"}, &stdout, &stderr)
	assert.Equal(t, 1, code)
	assert.Contains(t, stdout.String(), "RESULT: FAIL")
}

func TestRunMain_SuccessAndJSON(t *testing.T) {
	tempDir := t.TempDir()
	// Create minimal config where checks pass
	cfgContent := `version: "1.0.0"
hygiene:
  check_paths: false
  check_perms: false
  check_dups: false
  check_cli_names: false
  check_subprocess_hygiene: false
  check_command_specs: false
  check_cli_builders: false
  check_raw_goroutines: false
tree_police:
  forbidden_paths: []
  forbidden_files: []
payload:
  required_module_path: ""
  required_artifacts: []
`
	cfgPath := filepath.Join(tempDir, "gates.yaml")
	require.NoError(t, fileutil.WriteFile(cfgPath, []byte(cfgContent), fileutil.StandardFilePerm))

	var stdout, stderr bytes.Buffer
	code := runMain([]string{
		"--root", tempDir,
		"--config", cfgPath,
		"--suite", "hygiene,payload",
		"--json",
		"file1.go",
	}, &stdout, &stderr)

	assert.Equal(t, 0, code)
	assert.Empty(t, stderr.String())

	var report vet.Report
	err := json.Unmarshal(stdout.Bytes(), &report)
	require.NoError(t, err)
	assert.True(t, report.Passed)
	assert.Contains(t, report.SuiteResults, "hygiene")
	assert.Contains(t, report.SuiteResults, "payload")
}

func TestRunMain_TextOutputSuccess(t *testing.T) {
	tempDir := t.TempDir()
	cfgContent := `version: "1.0.0"
hygiene:
  check_paths: false
  check_perms: false
  check_dups: false
  check_cli_names: false
  check_subprocess_hygiene: false
  check_command_specs: false
  check_cli_builders: false
  check_raw_goroutines: false
tree_police:
  forbidden_paths: []
  forbidden_files: []
payload:
  required_module_path: ""
  required_artifacts: []
`
	cfgPath := filepath.Join(tempDir, "gates.yaml")
	require.NoError(t, fileutil.WriteFile(cfgPath, []byte(cfgContent), fileutil.StandardFilePerm))

	var stdout, stderr bytes.Buffer
	code := runMain([]string{
		"--root", tempDir,
		"--config", cfgPath,
		"--suite", "all",
	}, &stdout, &stderr)

	assert.Equal(t, 0, code)
	out := stdout.String()
	assert.Contains(t, out, "=== ZQK Verification Engine (zqk-vet) ===")
	assert.Contains(t, out, "RESULT: PASS")
}
