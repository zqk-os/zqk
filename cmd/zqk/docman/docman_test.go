package docman

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
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func setupDocmanTestWorkspace(t *testing.T) (string, func()) {
	t.Helper()
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		SkipSetupTestEnvironment: false,
		SkipFileStorage:          false,
	})
	tmpDir := proj.Root

	t.Setenv(zqkenv.ProjectRoot().Name(), tmpDir)
	t.Setenv(zqkenv.TestRoot().Name(), tmpDir)

	origDir, err := fileutil.Getwd()
	if err != nil {
		t.Fatalf("failed to get current working directory: %v", err)
	}

	if err := fileutil.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to change working directory to temp project root: %v", err)
	}

	cleanup := func() {
		_ = fileutil.Chdir(origDir)
	}
	t.Cleanup(cleanup)

	return tmpDir, cleanup
}

func writeDocmanTestFile(t *testing.T, baseDir, relPath, content string) string {
	t.Helper()
	fullPath := filepath.Join(baseDir, relPath)
	if err := fileutil.MkdirAll(filepath.Dir(fullPath), paths.DirPerm755); err != nil {
		t.Fatalf("failed to create directory structure for %s: %v", relPath, err)
	}
	if err := fileutil.WriteFile(fullPath, []byte(content), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write test document %s: %v", relPath, err)
	}
	return fullPath
}

func executeDocmanCommandWithOutput(cmd *cobra.Command, args []string) (string, error) {
	var buf bytes.Buffer
	ctx := pkgctx.WithCommandOutputWriter(pkgctx.NewSystemContext(), &buf)
	cmd.SetContext(ctx)
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return buf.String(), err
}

func TestDocman_RootCommandTree(t *testing.T) {
	cmd := NewDocmanCmd()
	if cmd == nil {
		t.Fatal("expected NewDocmanCmd to return non-nil command")
	}
	if cmd.Use != "docman" {
		t.Errorf("expected command Use to be 'docman', got %q", cmd.Use)
	}

	subCommands := cmd.Commands()
	expectedSubs := map[string]bool{"register": false, "verify": false}
	for _, sub := range subCommands {
		if _, ok := expectedSubs[sub.Name()]; ok {
			expectedSubs[sub.Name()] = true
		}
	}
	for name, found := range expectedSubs {
		if !found {
			t.Errorf("expected subcommand %q to be present under docman", name)
		}
	}

	out, err := executeDocmanCommandWithOutput(cmd, []string{"--help"})
	if err != nil {
		t.Fatalf("failed to execute docman --help: %v", err)
	}
	if !strings.Contains(out, "Documentation management operations") {
		t.Errorf("expected help output to describe docman operations, got:\n%s", out)
	}
}

func TestDocmanRegister_DryRunAndSubtrees(t *testing.T) {
	tmpDir, _ := setupDocmanTestWorkspace(t)

	writeDocmanTestFile(t, tmpDir, "docs/architecture/spec.md", "# System Specification\nContent for spec.")
	writeDocmanTestFile(t, tmpDir, "docs/guides/howto.md", "# Guide\nContent for howto.")

	// Test dry-run
	cmdDry := NewRegisterCmd()
	dryOut, err := executeDocmanCommandWithOutput(cmdDry, []string{"--dry-run", "--subtrees=docs/architecture"})
	if err != nil {
		t.Fatalf("register --dry-run failed: %v", err)
	}
	if !strings.Contains(dryOut, "Would create") {
		t.Errorf("expected dry-run output to contain 'Would create', got:\n%s", dryOut)
	}

	// Verify no objects actually persisted
	factory, err := storage.NewStorageFactory(context.Background(), tmpDir)
	if err != nil {
		t.Fatalf("storage factory failed: %v", err)
	}
	sp := factory.GetStorageForKind(objects.KindDocEntry)
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()

	resBefore, err := sp.List(context.Background(), secCtx, storageCtx, storage.ListFilter{Kind: objects.KindDocEntry})
	if err != nil {
		t.Fatalf("list doc_entry failed: %v", err)
	}
	if len(resBefore.Objects) != 0 {
		t.Fatalf("expected 0 doc_entry objects after dry-run, found %d", len(resBefore.Objects))
	}

	// Now run real register targeting only docs/architecture
	cmdReal := NewRegisterCmd()
	realOut, err := executeDocmanCommandWithOutput(cmdReal, []string{"--subtrees=docs/architecture"})
	if err != nil {
		t.Fatalf("register with --subtrees failed: %v", err)
	}
	if !strings.Contains(realOut, "Created") {
		t.Errorf("expected register output to contain 'Created', got:\n%s", realOut)
	}

	resAfter, err := sp.List(context.Background(), secCtx, storageCtx, storage.ListFilter{Kind: objects.KindDocEntry})
	if err != nil {
		t.Fatalf("list doc_entry failed: %v", err)
	}
	if len(resAfter.Objects) != 1 {
		t.Fatalf("expected 1 doc_entry registered for docs/architecture, got %d", len(resAfter.Objects))
	}
}

