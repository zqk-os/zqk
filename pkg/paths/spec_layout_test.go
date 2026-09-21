package paths

import (
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestFindObjectSpecFile_flatWins(t *testing.T) {
	root := t.TempDir()
	specs := filepath.Join(root, "objects")
	if err := fileutil.EnsureDir(filepath.Join(specs, "kernel")); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteStandardFile(filepath.Join(specs, "account.yaml"), []byte("kind: account\n")); err != nil {
		t.Fatal(err)
	}
	got, err := FindObjectSpecFile(specs, "account")
	if err != nil {
		t.Fatal(err)
	}
	if got != filepath.Join(specs, "account.yaml") {
		t.Fatalf("got %q", got)
	}
}

func TestFindObjectSpecFile_domainFallback(t *testing.T) {
	root := t.TempDir()
	specs := filepath.Join(root, "objects")
	kernel := filepath.Join(specs, "kernel")
	if err := fileutil.EnsureDir(kernel); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(kernel, "role.yaml")
	if err := fileutil.WriteStandardFile(want, []byte("kind: role\n")); err != nil {
		t.Fatal(err)
	}
	got, err := FindObjectSpecFile(specs, "role")
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestFindLifecycleFile_domainFallback(t *testing.T) {
	root := t.TempDir()
	lc := filepath.Join(root, "lifecycles")
	kernel := filepath.Join(lc, "kernel")
	if err := fileutil.EnsureDir(kernel); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(kernel, "account_lifecycle.yaml")
	if err := fileutil.WriteStandardFile(want, []byte("object_type: account\n")); err != nil {
		t.Fatal(err)
	}
	got := FindLifecycleFile(lc, "account")
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestObjectSpecPathCandidates(t *testing.T) {
	t.Parallel()
	got := ObjectSpecPathCandidates("/proj/.zqk/specs/objects", "account")
	if len(got) != 1+len(ObjectSpecDomainDirs) {
		t.Fatalf("candidates=%d", len(got))
	}
	if filepath.Base(got[0]) != "account.yaml" {
		t.Fatalf("flat=%q", got[0])
	}
}
