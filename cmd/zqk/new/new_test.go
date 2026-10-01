package newcmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
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
	t.Cleanup(func() {
		_ = storage.RunProjectTestTeardown(storage.TempProjectTeardown(testRoot, nil))
		_ = fileutil.RemoveAll(filepath.Join(testRoot, paths.ProjectDataDir))
	})

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
	matches, err := filepath.Glob(filepath.Join(tmp, paths.ProjectDataDir, paths.DraftsSubdir, "my-bundle-*.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected one bundle draft, got %v (stdout=%q)", matches, buf.String())
	}
	ptrPath := filepath.Join(tmp, paths.ProjectDataDir, paths.DraftsSubdir, "last-draft.yaml")
	ptrBytes, err := fileutil.ReadFile(ptrPath)
	if err != nil {
		t.Fatalf("last-draft pointer: %v", err)
	}
	s := string(ptrBytes)
	if !strings.Contains(s, "scope: bundle") || !strings.Contains(s, "kind: scenario_bundle") {
		t.Fatalf("unexpected pointer: %s", s)
	}
}

func TestNewScenario_alias_stdout_contains_scenario_bundle(t *testing.T) {
	var buf bytes.Buffer
	ctx := pkgctx.WithCommandOutputWriter(context.Background(), &buf)
	cmd := NewNewCmd()
	cmd.SetContext(ctx)
	cmd.SetArgs([]string{"scenario", "-o", "-", "--name", "scenario-alias-test"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "kind: scenario_bundle") || !strings.Contains(out, "name: scenario-alias-test") {
		t.Fatalf("expected scenario_bundle in stdout, got: %q", out)
	}
}

func TestNewSwarm_stdout_contains_swarm_package(t *testing.T) {
	var buf bytes.Buffer
	ctx := pkgctx.WithCommandOutputWriter(context.Background(), &buf)
	cmd := NewNewCmd()
	cmd.SetContext(ctx)
	cmd.SetArgs([]string{"swarm", "-o", "-", "--name", "my-swarm-test", "--description", "Autonomous migration pipeline"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, needle := range []string{
		"name: my-swarm-test",
		"version: 1.0.0",
		"description: Autonomous migration pipeline",
		"entrypoint: task-execute",
		"agents:",
		"tasks:",
		"task-execute",
		"task-verify",
	} {
		if !strings.Contains(out, needle) {
			t.Fatalf("stdout missing %q:\n%s", needle, out)
		}
	}
}

func TestNewSwarm_cellular_archetype_stdout(t *testing.T) {
	var buf bytes.Buffer
	ctx := pkgctx.WithCommandOutputWriter(context.Background(), &buf)
	cmd := NewNewCmd()
	cmd.SetContext(ctx)
	cmd.SetArgs([]string{"swarm-manifest", "-o", "-", "--name", "neuron-cell", "--cell-type", "neuron"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "team_configuration:") || !strings.Contains(out, "cell_type: neuron") {
		t.Fatalf("expected cellular team_configuration in stdout, got:\n%s", out)
	}
}

func TestNewSwarm_defaultOutput_createsDraftAndLastDraftPointer(t *testing.T) {
	tmp := t.TempDir()
	if err := fileutil.EnsureDir(filepath.Join(tmp, paths.ProjectDataDir)); err != nil {
		t.Fatal(err)
	}
	t.Setenv(zqkenv.ProjectRoot().Name(), tmp)

	var buf bytes.Buffer
	ctx := pkgctx.WithCommandOutputWriter(context.Background(), &buf)
	cmd := NewNewCmd()
	cmd.SetContext(ctx)
	cmd.SetArgs([]string{"swarm", "--name", "my-draft-swarm"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	matches, err := filepath.Glob(filepath.Join(tmp, paths.ProjectDataDir, paths.DraftsSubdir, "my-draft-swarm-*.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected one swarm draft, got %v (stdout=%q)", matches, buf.String())
	}
	ptrPath := filepath.Join(tmp, paths.ProjectDataDir, paths.DraftsSubdir, "last-draft.yaml")
	ptrBytes, err := fileutil.ReadFile(ptrPath)
	if err != nil {
		t.Fatalf("last-draft pointer: %v", err)
	}
	s := string(ptrBytes)
	if !strings.Contains(s, "scope: swarm") || !strings.Contains(s, "kind: swarm_package") {
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

func TestRunNewCommandSpec_RichScaffold(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("ZQK_TEST_ROOT", tmpDir)

	root := &cobra.Command{Use: "zqk"}
	newCmd := NewNewCmd()
	root.AddCommand(newCmd)

	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs([]string{
		"new", "command-spec", "testpkg/my-action",
		"--short", "Perform a test action",
		"--description", "Detailed description of the test action.",
		"--force",
	})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	specPath := filepath.Join(tmpDir, ".zqk/cli/specs/testpkg/my_action_command.yaml")
	data, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatalf("failed to read generated spec: %v", err)
	}

	var parsed map[string]any
	if err := yaml.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to parse generated spec YAML: %v", err)
	}

	if parsed["name"] != "my-action" {
		t.Errorf("expected name 'my-action', got %v", parsed["name"])
	}
	if parsed["short"] != "Perform a test action" {
		t.Errorf("expected short 'Perform a test action', got %v", parsed["short"])
	}
	if parsed["common_flags"] != true {
		t.Errorf("expected common_flags true, got %v", parsed["common_flags"])
	}
	if parsed["run_e"] != "runTestpkgMyAction" {
		t.Errorf("expected run_e 'runTestpkgMyAction', got %v", parsed["run_e"])
	}

	argsMap, ok := parsed["args"].(map[string]any)
	if !ok || argsMap["type"] != "no_args" {
		t.Errorf("expected args.type 'no_args', got %v", parsed["args"])
	}

	helpMap, ok := parsed["help"].(map[string]any)
	if !ok {
		t.Fatalf("expected help block, got %v", parsed["help"])
	}
	examples, ok := helpMap["examples"].([]any)
	if !ok || len(examples) == 0 {
		t.Errorf("expected generated help examples, got %v", helpMap["examples"])
	}
}

func TestRunNewCommandSpec_FromCmdIntrospection(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("ZQK_TEST_ROOT", tmpDir)

	root := &cobra.Command{Use: "zqk"}
	newCmd := NewNewCmd()
	root.AddCommand(newCmd)

	// Register a mock command with flags, aliases, and examples in root
	sampleCmd := &cobra.Command{
		Use:     "sample-worker <target>",
		Aliases: []string{"worker", "sw"},
		Short:   "Execute worker on target",
		Long:    "Long form description of the worker processing steps.",
		Example: "# Sample invocation\nzqk sample-worker foo --count 10",
	}
	sampleCmd.Flags().IntP("count", "c", 5, "Number of items to process")
	sampleCmd.Flags().Bool("dry-run", false, "Simulate execution without mutations")
	root.AddCommand(sampleCmd)

	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs([]string{
		"new", "command-spec", "sample-worker",
		"--from-cmd",
		"--force",
	})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	specPath := filepath.Join(tmpDir, ".zqk/cli/specs/sample_worker_command.yaml")
	data, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatalf("failed to read generated spec: %v", err)
	}

	var parsed map[string]any
	if err := yaml.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to parse generated spec YAML: %v", err)
	}

	if parsed["name"] != "sample-worker <target>" {
		t.Errorf("expected name 'sample-worker <target>', got %v", parsed["name"])
	}
	if parsed["short"] != "Execute worker on target" {
		t.Errorf("expected short 'Execute worker on target', got %v", parsed["short"])
	}
	if parsed["description"] != "Long form description of the worker processing steps." {
		t.Errorf("expected long description, got %v", parsed["description"])
	}

	aliases, ok := parsed["aliases"].([]any)
	if !ok || len(aliases) != 2 || aliases[0] != "worker" || aliases[1] != "sw" {
		t.Errorf("expected aliases ['worker', 'sw'], got %v", parsed["aliases"])
	}

	argsMap, ok := parsed["args"].(map[string]any)
	if !ok || argsMap["type"] != "exact" || argsMap["count"] != 1 {
		t.Errorf("expected args {type: exact, count: 1}, got %v", parsed["args"])
	}

	flags, ok := parsed["flags"].([]any)
	if !ok || len(flags) != 2 {
		t.Fatalf("expected 2 flags introspected, got %v", parsed["flags"])
	}

	flag0 := flags[0].(map[string]any)
	if flag0["name"] != "count" || flag0["type"] != "int" || flag0["shorthand"] != "c" || flag0["default"] != 5 {
		t.Errorf("unexpected flag0: %v", flag0)
	}

	flag1 := flags[1].(map[string]any)
	if flag1["name"] != "dry-run" || flag1["type"] != "bool" || flag1["default"] != false {
		t.Errorf("unexpected flag1: %v", flag1)
	}
}

func TestRunNewCommandSpec_CustomFlagsAndExamples(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("ZQK_TEST_ROOT", tmpDir)

	root := &cobra.Command{Use: "zqk"}
	newCmd := NewNewCmd()
	root.AddCommand(newCmd)

	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs([]string{
		"new", "command-spec", "compute/hash",
		"--short", "Hash input data",
		"--description", "Computes cryptographic hashes of inputs.",
		"--aliases", "sha,digest",
		"--args-type", "exact",
		"--args-count", "1",
		"--flag", "algo:string:sha256:Algorithm to use:a",
		"--flag", "iterations:int:1000:Number of iterations",
		"--example", "Standard hash:%s compute hash my-file",
		"--force",
	})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	specPath := filepath.Join(tmpDir, ".zqk/cli/specs/compute/hash_command.yaml")
	data, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatalf("failed to read generated spec: %v", err)
	}

	var parsed map[string]any
	if err := yaml.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to parse generated spec YAML: %v", err)
	}

	aliases := parsed["aliases"].([]any)
	if len(aliases) != 2 || aliases[0] != "sha" || aliases[1] != "digest" {
		t.Errorf("expected aliases ['sha', 'digest'], got %v", parsed["aliases"])
	}

	argsMap := parsed["args"].(map[string]any)
	if argsMap["type"] != "exact" || argsMap["count"] != 1 {
		t.Errorf("expected args {type: exact, count: 1}, got %v", argsMap)
	}

	flags := parsed["flags"].([]any)
	if len(flags) != 2 {
		t.Fatalf("expected 2 flags, got %v", flags)
	}
	f0 := flags[0].(map[string]any)
	if f0["name"] != "algo" || f0["type"] != "string" || f0["default"] != "sha256" || f0["shorthand"] != "a" {
		t.Errorf("unexpected flag0: %v", f0)
	}

	f1 := flags[1].(map[string]any)
	if f1["name"] != "iterations" || f1["type"] != "int" || f1["default"] != "1000" {
		t.Errorf("unexpected flag1: %v", f1)
	}

	helpMap := parsed["help"].(map[string]any)
	examples := helpMap["examples"].([]any)
	if len(examples) != 1 {
		t.Fatalf("expected 1 example, got %v", examples)
	}
	ex0 := examples[0].(map[string]any)
	if ex0["comment"] != "Standard hash" || ex0["command"] != "%s compute hash my-file" {
		t.Errorf("unexpected example: %v", ex0)
	}
}