func TestDocmanRegister_UpdateExisting(t *testing.T) {
	tmpDir, _ := setupDocmanTestWorkspace(t)

	writeDocmanTestFile(t, tmpDir, "docs/best-practices/code.md", "# Best Practices\nClean code principles.")

	// Initial registration
	initCmd := NewRegisterCmd()
	_, err := executeDocmanCommandWithOutput(initCmd, []string{"--shipped-only"})
	if err != nil {
		t.Fatalf("initial register failed: %v", err)
	}

	// Run register with update-existing
	updateCmd := NewRegisterCmd()
	out, err := executeDocmanCommandWithOutput(updateCmd, []string{"--update-existing"})
	if err != nil {
		t.Fatalf("register with --update-existing failed: %v", err)
	}
	if !strings.Contains(out, "Updated") {
		t.Errorf("expected update-existing output to contain 'Updated', got:\n%s", out)
	}

	// Also run update-existing with --dry-run
	updateDryCmd := NewRegisterCmd()
	_, err = executeDocmanCommandWithOutput(updateDryCmd, []string{"--update-existing", "--dry-run"})
	if err != nil {
		t.Fatalf("register with --update-existing --dry-run failed: %v", err)
	}
}

func TestDocmanVerify_SpecificIDsAndMissingTarget(t *testing.T) {
	tmpDir, _ := setupDocmanTestWorkspace(t)

	docRel := "docs/architecture/removable.md"
	fullPath := writeDocmanTestFile(t, tmpDir, docRel, "# Removable Doc\nTemporary documentation.")

	// Register
	regCmd := NewRegisterCmd()
	_, err := executeDocmanCommandWithOutput(regCmd, []string{"--shipped-only"})
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}

	// Get ID
	factory, err := storage.NewStorageFactory(context.Background(), tmpDir)
	if err != nil {
		t.Fatalf("storage factory failed: %v", err)
	}
	sp := factory.GetStorageForKind(objects.KindDocEntry)
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()

	listRes, err := sp.List(context.Background(), secCtx, storageCtx, storage.ListFilter{Kind: objects.KindDocEntry})
	if err != nil || len(listRes.Objects) == 0 {
		t.Fatalf("failed to retrieve doc_entry object: %v", err)
	}
	docID, _ := listRes.Objects[0][objects.FieldKeyID].(string)

	// Verify specifically by ID
	idVerifyCmd := NewVerifyCmd()
	idOut, err := executeDocmanCommandWithOutput(idVerifyCmd, []string{"--ids", docID})
	if err != nil {
		t.Fatalf("verify with --ids flag failed: %v, output: %s", err, idOut)
	}
	if !strings.Contains(idOut, "Scanned 1 doc_entry") {
		t.Errorf("expected verify output to show 1 scanned object, got:\n%s", idOut)
	}

	// Now remove file to simulate missing target
	if err := fileutil.Remove(fullPath); err != nil {
		t.Fatalf("failed to remove document file: %v", err)
	}

	// Verify with --strict=false should report missing target without error
	missingVerifyCmd := NewVerifyCmd()
	missingOut, err := executeDocmanCommandWithOutput(missingVerifyCmd, []string{"--ids", docID, "--strict=false"})
	if err != nil {
		t.Fatalf("verify with missing target and strict=false should not return error: %v", err)
	}
	if !strings.Contains(missingOut, "Missing target: 1") {
		t.Errorf("expected verify output to report 'Missing target: 1', got:\n%s", missingOut)
	}

	// Verify with --strict=true should fail closed
	strictVerifyCmd := NewVerifyCmd()
	_, err = executeDocmanCommandWithOutput(strictVerifyCmd, []string{"--ids", docID, "--strict=true"})
	if err == nil {
		t.Fatalf("expected verify with missing target and strict=true to fail closed")
	}
}

func TestDocmanContext_FlagResolution(t *testing.T) {
	cmd := NewRegisterCmd()
	cmd.Flags().Set("context", "debug")
	cmd.Flags().Set("shipped-only", "true")
	cmd.Flags().Set("subtrees", "docs/architecture,docs/onboarding")

	subtrees, shipped, err := resolveSubtreesAndShipped(cmd)
	if err != nil {
		t.Fatalf("resolveSubtreesAndShipped failed: %v", err)
	}
	if !shipped {
		t.Errorf("expected shipped to be true")
	}
	if len(subtrees) != 2 {
		t.Errorf("expected 2 subtrees, got %v", subtrees)
	}
}
