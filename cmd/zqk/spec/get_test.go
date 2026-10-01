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

func createSpecGetTestCommand(t *testing.T, projectRoot string, storageProvider storage.ObjectStorageProvider, format cli.OutputFormat) *cobra.Command {
	t.Helper()
	cmd := NewSpecGetCmd()
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

func TestRunSpecGet_Success(t *testing.T) {
	projectRoot, storageProvider := setupSpecTestProject(t)

	specsDir := filepath.Join(projectRoot, paths.ProcessInternalObjectSpecsDir)
	if err := fileutil.MkdirAll(specsDir, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir specs: %v", err)
	}
	sampleSpec := `schema_version: "2.0.0"
ontology: "widget"
description: "A test widget object specification"
traits:
  - "base_object_traits"
fields:
  name:
    type: "string"
    validation:
      required: true
  count:
    type: "integer"
`
	if err := fileutil.WriteFile(filepath.Join(specsDir, "widget.yaml"), []byte(sampleSpec), paths.FilePerm644); err != nil {
		t.Fatalf("write widget spec: %v", err)
	}

	cmd := createSpecGetTestCommand(t, projectRoot, storageProvider, cli.FormatJSON)
	if err := runSpecGet(cmd, []string{"widget"}); err != nil {
		t.Fatalf("runSpecGet failed: %v", err)
	}
}

func TestRunSpecGet_NotFound(t *testing.T) {
	projectRoot, storageProvider := setupSpecTestProject(t)

	cmd := createSpecGetTestCommand(t, projectRoot, storageProvider, cli.FormatTable)
	err := runSpecGet(cmd, []string{"non_existent_kind_xyz"})
	if err == nil {
		t.Fatal("expected error for non-existent kind, got nil")
	}
}
