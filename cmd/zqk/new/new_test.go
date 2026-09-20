package newcmd

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/testenvroot"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func findModuleRoot() (string, error) {
	dir, err := fileutil.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := fileutil.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fileutil.ErrNotExist
		}
		dir = parent
	}
}

func TestNewObject_mintRequiresTitle(t *testing.T) {
	var buf bytes.Buffer
	ctx := pkgctx.WithCommandOutputWriter(context.Background(), &buf)
	cmd := NewNewCmd()
	cmd.SetContext(ctx)
	cmd.SetArgs([]string{"object", "backlog_item"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error without --title")
	}
	if !strings.Contains(err.Error(), "title is required") {
		t.Fatalf("got: %v", err)
	}
	if !strings.Contains(err.Error(), "object template") {
		t.Fatalf("error should point at object template: %v", err)
	}
}

func TestNewObject_unknown_kind_errors(t *testing.T) {
	testRoot := t.TempDir()
	t.Setenv(zqkenv.TestRoot().Name(), testRoot)

	repoRoot := zqkenv.ProjectRoot().Get()
	if repoRoot == "" {
		if root, err := findModuleRoot(); err == nil {
			repoRoot = root
		}
	}
	if err := testenvroot.BootstrapRoot(testRoot, repoRoot); err != nil {
		t.Fatalf("BootstrapRoot: %v", err)
	}

	var buf bytes.Buffer
	ctx := pkgctx.WithCommandOutputWriter(context.Background(), &buf)
	cmd := NewNewCmd()
	cmd.SetContext(ctx)
	cmd.SetArgs([]string{"object", "not_a_real_kind_xyz_12345", "--title", "x"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for unknown kind")
	}
	if !strings.Contains(err.Error(), "not_a_real_kind_xyz_12345") {
		t.Fatalf("error should mention kind: %v", err)
	}
}

func TestNewObject_commandSpecRejectsCASAuthoring(t *testing.T) {
	t.Setenv(zqkenv.TestRoot().Name(), "")
	cmd := NewNewCmd()
	cmd.SetContext(context.Background())
	cmd.SetArgs([]string{"object", "command_spec", "--title", "Split brain"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "file-authored CLI DNA") {
		t.Fatalf("expected canonical file-DNA guidance, got %v", err)
	}
}

func TestCommandSpecOutputPath(t *testing.T) {
	t.Parallel()
	specsDir := t.TempDir()
	got, use, err := commandSpecOutputPath(specsDir, "system/validate-foo")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(specsDir, "system", "validate_foo_command.yaml")
	if got != want || use != "validate-foo" {
		t.Fatalf("path/use = %q/%q, want %q/%q", got, use, want, "validate-foo")
	}
	if _, _, err := commandSpecOutputPath(specsDir, "../escape"); err == nil {
		t.Fatal("expected traversal path to be rejected")
	}
}

func TestResolveCommandSpecsDir_staysWithinProjectRoot(t *testing.T) {
	t.Parallel()
	projectRoot := t.TempDir()
	want := filepath.Join(projectRoot, paths.CLICommandSpecsDir)
	if got := resolveCommandSpecsDir(projectRoot); got != want {
		t.Fatalf("command specs dir = %q, want %q", got, want)
	}
}

func TestNewInternal_subcommandRemoved(t *testing.T) {
	var buf bytes.Buffer
	ctx := pkgctx.WithCommandOutputWriter(context.Background(), &buf)
	cmd := NewNewCmd()
	cmd.SetContext(ctx)
	cmd.SetArgs([]string{"internal", "object_spec", "-o", "-"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error: new internal removed (use object template / new object-spec)")
	}
	msg := err.Error()
	if !strings.Contains(msg, "unknown command") && !strings.Contains(msg, "internal") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewRootHelp_prefersObjectInternal(t *testing.T) {
	cmd := NewNewCmd()
	long := cmd.Long
	if strings.Contains(long, "zqk internal create") {
		t.Fatalf("stale internal create help still present: %s", long)
	}
	if !strings.Contains(long, "object create <kind> --internal") {
		t.Fatalf("expected elevated object create help, got: %s", long)
	}
	if !strings.Contains(long, "new command-spec") {
		t.Fatalf("expected canonical command DNA veneer in help, got: %s", long)
	}
}

func TestNewBundle_stdout_contains_scenario_bundle(t *testing.T) {
	var buf bytes.Buffer
	ctx := pkgctx.WithCommandOutputWriter(context.Background(), &buf)
	cmd := NewNewCmd()
	cmd.SetContext(ctx)
	cmd.SetArgs([]string{"bundle", "-o", "-", "--name", "t-test"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "kind: scenario_bundle") {
		t.Fatalf("expected bundle kind in stdout, got: %q", out)
	}
}

// TestNewBundle_stdout_matches_NEW_COMMAND_contract asserts minimal scenario_bundle scaffold fields
// (docs/architecture/NEW_COMMAND_OBJECT_ORIGINATION.md).
func TestNewBundle_stdout_matches_NEW_COMMAND_contract(t *testing.T) {
	var buf bytes.Buffer
	ctx := pkgctx.WithCommandOutputWriter(context.Background(), &buf)
	cmd := NewNewCmd()
	cmd.SetContext(ctx)
	cmd.SetArgs([]string{"bundle", "-o", "-", "--name", "contract-test", "--description", "e2e desc"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, needle := range []string{
		"api_version: v1",
		"kind: scenario_bundle",
		"metadata:",
		"name: contract-test",
		"description: e2e desc",
		"objects:",
	} {
		if !strings.Contains(out, needle) {
			t.Fatalf("stdout missing %q:\n%s", needle, out)
		}
	}
}

func TestNewBundle_defaultOutput_createsDraftAndLastDraftPointer(t *testing.T) {
	tmp := t.TempDir()
	if err := fileutil.EnsureDir(filepath.Join(tmp, paths.ProjectDataDir)); err != nil {
		t.Fatal(err)
	}
	t.Setenv(zqkenv.ProjectRoot().Name(), tmp)

	var buf bytes.Buffer
	ctx := pkgctx.WithCommandOutputWriter(context.Background(), &buf)
	cmd := NewNewCmd()
	cmd.SetContext(ctx)
	cmd.SetArgs([]string{"bundle", "--name", "my-bundle"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	matches, err := filepath.Glob(filepath.Join(tmp, paths.ProjectDataDir, "drafts", "my-bundle-*.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected one bundle draft, got %v (stdout=%q)", matches, buf.String())
	}
	ptrPath := filepath.Join(tmp, paths.ProjectDataDir, "drafts", "last-draft.yaml")
	ptrBytes, err := fileutil.ReadFile(ptrPath)
	if err != nil {
		t.Fatalf("last-draft pointer: %v", err)
	}
	s := string(ptrBytes)
	if !strings.Contains(s, "scope: bundle") || !strings.Contains(s, "kind: scenario_bundle") {
		t.Fatalf("unexpected pointer: %s", s)
	}
}

func TestNewObjectSpecKind_stdout_contains_inheritance_and_storage_profile(t *testing.T) {
	var buf bytes.Buffer
	ctx := pkgctx.WithCommandOutputWriter(context.Background(), &buf)
	cmd := NewNewCmd()
	cmd.SetContext(ctx)
	cmd.SetArgs([]string{"object-spec", "draft_kind_xyz", "--extends", "base_object", "-o", "-"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "ontology: draft_kind_xyz") {
		t.Fatalf("expected ontology in draft, got: %q", out)
	}
	if !strings.Contains(out, "Inheritance model") {
		t.Fatalf("expected inheritance comment block, got: %q", out)
	}
	if !strings.Contains(out, "storage_profile: cas_entity") {
		t.Fatalf("expected inherited storage_profile, got: %q", out)
	}
}

func TestShouldAutoTracePipeline(t *testing.T) {
	cmd := NewNewCmd()
	leaf := cmd.Commands()[0]
	if leaf.Name() != "object" {
		for _, c := range cmd.Commands() {
			if c.Name() == "object" {
				leaf = c
				break
			}
		}
	}
	if shouldAutoTracePipeline(leaf, objects.KindBacklogItem) {
		t.Fatal("backlog_item must not auto-run trace pipeline")
	}
	t.Setenv(zqkenv.TestRoot().Name(), t.TempDir())
	if shouldAutoTracePipeline(leaf, objects.KindRequirement) {
		t.Fatal("test root must skip auto trace pipeline")
	}
}

// TestAutoTracePipeline_BLI1789335658105469000_Discipline tests the fail-closed mint discipline
// where requirement, goal, and milestone mints must auto-run gen-trace-pipeline unless skipped or in test.
// POL-AGENT-TPM-TRACE-PIPELINE-001
func TestAutoTracePipeline_BLI1789335658105469000_Discipline(t *testing.T) {
	cmd := NewNewCmd()
	var leaf *cobra.Command
	for _, c := range cmd.Commands() {
		if c.Name() == "object" {
			leaf = c
			break
		}
	}
	if leaf == nil {
		t.Fatal("object subcommand not found")
	}

	// 1. Qualifying kinds (requirement, goal, milestone) trigger trace pipeline in non-test mode
	for _, kind := range []string{objects.KindRequirement, objects.KindGoal, objects.KindMilestone} {
		if !shouldAutoTracePipelineConfigured(leaf, kind, false, false) {
			t.Errorf("kind %s should auto-run trace pipeline in standard production mode", kind)
		}
	}

	// 2. Non-qualifying kinds never trigger trace pipeline
	for _, kind := range []string{objects.KindBacklogItem, objects.KindCriteria, objects.KindTestCase, objects.KindAgentTask} {
		if shouldAutoTracePipelineConfigured(leaf, kind, false, false) {
			t.Errorf("kind %s must never auto-run trace pipeline", kind)
		}
	}

	// 3. In-test mode or test root suppress auto trace pipeline
	if shouldAutoTracePipelineConfigured(leaf, objects.KindRequirement, true, false) {
		t.Error("inTest=true must suppress auto trace pipeline")
	}
	if shouldAutoTracePipelineConfigured(leaf, objects.KindRequirement, false, true) {
		t.Error("hasTestRoot=true must suppress auto trace pipeline")
	}

	// 4. --skip-trace-pipeline flag suppresses auto trace pipeline
	skipCmd := NewNewCmd()
	var skipLeaf *cobra.Command
	for _, c := range skipCmd.Commands() {
		if c.Name() == "object" {
			skipLeaf = c
			break
		}
	}
	if skipLeaf != nil && skipLeaf.Flags().Lookup("skip-trace-pipeline") != nil {
		if err := skipLeaf.Flags().Set("skip-trace-pipeline", "true"); err != nil {
			t.Fatalf("failed to set skip-trace-pipeline flag: %v", err)
		}
		if shouldAutoTracePipelineConfigured(skipLeaf, objects.KindRequirement, false, false) {
			t.Error("--skip-trace-pipeline must suppress auto trace pipeline")
		}
	}
}
