package kindpack

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/testkit"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func writeTestPack(t *testing.T, name, kind string) string {
	t.Helper()
	dir := t.TempDir()
	manifest := "name: " + name + "\nkinds:\n  - " + kind + "\n"
	if err := fileutil.WriteFile(filepath.Join(dir, "kindpack.yaml"), []byte(manifest), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	specDir := filepath.Join(dir, "specs")
	if err := fileutil.EnsureDir(specDir); err != nil {
		t.Fatal(err)
	}
	spec := "ontology: " + kind + "\nschema_version: 2.0.0\nfields:\n  - name: title\n    type: string\n"
	if err := fileutil.WriteFile(filepath.Join(specDir, kind+".yaml"), []byte(spec), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	lifecycleDir := filepath.Join(dir, "lifecycles")
	if err := fileutil.EnsureDir(lifecycleDir); err != nil {
		t.Fatal(err)
	}
	life := "name: " + kind + "\nstatuses:\n  - name: originated\n"
	if err := fileutil.WriteFile(filepath.Join(lifecycleDir, kind+"_lifecycle.yaml"), []byte(life), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestNewKindPackCmd_Hierarchy(t *testing.T) {
	cmd := NewKindPackCmd()
	if cmd == nil {
		t.Fatal("expected NewKindPackCmd to return non-nil command")
	}
	if cmd.Use == "" {
		t.Fatal("expected non-empty command Use")
	}
	var foundInstall bool
	for _, sub := range cmd.Commands() {
		if sub.Name() == "install" {
			foundInstall = true
		}
	}
	if !foundInstall {
		t.Fatal("expected install subcommand under kindpack")
	}
}

func executeKindPack(args ...string) (string, error) {
	cmd := NewKindPackCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return buf.String(), err
}

func TestKindPack_Install(t *testing.T) {
	testkit.PrepareIsolatedTempProject(t, nil)
	packDir := writeTestPack(t, "test-gadget-pack", "gadget")

	out, err := executeKindPack("install", packDir)
	if err != nil {
		t.Fatalf("kindpack install failed: %v, out: %s", err, out)
	}
	if !strings.Contains(out, "recorded test-gadget-pack (gadget)") {
		t.Fatalf("expected recorded output, got: %s", out)
	}
}

func TestKindPack_Install_MissingArg(t *testing.T) {
	if _, err := executeKindPack("install"); err == nil {
		t.Fatal("expected error executing install without args")
	}
}

func TestKindPack_Install_InvalidPack(t *testing.T) {
	emptyDir := t.TempDir()
	if _, err := executeKindPack("install", emptyDir); err == nil {
		t.Fatal("expected error installing invalid pack directory")
	}
}

func TestJoinKinds(t *testing.T) {
	tests := []struct {
		kinds    []string
		expected string
	}{
		{nil, ""},
		{[]string{}, ""},
		{[]string{"alpha"}, "alpha"},
		{[]string{"alpha", "beta"}, "alpha,beta"},
		{[]string{"alpha", "beta", "gamma"}, "alpha,beta,gamma"},
	}
	for _, tt := range tests {
		got := joinKinds(tt.kinds)
		if got != tt.expected {
			t.Errorf("joinKinds(%v) = %q, want %q", tt.kinds, got, tt.expected)
		}
	}
}
