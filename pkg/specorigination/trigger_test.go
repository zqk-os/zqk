package specorigination

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestTruncateRunes(t *testing.T) {
	t.Parallel()
	if got := truncateRunes("hello", 10); got != "hello" {
		t.Fatalf("got %q", got)
	}
	long := strings.Repeat("a", 3000)
	got := truncateRunes(long, 10)
	if len([]rune(got)) != 11 { // 10 + ellipsis
		t.Fatalf("expected 11 runes, got %d: %q", len([]rune(got)), got)
	}
	if !strings.HasSuffix(got, "…") {
		t.Fatalf("expected ellipsis suffix: %q", got)
	}
}

func TestAdminTriggerBinaryPath(t *testing.T) {
	t.Parallel()

	binDir := filepath.Join(string(filepath.Separator), "opt", "zqk", "bin")
	admin := filepath.Join(binDir, adminBinaryName)
	if got := adminTriggerBinaryPath(filepath.Join(binDir, "zqk")); got != admin {
		t.Fatalf("primary binary resolved to %q, want %q", got, admin)
	}
	if got := adminTriggerBinaryPath(admin); got != admin {
		t.Fatalf("admin binary resolved to %q, want %q", got, admin)
	}

	projectRoot := filepath.Join(string(filepath.Separator), "work", "zqk")
	stable := filepath.Join(projectRoot, paths.ProjectDataDir, "bin", "zqk-stable")
	candidates := adminTriggerBinaryCandidates(stable)
	repoAdmin := filepath.Join(projectRoot, "bin", adminBinaryName)
	if len(candidates) != 2 || candidates[1] != repoAdmin {
		t.Fatalf("stable candidates = %#v, want repository fallback %q", candidates, repoAdmin)
	}
}

func TestGenerateSpecBuilder(t *testing.T) {
	root := t.TempDir()
	specDir := filepath.Join(root, paths.ProcessInternalObjectSpecsDir)
	if err := fileutil.MkdirAll(specDir, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	versionDir := filepath.Join(root, "pkg", "specbuilder", "bldr_v2")
	if err := fileutil.MkdirAll(versionDir, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	existingConstants := "package bldr_v2\n\nconst FieldName = \"name\"\n"
	if err := fileutil.WriteFile(filepath.Join(versionDir, "persona_constants.go"), []byte(existingConstants), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	spec := `schema_version: 2.0.0
ontology: trigger_probe
visibility: internal
fields:
  name:
    type: string
`
	if err := fileutil.WriteFile(filepath.Join(specDir, "trigger_probe.yaml"), []byte(spec), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}

	if err := generateSpecBuilder(root, "trigger_probe"); err != nil {
		t.Fatalf("generate spec builder: %v", err)
	}
	for _, name := range []string{"trigger_probe_builder.go", "trigger_probe_constants.go"} {
		if _, err := fileutil.Stat(filepath.Join(versionDir, name)); err != nil {
			t.Errorf("expected generated %s: %v", name, err)
		}
	}
	constants, err := fileutil.ReadFile(filepath.Join(versionDir, "trigger_probe_constants.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(constants), `TriggerProbeFieldName = "name"`) {
		t.Fatalf("expected collision-safe field constant, got:\n%s", constants)
	}
}
