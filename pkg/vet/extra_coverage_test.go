package vet

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestCheckCLIBuilders_Helpers(t *testing.T) {
	assert.Equal(t, "FooBarBaz", toCamelCase("foo_bar_baz"))
	assert.Equal(t, "FooBarBaz", toCamelCase("foo-bar-baz"))
	assert.Equal(t, "Simple", toCamelCase("simple"))
	assert.Equal(t, "", toCamelCase(""))
}

func TestCheckCLIBuilders_Scenarios(t *testing.T) {
	tempDir := t.TempDir()

	// 1. When specs and builders directories do not exist, return empty findings and nil error
	findings, err := CheckCLIBuilders(tempDir, DefaultConfig(), nil)
	require.NoError(t, err)
	assert.Empty(t, findings)

	// 2. Setup spec dir with a command spec that has NO builder
	specsDir := filepath.Join(tempDir, paths.ProjectDataDir, paths.CLISpecsDir)
	require.NoError(t, fileutil.MkdirAll(specsDir, 0755))
	specFile := filepath.Join(specsDir, "sample_command.yaml")
	require.NoError(t, fileutil.WriteFile(specFile, []byte("name: sample\nshort: Sample command\n"), fileutil.StandardFilePerm))

	// Setup cmd/zqk with a constructor that violates builder pattern (directly instantiates cobra.Command)
	cmdDir := filepath.Join(tempDir, "cmd", "zqk", "sample")
	require.NoError(t, fileutil.MkdirAll(cmdDir, 0755))
	cmdSrc := `package sample
import "github.com/spf13/cobra"
func NewsampleCmd() *cobra.Command {
	return &cobra.Command{
		Use: "sample",
	}
}
`
	require.NoError(t, fileutil.WriteFile(filepath.Join(cmdDir, "sample.go"), []byte(cmdSrc), fileutil.StandardFilePerm))

	findings, err = CheckCLIBuilders(tempDir, DefaultConfig(), nil)
	require.NoError(t, err)
	// Should have missing_builder AND builder_pattern_violation
	assert.NotEmpty(t, findings)
	var foundMissing, foundViolation bool
	for _, f := range findings {
		if f.CheckID == "cli/missing_builder" {
			foundMissing = true
		}
		if f.CheckID == "cli/builder_pattern_violation" {
			foundViolation = true
		}
	}
	assert.True(t, foundMissing, "expected missing builder finding")
	assert.True(t, foundViolation, "expected builder pattern violation finding")

	// 3. Now create builder in pkg/cli/bldr_cli_cmd_v1
	buildersDir := filepath.Join(tempDir, "pkg", "cli", "bldr_cli_cmd_v1")
	require.NoError(t, fileutil.MkdirAll(buildersDir, 0755))
	builderSrc := `package bldr_cli_cmd_v1
func NewSampleCommandBuilder() {}
`
	require.NoError(t, fileutil.WriteFile(filepath.Join(buildersDir, "sample_builder.go"), []byte(builderSrc), fileutil.StandardFilePerm))

	// Update constructor to call builder
	goodCmdSrc := `package sample
import "github.com/spf13/cobra"
func NewsampleCmd() *cobra.Command {
	NewSampleCommandBuilder()
	return &cobra.Command{
		Use: "sample",
	}
}
`
	require.NoError(t, fileutil.WriteFile(filepath.Join(cmdDir, "sample.go"), []byte(goodCmdSrc), fileutil.StandardFilePerm))

	findings, err = CheckCLIBuilders(tempDir, DefaultConfig(), nil)
	require.NoError(t, err)
	var hasMissingOrViolation bool
	for _, f := range findings {
		if f.CheckID == "cli/missing_builder" || f.CheckID == "cli/builder_pattern_violation" {
			hasMissingOrViolation = true
		}
	}
	assert.False(t, hasMissingOrViolation)
}

