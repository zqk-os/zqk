package storage

import (
	"bufio"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func storageDecomposeRepoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func countFileLines(path string) (int, error) {
	f, err := fileutil.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	n := 0
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		n++
	}
	return n, sc.Err()
}

func TestCRITStorageFileSizeNamedFiles(t *testing.T) {
	t.Parallel()
	root := storageDecomposeRepoRoot(t)
	named := []string{
		"pkg/storage/object_storage_file_update.go",
		"pkg/storage/audit_aggregation.go",
		"pkg/storage/object_storage_file_list_main.go",
		"cmd/zqk/agent/orchestrate.go",
	}
	for _, rel := range named {
		p := filepath.Join(root, rel)
		n, err := countFileLines(p)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			t.Fatalf("%s: %v", rel, err)
		}
		if n > 800 {
			t.Errorf("CRIT-CEF-STORAGE-FILE-SIZE-001: %s has %d lines, want <=800", rel, n)
		}
	}
}

func TestCRITStorageSubpackageInterfaces(t *testing.T) {
	t.Parallel()
	root := storageDecomposeRepoRoot(t)
	need := []string{
		"pkg/storage/crud/crud_facade.go",
		"pkg/storage/cas/cas_facade.go",
		"pkg/storage/audit/writer.go",
		"pkg/storage/migration/interfaces.go",
		"pkg/storage/systemcheck/checker.go",
		"pkg/storage/root_facade.go",
		"cmd/zqk/system/migrate_cas.go",
		"cmd/zqk/system/migrate_dsia.go",
	}
	for _, rel := range need {
		if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
			t.Errorf("CRIT-CEF-STORAGE-SUBPACKAGES-001 missing %s: %v", rel, err)
		}
	}
}

func TestCRITOrchestrateOver800HasTRACK(t *testing.T) {
	t.Parallel()
	root := storageDecomposeRepoRoot(t)
	dir := filepath.Join(root, "cmd", "zqk", "agent")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasPrefix(name, "orchestrate") || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		rel := filepath.Join("cmd/zqk/agent", name)
		p := filepath.Join(dir, name)
		n, err := countFileLines(p)
		if err != nil {
			t.Fatalf("%s: %v", rel, err)
		}
		if n <= 800 {
			continue
		}
		body, err := fileutil.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(body), "TRACK:") {
			t.Errorf("CRIT-CEF-STORAGE-FILE-SIZE-001: %s has %d lines and no TRACK split", rel, n)
		}
	}
}
