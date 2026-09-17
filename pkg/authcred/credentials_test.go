package authcred

import (
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/utils/fileutil"
	"github.com/lanceman/zqk/pkg/zqkenv"
)

func TestResolveCredentialPath_isolatedTestRootDoesNotUseHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv(zqkenv.OSHome().Name(), home)
	if err := fileutil.EnsureDir(filepath.Join(home, paths.ProjectDataDir)); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteSecureFile(filepath.Join(home, paths.ProjectDataDir, "credentials"), []byte("ACC-HOME\n")); err != nil {
		t.Fatal(err)
	}
	isolate := t.TempDir()
	t.Setenv(zqkenv.TestRoot().Name(), isolate)
	got := ResolveCredentialPath(t.TempDir())
	want := filepath.Join(isolate, paths.ProjectDataDir, "credentials")
	if got != want {
		t.Fatalf("got %q want isolated %q", got, want)
	}
}

func TestResolveCredentialPath_liveTestRootFallsBackToHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv(zqkenv.OSHome().Name(), home)
	if err := fileutil.EnsureDir(filepath.Join(home, paths.ProjectDataDir)); err != nil {
		t.Fatal(err)
	}
	homeCred := filepath.Join(home, paths.ProjectDataDir, "credentials")
	if err := fileutil.WriteSecureFile(homeCred, []byte("ACC-HOME\n")); err != nil {
		t.Fatal(err)
	}
	project := t.TempDir()
	t.Setenv(zqkenv.TestRoot().Name(), project)
	got := ResolveCredentialPath(project)
	if got != homeCred {
		t.Fatalf("got %q want home %q", got, homeCred)
	}
}

func TestResolveCredentialPath_prefersExistingTestRootFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv(zqkenv.OSHome().Name(), home)
	isolate := t.TempDir()
	credDir := filepath.Join(isolate, paths.ProjectDataDir)
	if err := fileutil.EnsureDir(credDir); err != nil {
		t.Fatal(err)
	}
	isolateCred := filepath.Join(credDir, "credentials")
	if err := fileutil.WriteSecureFile(isolateCred, []byte("ACC-TEST\n")); err != nil {
		t.Fatal(err)
	}
	t.Setenv(zqkenv.TestRoot().Name(), isolate)
	got := ResolveCredentialPath(t.TempDir())
	if got != isolateCred {
		t.Fatalf("got %q want %q", got, isolateCred)
	}
}
