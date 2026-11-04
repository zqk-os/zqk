package newcmd

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testenvroot"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func setupTestProjectRoot(t *testing.T) string {
	t.Helper()
	testRoot := t.TempDir()
	t.Setenv(zqkenv.TestRoot().Name(), testRoot)
	repoRoot := zqkenv.ProjectRoot().Get()
	if repoRoot == "" {
		root, err := findModuleRoot()
		if err != nil {
			t.Fatalf("findModuleRoot: %v", err)
		}
		repoRoot = root
	}
	if err := testenvroot.BootstrapRoot(testRoot, repoRoot); err != nil {
		t.Fatalf("BootstrapRoot: %v", err)
	}
	t.Cleanup(func() {
		if teardownErr := storage.RunProjectTestTeardown(storage.TempProjectTeardown(testRoot, nil)); teardownErr != nil {
			t.Logf("teardown error: %v", teardownErr)
		}
	})
	return testRoot
}

func TestEnsureCommandSpecFlags_RegistersAllFlags(t *testing.T) {
	cmd := &cobra.Command{Use: "custom"}
	ensureCommandSpecFlags(cmd)

	expectedFlags := []string{
		"from-cmd", "aliases", "args-type", "args-count", "args-min", "args-max",
		"flag", "example", "run-e", "common-flags", "async",
	}

	for _, flagName := range expectedFlags {
		if cmd.Flags().Lookup(flagName) == nil {
			t.Fatalf("expected flag %q to be registered by ensureCommandSpecFlags", flagName)
		}
	}
}

func TestFindCobraCommand_EdgeCases(t *testing.T) {
	if cmd := findCobraCommand(nil, []string{"sub"}); cmd != nil {
		t.Fatalf("expected nil command when root is nil")
	}
	root := &cobra.Command{Use: "root"}
	if cmd := findCobraCommand(root, nil); cmd != nil {
		t.Fatalf("expected nil command when segments is nil")
	}
	if cmd := findCobraCommand(root, []string{}); cmd != nil {
		t.Fatalf("expected nil command when segments is empty")
	}
	if cmd := findCobraCommand(root, []string{"missing"}); cmd != nil {
		t.Fatalf("expected nil command when subcommand not found")
	}
}

func TestEnqueueMintPromoteJob_UpdateExisting(t *testing.T) {
	testRoot := setupTestProjectRoot(t)
	objectID := "BLI-CYCLE109-UPDATE-001"

	jobID1, err := enqueueMintPromoteJob(testRoot, objectID)
	if err != nil {
		t.Fatalf("first enqueueMintPromoteJob failed: %v", err)
	}

	jobID2, err := enqueueMintPromoteJob(testRoot, objectID)
	if err != nil {
		t.Fatalf("second enqueueMintPromoteJob (update branch) failed: %v", err)
	}
	if jobID1 != jobID2 {
		t.Fatalf("expected same job ID %q, got %q", jobID1, jobID2)
	}
}

