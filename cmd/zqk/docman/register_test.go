package docman

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/internal/cli"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/docman"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestDocmanRegister_ShippedOnly(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{SkipSetupTestEnvironment: true, SkipFileStorage: false})
	tmpDir := proj.Root

	// Create files in shipped subtrees and non-shipped
	docs := map[string]string{
		"docs/architecture/arch.md":      "# Architecture\nDesign overview.",
		"docs/best-practices/bp.md":      "# Best Practices\nStandards.",
		"docs/onboarding/first_run.md":   "# First Run\nWalkthrough.",
		"docs/onboarding/archive/old.md": "# Old\nDeprecated.",
		"docs/launch/launch.md":          "# Launch\nNotes.",
	}
	for rel, content := range docs {
		full := filepath.Join(tmpDir, rel)
		if err := fileutil.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatalf("mkdir failed: %v", err)
		}
		if err := fileutil.WriteFile(full, []byte(content), 0644); err != nil {
			t.Fatalf("write file failed: %v", err)
		}
	}
	t.Setenv(zqkenv.ProjectRoot().Name(), tmpDir)
	t.Setenv(zqkenv.TestRoot().Name(), tmpDir)

	// Change cwd
	originalDir, err := fileutil.Getwd()
	if err != nil {
		t.Fatalf("getwd failed: %v", err)
	}
	defer fileutil.Chdir(originalDir)
	if err := fileutil.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir failed: %v", err)
	}

	// Run docman register --shipped-only
	t.Logf("resolved projectRoot=%s, tmpDir=%s", cli.ResolveProjectRoot("."), tmpDir)
	cmd := NewRegisterCmd()
	var outBuf bytes.Buffer
	cmd.SetOut(&outBuf)
	cmd.SetArgs([]string{"--shipped-only"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("register --shipped-only failed: %v", err)
	}

	// Verify doc_entries in storage
	factory, err := storage.NewStorageFactory(context.Background(), tmpDir)
	if err != nil {
		t.Fatalf("storage factory failed: %v", err)
	}
	sp := factory.GetStorageForKind(objects.KindDocEntry)
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()

	res, err := sp.List(context.Background(), secCtx, storageCtx, storage.ListFilter{Kind: objects.KindDocEntry})
	if err != nil {
		t.Fatalf("list doc_entry failed: %v", err)
	}

	registered := make(map[string]bool)
	for _, obj := range res.Objects {
		p, _ := obj[objects.FieldKeyPath].(string)
		clean := paths.NormalizeDocEntryPathForKey(p)
		registered[clean] = true
	}
	t.Logf("registered count=%d: %v", len(registered), registered)

	for _, exp := range docman.ShippedInitDocSubtrees {
		var found bool
		for regPath := range registered {
			if filepath.HasPrefix(regPath, exp) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected at least one doc registered under %s", exp)
		}
	}

	if registered["docs/onboarding/archive/old.md"] {
		t.Errorf("archive doc should NOT be registered")
	}
	if registered["docs/launch/launch.md"] {
		t.Errorf("launch doc should NOT be registered with --shipped-only")
	}
}
