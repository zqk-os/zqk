package spec

import (
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func createSpecFieldsTestCommand(t *testing.T, projectRoot string, storageProvider storage.ObjectStorageProvider, format cli.OutputFormat) *cobra.Command {
	t.Helper()
	cmd := NewSpecFieldsCmd()
	baseCtx := pkgctx.NewSystemContext()
	cmd.SetContext(cli.WithStorageProvider(baseCtx, storageProvider))

	initCtx := pkgctx.NewCliInitializationContext(cli.ResolveProjectRoot, ".")
	ctx, err := cli.GetContextFromCommand(cmd, initCtx)
	if err != nil {
		t.Fatalf("create context: %v", err)
	}
	ctx.ProjectRoot = projectRoot
	if format != "" {
		ctx.Format = format
	}
	cli.SetContext(cmd, ctx)
	return cmd
}

func TestRunSpecFields_Success(t *testing.T) {
	projectRoot, storageProvider := setupSpecTestProject(t)

	specsDir := filepath.Join(projectRoot, paths.ProcessInternalObjectSpecsDir)
	if err := fileutil.MkdirAll(specsDir, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir specs: %v", err)
	}
	sampleSpec := `schema_version: "2.0.0"
ontology: "widget"
description: "A test widget object specification"
fields:
  name:
    type: "string"
    permissions: "rwx"
    purpose: "Widget title"
    validation:
      required: true
  count:
    type: "integer"
    validation:
      required: false
`
	if err := fileutil.WriteFile(filepath.Join(specsDir, "widget.yaml"), []byte(sampleSpec), paths.FilePerm644); err != nil {
		t.Fatalf("write widget spec: %v", err)
	}

	cmd := createSpecFieldsTestCommand(t, projectRoot, storageProvider, cli.FormatJSON)
	if err := runSpecFields(cmd, []string{"widget"}); err != nil {
		t.Fatalf("runSpecFields failed: %v", err)
	}
}

func TestRunSpecFields_RequiredOnly(t *testing.T) {
	projectRoot, storageProvider := setupSpecTestProject(t)

	specsDir := filepath.Join(projectRoot, paths.ProcessInternalObjectSpecsDir)
	if err := fileutil.MkdirAll(specsDir, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir specs: %v", err)
	}
	sampleSpec := `schema_version: "2.0.0"
ontology: "widget"
description: "A test widget object specification"
fields:
  name:
    type: "string"
    validation:
      required: true
  count:
    type: "integer"
    validation:
      required: false
`
	if err := fileutil.WriteFile(filepath.Join(specsDir, "widget.yaml"), []byte(sampleSpec), paths.FilePerm644); err != nil {
		t.Fatalf("write widget spec: %v", err)
	}

	cmd := createSpecFieldsTestCommand(t, projectRoot, storageProvider, cli.FormatJSON)
	_ = cmd.Flags().Set("required-only", "true")
	if err := runSpecFields(cmd, []string{"widget"}); err != nil {
		t.Fatalf("runSpecFields required-only failed: %v", err)
	}
}

func TestRunSpecFields_NotFound(t *testing.T) {
	projectRoot, storageProvider := setupSpecTestProject(t)

	cmd := createSpecFieldsTestCommand(t, projectRoot, storageProvider, cli.FormatTable)
	err := runSpecFields(cmd, []string{"non_existent_kind_xyz"})
	if err == nil {
		t.Fatal("expected error for non-existent kind, got nil")
	}
}
