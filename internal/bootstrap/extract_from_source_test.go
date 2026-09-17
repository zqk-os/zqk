package bootstrap

import (
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

func TestFindSourceProjectRoot_findsRepoWithProcessInternal(t *testing.T) {
	t.Parallel()
	root := FindSourceProjectRoot()
	if root == "" {
		t.Fatal("expected FindSourceProjectRoot to locate repo with process _internal")
	}
	internal := filepath.Join(root, paths.ProcessInternalDir)
	if st, err := fileutil.Stat(internal); err != nil || !st.IsDir() {
		t.Fatalf("expected %s to exist as directory: %v", internal, err)
	}
}

func TestExtractFiles_sourceFallbackCopiesConfig(t *testing.T) {
	t.Parallel()
	srcRoot := FindSourceProjectRoot()
	if srcRoot == "" {
		t.Skip("no source project root")
	}
	dst := t.TempDir()
	if err := ExtractFiles(dst, nil, true); err != nil {
		t.Fatalf("ExtractFiles: %v", err)
	}
	// At least one required config should land (embedded or source).
	candidates := []string{
		filepath.Join(dst, paths.ProcessInternalDir, "namespaces_config.yaml"),
		filepath.Join(dst, paths.ProcessInternalConfigsDir, "namespaces_config.yaml"),
	}
	found := false
	for _, c := range candidates {
		if _, err := fileutil.Stat(c); err == nil {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected namespaces_config.yaml under %s/_internal", dst)
	}
}
