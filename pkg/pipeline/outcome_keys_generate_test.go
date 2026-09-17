package pipeline

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/utils/fileutil"
)

func TestLoadOutcomeKeyNamesFromYAML_duplicate(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	p := filepath.Join(dir, "registry.yaml")
	if err := fileutil.WriteSecureFile(p, []byte("schema_version: 1\nkeys:\n  - a\n  - a\n")); err != nil {
		t.Fatal(err)
	}
	_, err := LoadOutcomeKeyNamesFromYAML(p)
	if err == nil {
		t.Fatal("expected error for duplicate key")
	}
}

func TestGenerateOutcomeKeysGoSource_roundTrip(t *testing.T) {
	t.Parallel()
	names := []string{"alpha_key", "beta_thing"}
	src, err := GenerateOutcomeKeysGoSource(paths.ProcessInternalPipelineOutcomeKeysFile, names)
	if err != nil {
		t.Fatal(err)
	}
	if len(src) < 100 {
		t.Fatalf("unexpected short source: %q", string(src))
	}
	for _, sub := range []string{"OutcomeKeyAlphaKey", `"alpha_key"`, "OutcomeKeyBetaThing", `"beta_thing"`} {
		if !strings.Contains(string(src), sub) {
			t.Fatalf("generated source missing %q:\n%s", sub, string(src))
		}
	}
}