func TestCheckCommandSpecs_Helpers(t *testing.T) {
	assert.Equal(t, "user", inferCommandNameFromConstructor("NewUserCmd"))
	assert.Equal(t, "workflow", inferCommandNameFromConstructor("pkg.NewWorkflowCommand"))
	assert.Equal(t, "", inferCommandNameFromConstructor("Execute"))

	assert.True(t, isCommandConstructorName("NewFooCmd"))
	assert.True(t, isCommandConstructorName("NewBarCommand"))
	assert.False(t, isCommandConstructorName("NewClient"))

	assert.Equal(t, "status", resolveFullCommandPath("", "status"))
	assert.Equal(t, "status", resolveFullCommandPath("app", "status"))
	assert.Equal(t, "agent status", resolveFullCommandPath("agent", "status"))
	assert.Equal(t, "agent", resolveFullCommandPath("agent", "agent"))

	assert.Equal(t, "foo_bar", canonicalCommandIDString("foo bar"))
	assert.Equal(t, "foo_bar", canonicalCommandIDString("foo-bar"))
	assert.Equal(t, "foo_bar", canonicalCommandIDString("foo/bar"))
	assert.Equal(t, "foo_bar", canonicalCommandSpecID("CSPEC-foo-bar-command.yaml"))
}

func TestCheckCommandSpecs_Scenarios(t *testing.T) {
	tempDir := t.TempDir()

	// 1. Non-existent specs dir
	findings, err := CheckCommandSpecs(tempDir, DefaultConfig(), nil)
	require.NoError(t, err)
	assert.Empty(t, findings)

	// 2. Setup specs dir with invalid YAML, missing name, missing short/desc, and valid spec
	specsDir := filepath.Join(tempDir, paths.ProjectDataDir, paths.CLISpecsDir)
	require.NoError(t, fileutil.MkdirAll(specsDir, 0755))

	// Invalid YAML
	require.NoError(t, fileutil.WriteFile(filepath.Join(specsDir, "syntax_err.yaml"), []byte(":\n  - bad: ["), fileutil.StandardFilePerm))
	// Missing name
	require.NoError(t, fileutil.WriteFile(filepath.Join(specsDir, "missing_name.yaml"), []byte("short: has short\n"), fileutil.StandardFilePerm))
	// Missing short & desc
	require.NoError(t, fileutil.WriteFile(filepath.Join(specsDir, "missing_desc.yaml"), []byte("name: nodirection\n"), fileutil.StandardFilePerm))
	// Valid spec
	require.NoError(t, fileutil.WriteFile(filepath.Join(specsDir, "valid.yaml"), []byte("name: valid\nshort: is valid\n"), fileutil.StandardFilePerm))

	findings, err = CheckCommandSpecs(tempDir, DefaultConfig(), nil)
	require.NoError(t, err)
	assert.True(t, len(findings) >= 3)

	var hasSyntax, hasMissingName, hasMissingDesc bool
	for _, f := range findings {
		if f.CheckID == "command_spec/syntax" {
			hasSyntax = true
		}
		if f.CheckID == "command_spec/schema" && f.File == filepath.Join(paths.ProjectDataDir, paths.CLISpecsDir, "missing_name.yaml") {
			hasMissingName = true
		}
		if f.CheckID == "command_spec/schema" && f.File == filepath.Join(paths.ProjectDataDir, paths.CLISpecsDir, "missing_desc.yaml") {
			hasMissingDesc = true
		}
	}
	assert.True(t, hasSyntax)
	assert.True(t, hasMissingName)
	assert.True(t, hasMissingDesc)

	// 3. Fallback scan of root_commands.go
	appDir := filepath.Join(tempDir, "cmd", "zqk", "app")
	require.NoError(t, fileutil.MkdirAll(appDir, 0755))
	rootCmdsSrc := `package app
func registerCommands() {
	cmdA := NewValidCmd()
	cmdB := NewUnspeccedCmd()
	rootCmd.AddCommand(cmdA)
	rootCmd.AddCommand(cmdB)
}
`
	require.NoError(t, fileutil.WriteFile(filepath.Join(appDir, "root_commands.go"), []byte(rootCmdsSrc), fileutil.StandardFilePerm))

	// Create baseline excluding nothing
	cliDir := filepath.Join(tempDir, paths.ProjectDataDir, "cli")
	require.NoError(t, fileutil.MkdirAll(cliDir, 0755))
	require.NoError(t, fileutil.WriteFile(filepath.Join(cliDir, "command_spec_coverage_baseline.json"), []byte(`{"commands_without_specs":[]}`), fileutil.StandardFilePerm))

	findings, err = CheckCommandSpecs(tempDir, DefaultConfig(), nil)
	require.NoError(t, err)
	var hasMissingCmd bool
	for _, f := range findings {
		if f.CheckID == "command_spec/missing" {
			hasMissingCmd = true
		}
	}
	assert.True(t, hasMissingCmd, "expected missing command spec finding for unspecced command")

	// 4. Test with live cobra command
	root := &cobra.Command{Use: "root"}
	child := &cobra.Command{Use: "valid"}
	root.AddCommand(child)

	findings, err = CheckCommandSpecs(tempDir, DefaultConfig(), root)
	require.NoError(t, err)
	assert.NotNil(t, findings)
}

