package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	commsystem "github.com/zqk-os/zqk/cmd/zqk-community/system"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestFirstRunRootHelpOmitsStudioAdminSurface(t *testing.T) {
	cmd := NewRootCommand()
	got := map[string]struct{}{}
	for _, sub := range cmd.Commands() {
		got[sub.Name()] = struct{}{}
	}
	for _, banned := range []string{"healthchk", "learn", "inbox", "keystore", "join", "internal", "automation"} {
		if _, ok := got[banned]; ok {
			t.Fatalf("first-run root still ships %q", banned)
		}
	}
	for _, need := range []string{"object", "system", "test", "workflow", "scheduler", "mcp"} {
		if _, ok := got[need]; !ok {
			t.Fatalf("first-run root missing %q", need)
		}
	}
}

func TestFirstRunSystemOmitsSpecOrigination(t *testing.T) {
	t.Parallel()
	cmd := commsystem.NewSystemCmd()
	for _, sub := range cmd.Commands() {
		if sub.Name() == "spec-origination" || sub.Name() == "update-specs" || sub.Name() == "federate" {
			t.Fatalf("open-core system still ships %q", sub.Name())
		}
	}
}

func TestShippedFirstRunDocsExist(t *testing.T) {
	t.Parallel()
	cwd, err := fileutil.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Clean(filepath.Join(cwd, "../../.."))
	for _, rel := range []string{
		"docs/onboarding/COMMUNITY_FIRST_RUN.md",
		"docs/onboarding/QUICKSTART.md",
		"docs/architecture/README.md",
		"docs/architecture/INDEX.md",
		"docs/INDEX.md",
	} {
		if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
			t.Fatalf("missing shipped first-run doc %s: %v", rel, err)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "docs/_archive")); !os.IsNotExist(err) {
		t.Fatal("docs/_archive must not ship")
	}
}

func TestPublicModulePath(t *testing.T) {
	t.Parallel()
	cwd, err := fileutil.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Clean(filepath.Join(cwd, "../../.."))
	goMod, err := fileutil.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	if first, _, _ := strings.Cut(string(goMod), "\n"); first != "module github.com/zqk-os/zqk" {
		t.Fatalf("go.mod module line = %q", first)
	}
	privateModule := strings.Join([]string{"github.com", "lanceman", "zqk"}, "/")

	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", ".zqk", "vendor":
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" {
			return nil
		}
		data, readErr := fileutil.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if strings.Contains(string(data), privateModule) {
			t.Errorf("leftover private module import: %s", strings.TrimPrefix(path, root+string(filepath.Separator)))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
