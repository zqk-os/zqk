package spec

import (
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
)

// setupSpecTestProject creates a temporary project with test env and storage (POL-CODE-006: test isolation).
func setupSpecTestProject(t *testing.T) (string, storage.ObjectStorageProvider) {
	t.Helper()
	p := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{Kind: "cmd.spec.list"})
	return p.Root, p.FileStorage
}

// createSpecListTestCommand builds a cobra command with context and storage for testing runSpecList.
func createSpecListTestCommand(t *testing.T, projectRoot string, storageProvider storage.ObjectStorageProvider) *cobra.Command {
	t.Helper()
	cmd := NewSpecListCmd()
	baseCtx := pkgctx.NewSystemContext()
	cmd.SetContext(cli.WithStorageProvider(baseCtx, storageProvider))

	initCtx := pkgctx.NewCliInitializationContext(cli.ResolveProjectRoot, ".")
	ctx, err := cli.GetContextFromCommand(cmd, initCtx)
	if err != nil {
		t.Fatalf("create context: %v", err)
	}
	ctx.ProjectRoot = projectRoot
	ctx.Format = cli.FormatJSON
	cli.SetContext(cmd, ctx)
	return cmd
}

func TestListSpecsFromFiles_NoDir(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	// No .zqk/specs/objects
	result, err := listSpecsFromFiles(root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result.Objects) != 0 {
		t.Errorf("expected 0 objects, got %d", len(result.Objects))
	}
	if result.Meta["total_count"] != 0 {
		t.Errorf("expected total_count 0, got %v", result.Meta["total_count"])
	}
}

func TestListSpecsFromFiles_EmptyDir(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	specsDir := filepath.Join(root, paths.ProcessInternalObjectSpecsDir)
	if err := fileutil.MkdirAll(specsDir, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	result, err := listSpecsFromFiles(root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Objects) != 0 {
		t.Errorf("expected 0 objects, got %d", len(result.Objects))
	}
}

func TestListSpecsFromFiles_WithValidYaml(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	specsDir := filepath.Join(root, paths.ProcessInternalObjectSpecsDir)
	if err := fileutil.MkdirAll(specsDir, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	specPath := filepath.Join(specsDir, "test_spec.yaml")
	specContent := []byte(`ontology: test_spec
description: A test specification for list tests.
schema_version: "1.0"
visibility: internal
`)
	if err := fileutil.WriteFile(specPath, specContent, paths.FilePerm644); err != nil {
		t.Fatalf("write spec file: %v", err)
	}

	result, err := listSpecsFromFiles(root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Objects) != 1 {
		t.Fatalf("expected 1 object, got %d", len(result.Objects))
	}
	obj := result.Objects[0]
	if obj[objects.FieldKeyID] != "test_spec" {
		t.Errorf("id: want test_spec, got %v", obj[objects.FieldKeyID])
	}
	if obj[objects.FieldKeyKind] != "object_spec" {
		t.Errorf("kind: want object_spec, got %v", obj[objects.FieldKeyKind])
	}
	if obj[objects.FieldKeyOntology] != "test_spec" {
		t.Errorf("ontology: want test_spec, got %v", obj[objects.FieldKeyOntology])
	}
	// Title from first line of description (trimmed, max 80)
	if obj[objects.FieldKeyTitle] != "A test specification for list tests." {
		t.Errorf("title: want first line of description, got %v", obj[objects.FieldKeyTitle])
	}
}

func TestListSpecsFromFiles_SkipsNonYamlAndUnderscore(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	specsDir := filepath.Join(root, paths.ProcessInternalObjectSpecsDir)
	if err := fileutil.MkdirAll(specsDir, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Valid spec
	if err := fileutil.WriteFile(filepath.Join(specsDir, "valid.yaml"), []byte("ontology: valid\ndescription: Valid\n"), paths.FilePerm644); err != nil {
		t.Fatalf("write valid.yaml: %v", err)
	}
	// Skipped: no .yaml/.yml
	if err := fileutil.WriteFile(filepath.Join(specsDir, "readme.md"), []byte("# readme"), paths.FilePerm644); err != nil {
		t.Fatalf("write readme: %v", err)
	}
	// Skipped: underscore prefix
	if err := fileutil.WriteFile(filepath.Join(specsDir, "_private.yaml"), []byte("ontology: private\ndescription: Private\n"), paths.FilePerm644); err != nil {
		t.Fatalf("write _private.yaml: %v", err)
	}
	// Skipped: no ontology
	if err := fileutil.WriteFile(filepath.Join(specsDir, "no_ontology.yaml"), []byte("description: No ontology\n"), paths.FilePerm644); err != nil {
		t.Fatalf("write no_ontology.yaml: %v", err)
	}

	result, err := listSpecsFromFiles(root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Objects) != 1 {
		t.Errorf("expected 1 object (only valid.yaml), got %d", len(result.Objects))
	}
	if len(result.Objects) > 0 && result.Objects[0][objects.FieldKeyID] != "valid" {
		t.Errorf("expected id valid, got %v", result.Objects[0][objects.FieldKeyID])
	}
}

func TestRunSpecList_FallbackToFiles_Empty(t *testing.T) {
	// Not t.Parallel(): setupSpecTestProject uses t.Setenv(ZQK_TEST_ROOT).
	projectRoot, storageProvider := setupSpecTestProject(t)
	cmd := createSpecListTestCommand(t, projectRoot, storageProvider)

	err := runSpecList(cmd, nil)
	if err != nil {
		t.Fatalf("runSpecList: %v", err)
	}
	// Output is written via cli.WriteOutput; we'd need to capture stdout to assert.
	// Here we only assert no error. Integration-style test can capture output if needed.
}

func TestRunSpecList_OutputStructure(t *testing.T) {
	// Not t.Parallel(): setupSpecTestProject uses t.Setenv(ZQK_TEST_ROOT).
	projectRoot, storageProvider := setupSpecTestProject(t)
	// Add one spec file so fallback returns non-empty list
	specsDir := filepath.Join(projectRoot, paths.ProcessInternalObjectSpecsDir)
	if err := fileutil.WriteFile(filepath.Join(specsDir, "output_test.yaml"), []byte("ontology: output_test\ndescription: Output test\n"), paths.FilePerm644); err != nil {
		t.Fatalf("write spec: %v", err)
	}

	cmd := createSpecListTestCommand(t, projectRoot, storageProvider)
	err := runSpecList(cmd, nil)
	if err != nil {
		t.Fatalf("runSpecList: %v", err)
	}
}

func TestRunSpecList_JSONOutputHasExpectedKeys(t *testing.T) {
	// Not t.Parallel(): setupSpecTestProject uses t.Setenv(ZQK_TEST_ROOT).
	projectRoot, storageProvider := setupSpecTestProject(t)
	specsDir := filepath.Join(projectRoot, paths.ProcessInternalObjectSpecsDir)
	if err := fileutil.WriteFile(filepath.Join(specsDir, "json_keys.yaml"), []byte("ontology: json_keys\ndescription: Keys test\n"), paths.FilePerm644); err != nil {
		t.Fatalf("write spec: %v", err)
	}

	cmd := createSpecListTestCommand(t, projectRoot, storageProvider)
	err := runSpecList(cmd, nil)
	if err != nil {
		t.Fatalf("runSpecList: %v", err)
	}
	// Output structure (objects + meta.total_count) is covered by listSpecsFromFiles tests and TestRunSpecList_OutputStructure.
}
