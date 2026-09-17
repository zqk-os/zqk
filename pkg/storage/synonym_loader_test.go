package storage_test

import (
	"context"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lanceman/zqk/pkg/datacell"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/storage"

	"gopkg.in/yaml.v3"

	"github.com/lanceman/zqk/pkg/utils/fileutil"
)

func moduleRootFromGoEnv(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("go", "env", "GOMOD").Output()
	if err != nil {
		t.Fatalf("go env GOMOD: %v", err)
	}
	modPath := strings.TrimSpace(string(out))
	if modPath == "" || modPath == "/dev/null" {
		t.Skip("no module root (GOMOD empty or not in module context)")
	}
	return filepath.Dir(modPath)
}

type testSettingsShape struct {
	Version string         `yaml:"version"`
	Paths   map[string]any `yaml:"paths"`
}

func writeMinimalTestSettingsFile(testRoot string) error {
	path := filepath.Join(testRoot, paths.TestSettingsFilename)
	body := testSettingsShape{
		Version: paths.DefaultBrandSettingsVersion,
		Paths:   map[string]any{},
	}
	data, err := yaml.Marshal(body)
	if err != nil {
		return err
	}
	return fileutil.WriteSecureFile(path, data) //nolint:gosec // test file
}

// copyDirForTest copies a directory tree from src to dst (same behavior as pkg/testing.CopyDir; kept here to avoid pkg/testing).
func copyDirForTest(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fileutil.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		targetPath := filepath.Join(dst, relPath)
		if d.IsDir() {
			return fileutil.EnsureDir(targetPath)
		}
		srcFile, err := fileutil.Open(path)
		if err != nil {
			return err
		}
		defer srcFile.Close()
		if err := fileutil.EnsureDir(filepath.Dir(targetPath)); err != nil {
			return err
		}
		dstFile, err := fileutil.Create(targetPath)
		if err != nil {
			return err
		}
		defer dstFile.Close()
		if _, err := io.Copy(dstFile, srcFile); err != nil {
			return err
		}
		if fi, statErr := fileutil.Stat(path); statErr == nil {
			if chmodErr := fileutil.Chmod(targetPath, fi.Mode()); chmodErr != nil {
				return chmodErr
			}
		}
		return nil
	})
}

func setupSynonymLoaderTestRootLikeSetupTestEnvironment(t *testing.T) string {
	t.Helper()
	testRoot := t.TempDir()
	absRoot, err := filepath.Abs(testRoot)
	if err != nil {
		t.Fatalf("failed to resolve test root: %v", err)
	}
	if err := paths.LayoutUnder(absRoot).
		Dir(paths.ProjectDataDir, paths.DirPerm755).
		Dir(paths.ProcessDir, paths.DirPerm755).
		Dir(paths.ProcessInternalObjectSpecsDir, paths.DirPerm755).
		Dir(paths.ProcessInternalTraitsDir, paths.DirPerm755).
		Err(); err != nil {
		t.Fatalf("failed to create test project layout: %v", err)
	}
	if err := writeMinimalTestSettingsFile(absRoot); err != nil {
		t.Fatalf("failed to write test settings: %v", err)
	}
	modRoot := moduleRootFromGoEnv(t)
	src := datacell.CellCASPrimaryDir(modRoot, "kind_synonyms")
	dst := datacell.CellCASPrimaryDir(absRoot, "kind_synonyms")
	if err := copyDirForTest(src, dst); err != nil {
		t.Fatalf("copy kind_synonyms from module root: %v", err)
	}
	return absRoot
}

