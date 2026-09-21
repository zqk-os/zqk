package paths

import (
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestFirstProjectYAMLConfig_prefersRepoConfig(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, ConfigDir, ZqkConfigFileName)
	legacy := filepath.Join(root, ProjectDataDir, ConfigDir, ProjectConfigFile)
	if err := fileutil.EnsureDir(filepath.Dir(repo)); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.EnsureDir(filepath.Dir(legacy)); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteStandardFile(repo, []byte("project:\n  name: repo\n")); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteStandardFile(legacy, []byte("project:\n  name: kernel\n")); err != nil {
		t.Fatal(err)
	}
	got := FirstProjectYAMLConfig(root)
	want, err := filepath.EvalSymlinks(repo)
	if err != nil {
		want = repo
	}
	gotResolved, err := filepath.EvalSymlinks(got)
	if err != nil {
		gotResolved = got
	}
	if gotResolved != want {
		t.Fatalf("FirstProjectYAMLConfig() = %q, want repo config %q", got, repo)
	}
}

func TestFirstExistingFromCwdAny_prefersRepoConfig(t *testing.T) {
	root := t.TempDir()
	repoRel := filepath.Join(ConfigDir, ZqkConfigFileName)
	legacyRel := filepath.Join(ProjectDataDir, ConfigDir, ProjectConfigFile)
	repo := filepath.Join(root, repoRel)
	legacy := filepath.Join(root, legacyRel)
	if err := fileutil.EnsureDir(filepath.Dir(repo)); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.EnsureDir(filepath.Dir(legacy)); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteStandardFile(repo, []byte("ok: true\n")); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteStandardFile(legacy, []byte("ok: false\n")); err != nil {
		t.Fatal(err)
	}
	old, err := fileutil.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = fileutil.Chdir(old)
		ResetCwdDiscovery()
	})
	if err := fileutil.Chdir(root); err != nil {
		t.Fatal(err)
	}
	ResetCwdDiscovery()
	got := FirstExistingFromCwdAny(ProjectYAMLConfigRelatives())
	want, err := filepath.EvalSymlinks(repo)
	if err != nil {
		want = repo
	}
	gotResolved, err := filepath.EvalSymlinks(got)
	if err != nil {
		gotResolved = got
	}
	if gotResolved != want {
		t.Fatalf("got %q want %q", got, repo)
	}
}