func TestRunNewObject_WithPromoteFlag(t *testing.T) {
	testRoot := setupTestProjectRoot(t)
	if testRoot == "" {
		t.Fatalf("expected non-empty test root")
	}

	var buf bytes.Buffer
	ctx := pkgctx.WithCommandOutputWriter(context.Background(), &buf)

	cmd := NewNewCmd()
	cmd.SetContext(ctx)
	cmd.SetArgs([]string{"object", "backlog_item", "--title", "Promote Opt-In Item", "--description", "Valid description for promote", "--promote"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("cmd.Execute() with --promote failed: %v", err)
	}
}

func TestRunNewObject_DirectInvocation_UncoveredBranches(t *testing.T) {
	cmd := &cobra.Command{Use: "object"}
	cmd.Flags().String("title", "", "")
	cmd.Flags().String("file", "", "")
	cmd.Flags().String("content", "", "")
	cmd.Flags().String("description", "", "")
	cmd.Flags().Bool("promote", false, "")

	// 1. Invalid kind directly without pre-run
	if err := cmd.Flags().Set("title", "Some Title"); err != nil {
		t.Fatalf("set flag: %v", err)
	}
	err := runNewObject(cmd, []string{""})
	if err == nil {
		t.Fatalf("expected error for invalid object kind")
	}
	if !strings.Contains(err.Error(), "invalid object kind") {
		t.Fatalf("expected invalid object kind error, got: %v", err)
	}

	// 2. Nonexistent file with valid kind
	if err := cmd.Flags().Set("file", "/path/to/definitely/nonexistent/file.md"); err != nil {
		t.Fatalf("set flag: %v", err)
	}
	err = runNewObject(cmd, []string{"backlog_item"})
	if err == nil {
		t.Fatalf("expected error for nonexistent file")
	}
}

func TestNewKindConvenienceCommands_Execution(t *testing.T) {
	testRoot := setupTestProjectRoot(t)
	if testRoot == "" {
		t.Fatalf("expected non-empty test root")
	}

	kindsToTest := []struct {
		cmdName string
		args    []string
	}{
		{"bli", []string{"bli", "--title", "Sample BLI", "--description", "BLI Description"}},
		{"req", []string{"req", "--title", "Sample Req"}},
	}

	for _, tc := range kindsToTest {
		var buf bytes.Buffer
		ctx := pkgctx.WithCommandOutputWriter(context.Background(), &buf)
		cmd := NewNewCmd()
		cmd.SetContext(ctx)
		cmd.SetArgs(tc.args)
		if err := cmd.Execute(); err != nil {
			t.Fatalf("failed executing convenience command %q: %v", tc.cmdName, err)
		}
	}
}

func TestRunNewCommandSpec_ValidationErrors(t *testing.T) {
	cmd := NewNewCmd()
	cmd.SetArgs([]string{"command-spec", "   "})
	if err := cmd.Execute(); err == nil {
		t.Fatalf("expected error for whitespace command path")
	}

	cmd = NewNewCmd()
	cmd.SetArgs([]string{"command-spec", "invalid/UPPERCASE"})
	if err := cmd.Execute(); err == nil {
		t.Fatalf("expected error for invalid uppercase path segment")
	}

	cmd = NewNewCmd()
	cmd.SetArgs([]string{"command-spec", "valid/path", "--from-cmd"})
	if err := cmd.Execute(); err == nil {
		t.Fatalf("expected error for --from-cmd with non-registered command")
	}

	cmd = NewNewCmd()
	cmd.SetArgs([]string{"command-spec", "valid/path"})
	if err := cmd.Execute(); err == nil {
		t.Fatalf("expected error for missing --short flag")
	}
}

func TestRunNewCommandSpec_ForceAndOptions(t *testing.T) {
	testRoot := setupTestProjectRoot(t)

	cmd := NewNewCmd()
	cmd.SetArgs([]string{
		"command-spec", "tools/widget",
		"--short", "Manage widgets",
		"--description", "Full widget manager",
		"--args-type", "exact",
		"--args-count", "2",
		"--args-min", "1",
		"--args-max", "3",
		"--example", "Bare example command without colon",
		"--run-e", "runWidgetManager",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("initial command-spec creation failed: %v", err)
	}

	// Conflict without --force
	cmd = NewNewCmd()
	cmd.SetArgs([]string{"command-spec", "tools/widget", "--short", "Manage widgets"})
	if err := cmd.Execute(); err == nil {
		t.Fatalf("expected error when command spec file already exists without --force")
	}

	// Success with --force
	cmd = NewNewCmd()
	cmd.SetArgs([]string{"command-spec", "tools/widget", "--short", "Manage widgets", "--force"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("expected command-spec with --force to succeed, got: %v", err)
	}

	specPath := filepath.Join(testRoot, ".zqk", "cli", "specs", "tools", "widget_command.yaml")
	if _, statErr := fileutil.Stat(specPath); statErr != nil {
		t.Fatalf("expected spec file %s to exist: %v", specPath, statErr)
	}
}

func TestRunNewCommandSpec_IntrospectionVariants(t *testing.T) {
	testRoot := setupTestProjectRoot(t)
	if testRoot == "" {
		t.Fatalf("expected non-empty test root")
	}

	rootCmd := &cobra.Command{Use: "zqk"}
	subCmd := &cobra.Command{
		Use:     "subitem <target>",
		Short:   "Subitem short",
		Long:    "Subitem detailed long description",
		Example: "# Sample example comment\nzqk subitem target-1\n# Second comment\nzqk subitem target-2\nzqk subitem bare-invocation",
	}
	subCmd.Flags().Bool("enabled", true, "Is enabled")
	subCmd.Flags().Int("count", 42, "Item count")
	subCmd.Flags().StringSlice("tags", []string{"a", "b"}, "Tags")
	subCmd.Flags().Duration("duration", 5*time.Second, "Timeout duration")
	subCmd.Flags().String("unspecified", "", "Unspecified default")
	rootCmd.AddCommand(subCmd)

	subCmd2 := &cobra.Command{
		Use:     "subitem2 [items...]",
		Short:   "Subitem2 short only",
		Example: "\n# Comment 1\nzqk subitem2 a\n\n# Comment 2\nzqk subitem2 b\n",
	}
	subCmd2.Flags().Bool("format", false, "format flag")
	rootCmd.AddCommand(subCmd2)

	newSpecCmd := NewNewCmd()
	rootCmd.AddCommand(newSpecCmd)

	rootCmd.SetArgs([]string{"new", "command-spec", "subitem", "--from-cmd"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("introspecting subitem failed: %v", err)
	}

	rootCmd.SetArgs([]string{"new", "command-spec", "subitem2", "--from-cmd"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("introspecting subitem2 failed: %v", err)
	}
}

func TestResolveCommandSpecsDir_AbsPathConfig(t *testing.T) {
	testRoot := t.TempDir()
	absSpecsDir := filepath.Join(testRoot, "custom_specs_dir")

	configPath := filepath.Join(testRoot, paths.ProcessInternalConfigsDir, paths.PathsConfigFile)
	if err := paths.EnsureDir(filepath.Dir(configPath), paths.DirPerm755); err != nil {
		t.Fatalf("ensure dir: %v", err)
	}
	yamlContent := "paths:\n  command_specs: " + absSpecsDir + "\n"
	if err := fileutil.WriteFile(configPath, []byte(yamlContent), paths.FilePerm644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	resolved := resolveCommandSpecsDir(testRoot)
	if resolved != absSpecsDir {
		t.Fatalf("expected %q, got %q", absSpecsDir, resolved)
	}
}

func TestWriteDraft_OutputsAndSanitization(t *testing.T) {
	testRoot := t.TempDir()
	t.Setenv(zqkenv.TestRoot().Name(), testRoot)

	// 1. Stdout output
	var outBuf bytes.Buffer
	ctx := pkgctx.WithCommandOutputWriter(context.Background(), &outBuf)
	cmd := &cobra.Command{Use: "test"}
	cmd.SetContext(ctx)
	cmd.SetOut(&outBuf)
	if err := writeDraft(cmd, "-", "testbase", []byte("sample-data"), "", ""); err != nil {
		t.Fatalf("writeDraft to stdout failed: %v", err)
	}
	if !strings.Contains(outBuf.String(), "sample-data") {
		t.Fatalf("expected stdout to contain sample-data, got: %q", outBuf.String())
	}

	// 2. Default filename sanitization with slashes and colons
	if err := writeDraft(cmd, "", "name/with:colon\\slash", []byte("content"), "scope-1", "kind-1"); err != nil {
		t.Fatalf("writeDraft with sanitization failed: %v", err)
	}

	// 3. Permission error on invalid path
	invalidPath := filepath.Join("/dev/null/impossible_dir/spec.yaml")
	if err := writeDraft(cmd, invalidPath, "testbase", []byte("data"), "", ""); err == nil {
		t.Fatalf("expected error writing to invalid path")
	}
}

func TestResolveCommandSpecsDir_RelPathConfig(t *testing.T) {
	testRoot := setupTestProjectRoot(t)
	configPath := filepath.Join(testRoot, paths.ProcessInternalConfigsDir, paths.PathsConfigFile)
	if err := paths.EnsureDir(filepath.Dir(configPath), paths.DirPerm755); err != nil {
		t.Fatalf("ensure dir: %v", err)
	}
	yamlContent := "paths:\n  command_specs: relative/custom_specs\n"
	if err := fileutil.WriteFile(configPath, []byte(yamlContent), paths.FilePerm644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	want := filepath.Join(testRoot, "relative/custom_specs")
	if resolved := resolveCommandSpecsDir(testRoot); resolved != want {
		t.Fatalf("expected %q, got %q", want, resolved)
	}

	yamlEmpty := "paths:\n  command_specs: \"\"\n"
	if err := fileutil.WriteFile(configPath, []byte(yamlEmpty), paths.FilePerm644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	wantDefault := filepath.Join(testRoot, paths.CLICommandSpecsDir)
	if resolved := resolveCommandSpecsDir(testRoot); resolved != wantDefault {
		t.Fatalf("expected default %q, got %q", wantDefault, resolved)
	}
}

func TestRunNewCommandSpec_AltSchemaBranch(t *testing.T) {
	testRoot := setupTestProjectRoot(t)
	specsDir := resolveCommandSpecsDir(testRoot)
	altSchema := filepath.Join(specsDir, "_schemas", "command_spec.schema.json")
	if err := paths.EnsureDir(filepath.Dir(altSchema), paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteFile(altSchema, []byte("{}"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}

	cmd := NewNewCmd()
	cmd.SetArgs([]string{"command-spec", "demo/altschema", "--short", "Test alt schema"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("command-spec with alt schema failed: %v", err)
	}
}

func TestShouldAutoTracePipelineConfigured_Branches(t *testing.T) {
	if shouldAutoTracePipelineConfigured(nil, "requirement", true, false) {
		t.Fatal("expected false when inTest=true")
	}
	if shouldAutoTracePipelineConfigured(nil, "requirement", false, true) {
		t.Fatal("expected false when hasTestRoot=true")
	}
	if shouldAutoTracePipelineConfigured(nil, "goal", false, false) {
		t.Fatal("expected false for goal kind")
	}
	if shouldAutoTracePipelineConfigured(nil, "requirement", false, false) {
		t.Fatal("expected false when cmd is nil")
	}

	cmd := &cobra.Command{Use: "req"}
	if shouldAutoTracePipelineConfigured(cmd, "requirement", false, false) {
		t.Fatal("expected false when flags missing")
	}
	cmd.Flags().Bool("trace-pipeline", false, "")
	cmd.Flags().Bool("skip-trace-pipeline", false, "")
	if shouldAutoTracePipelineConfigured(cmd, "requirement", false, false) {
		t.Fatal("expected false when trace-pipeline=false")
	}
	if err := cmd.Flags().Set("trace-pipeline", "true"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("skip-trace-pipeline", "true"); err != nil {
		t.Fatal(err)
	}
	if shouldAutoTracePipelineConfigured(cmd, "requirement", false, false) {
		t.Fatal("expected false when skip-trace-pipeline=true")
	}
	if err := cmd.Flags().Set("skip-trace-pipeline", "false"); err != nil {
		t.Fatal(err)
	}
	if !shouldAutoTracePipelineConfigured(cmd, "requirement", false, false) {
		t.Fatal("expected true when trace-pipeline=true and skip-trace-pipeline=false")
	}
}

func TestRunNewKindValidatePreRun_EdgeCases(t *testing.T) {
	if err := runNewKindValidatePreRun(nil, nil); err != nil {
		t.Fatal(err)
	}
	cmdNoAnnot := &cobra.Command{Use: "test"}
	if err := runNewKindValidatePreRun(cmdNoAnnot, nil); err != nil {
		t.Fatal(err)
	}
	cmdEmptyAnnot := &cobra.Command{
		Use:         "test",
		Annotations: map[string]string{cli.AnnotationKeyObjectKindValidate: ""},
	}
	if err := runNewKindValidatePreRun(cmdEmptyAnnot, nil); err != nil {
		t.Fatal(err)
	}
}