func TestStorageSynonymLoader_LoadFromFiles(t *testing.T) {
	tmpDir := t.TempDir()
	storage.MustEnsureProcessSpecsLayoutForTest(t, tmpDir)

	processDir := datacell.ProcessPrimaryDir(tmpDir)
	kindSynonymsDir := filepath.Join(processDir, "kind_synonyms")
	if err := fileutil.EnsureDir(kindSynonymsDir); err != nil {
		t.Fatalf("Failed to create test directories: %v", err)
	}

	testSynonyms := []struct {
		id         string
		targetKind string
		synonym    string
		priority   int
	}{
		{"SYN-001", "backlog_item", "bi", 10},
		{"SYN-002", "backlog_item", "bli", 5},
		{"SYN-003", "priority_plan", "pp", 10},
	}

	for _, ts := range testSynonyms {
		filePath := filepath.Join(kindSynonymsDir, ts.id+".yaml")
		priorityStr := "10"
		if ts.priority == 5 {
			priorityStr = "5"
		}
		content := `id: ` + ts.id + `
kind: kind_synonym
schema_version: "` + objects.DefaultSchemaVersion + `"
target_kind: ` + ts.targetKind + `
synonym: ` + ts.synonym + `
priority: ` + priorityStr + `
`
		if err := fileutil.WriteSecureFile(filePath, []byte(content)); err != nil {
			t.Fatalf("Failed to write test synonym file: %v", err)
		}
	}

	fos, err := storage.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("NewFileObjectStorageForTest: %v", err)
	}

	defer func() { _ = fos.Shutdown(context.Background()) }()
	t.Cleanup(func() {
		opts := storage.TempProjectTeardown(tmpDir, fos)
		if err := storage.RunProjectTestTeardown(opts); err != nil {
			t.Logf("project test teardown: %v", err)
		}
	})

	loader := storage.NewStorageSynonymLoader(fos)

	synonyms, err := loader.LoadSynonyms()
	if err != nil {
		t.Fatalf("LoadSynonyms() failed: %v", err)
	}

	if len(synonyms) != len(testSynonyms) {
		t.Errorf("LoadSynonyms() returned %d synonyms, want %d", len(synonyms), len(testSynonyms))
	}

	synonymMap := make(map[string]objects.SynonymData)
	for _, s := range synonyms {
		key := s.Kind + ":" + s.Synonym
		synonymMap[key] = s
	}

	for _, ts := range testSynonyms {
		key := ts.targetKind + ":" + ts.synonym
		syn, exists := synonymMap[key]
		if !exists {
			t.Errorf("Synonym %q for %q not found", ts.synonym, ts.targetKind)
			continue
		}
		if syn.Kind != ts.targetKind {
			t.Errorf("Synonym %q has kind %q, want %q", ts.synonym, syn.Kind, ts.targetKind)
		}
		if syn.Synonym != ts.synonym {
			t.Errorf("Synonym has value %q, want %q", syn.Synonym, ts.synonym)
		}
		if syn.Priority != ts.priority {
			t.Errorf("Synonym %q has priority %d, want %d", ts.synonym, syn.Priority, ts.priority)
		}
	}
}

func TestStorageSynonymLoader_EmptyStorage(t *testing.T) {
	tmpDir := t.TempDir()
	storage.MustEnsureProcessSpecsLayoutForTest(t, tmpDir)

	fos, err := storage.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("NewFileObjectStorageForTest: %v", err)
	}

	defer func() { _ = fos.Shutdown(context.Background()) }()
	t.Cleanup(func() {
		opts := storage.TempProjectTeardown(tmpDir, fos)
		if err := storage.RunProjectTestTeardown(opts); err != nil {
			t.Logf("project test teardown: %v", err)
		}
	})

	loader := storage.NewStorageSynonymLoader(fos)
	synonyms, err := loader.LoadSynonyms()
	if err != nil {
		t.Fatalf("LoadSynonyms() failed: %v", err)
	}

	if len(synonyms) != 0 {
		t.Errorf("LoadSynonyms() returned %d synonyms, want 0", len(synonyms))
	}
}

// TestStorageSynonymLoader_WithRealProjectData loads kind_synonym YAML copied from the repo into a temp root;
// storage is never opened on the live checkout.
func TestStorageSynonymLoader_WithRealProjectData(t *testing.T) {
	t.Skip("kind_synonym objects are no longer stored in the repo data, skipping test")
}