func TestRunner_CompleteCoverage(t *testing.T) {
	tempDir := t.TempDir()

	// NewRunner with nil config defaults to DefaultConfig
	r := NewRunner(tempDir, nil)
	require.NotNil(t, r)
	require.NotNil(t, r.Config)

	// Run with fail-fast on unknown suite returns error
	opts := RunOptions{
		Suites:   []string{"unknown_suite"},
		FailFast: true,
	}
	_, err := r.Run(opts)
	assert.Error(t, err)

	// Run hygiene suite with all checks enabled on empty temp dir
	r.Config.Hygiene.CheckPaths = true
	r.Config.Hygiene.CheckPerms = true
	r.Config.Hygiene.CheckCLINames = true
	r.Config.Hygiene.CheckSubprocessHygiene = true
	r.Config.Hygiene.CheckCommandSpecs = true
	r.Config.Hygiene.CheckCLIBuilders = true
	r.Config.Hygiene.CheckRawGoroutines = true
	r.Config.Hygiene.CheckDups = true

	report, err := r.Run(RunOptions{
		Suites: []string{"hygiene"},
	})
	require.NoError(t, err)
	assert.NotNil(t, report)

	// Test PrintReport with PASS and FAIL
	var buf bytes.Buffer
	report.Passed = true
	r.PrintReport(&buf, report)
	assert.Contains(t, buf.String(), "RESULT: PASS")

	buf.Reset()
	report.Passed = false
	report.TotalErrors = 2
	report.SuiteResults["hygiene"] = SuiteResult{
		Suite:    "hygiene",
		Passed:   false,
		Findings: []Finding{{Severity: SeverityError, Message: "test err"}},
	}
	r.PrintReport(&buf, report)
	assert.Contains(t, buf.String(), "RESULT: FAIL")
}

func TestLoadConfig_ErrorsAndValid(t *testing.T) {
	tempDir := t.TempDir()

	// Invalid YAML content
	badPath := filepath.Join(tempDir, "bad.yaml")
	require.NoError(t, fileutil.WriteFile(badPath, []byte("broken:\n  - ["), fileutil.StandardFilePerm))
	_, err := LoadConfig(tempDir, badPath)
	assert.Error(t, err)

	// Valid custom YAML
	goodPath := filepath.Join(tempDir, "good.yaml")
	require.NoError(t, fileutil.WriteFile(goodPath, []byte("version: '2.0.0'\n"), fileutil.StandardFilePerm))
	cfg, err := LoadConfig(tempDir, goodPath)
	require.NoError(t, err)
	assert.Equal(t, "2.0.0", cfg.Version)
}
