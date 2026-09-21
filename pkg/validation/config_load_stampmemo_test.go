package validation

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func writeStampMoved(t *testing.T, path, body string) {
	t.Helper()
	if err := fileutil.WriteFile(path, []byte(body), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	later := time.Now().Add(2 * time.Second)
	if err := fileutil.Chtimes(path, later, later); err != nil {
		t.Fatal(err)
	}
}

func TestLoadIDPrefixesConfig_reloadsWhenStampMoves(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), paths.IdPrefixesConfigFile)
	writeStampMoved(t, path, "kind_to_prefixes:\n  decision:\n    - DEC-\n")
	first, err := LoadIDPrefixesConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := first.GetPrefixesForKind("decision"); len(got) != 1 || got[0] != "DEC-" {
		t.Fatalf("got %#v", got)
	}
	first.KindToPrefixes["decision"] = []string{"MUTATED-"}
	writeStampMoved(t, path, "kind_to_prefixes:\n  decision:\n    - DEC-\n    - ADR-\n")
	second, err := LoadIDPrefixesConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := second.GetPrefixesForKind("decision"); len(got) != 2 {
		t.Fatalf("expected reload after stamp move, got %#v", got)
	}
}

func TestLoadNamespacesConfig_reloadsWhenStampMoves(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), paths.NamespacesConfigFile)
	writeStampMoved(t, path, "default_namespace: zqk:kernel\nnamespaces:\n  zqk:kernel:\n    kinds: [decision]\n")
	first, err := LoadNamespacesConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if first.GetNamespaceForKind("decision") != "zqk:kernel" {
		t.Fatalf("got %q", first.GetNamespaceForKind("decision"))
	}
	writeStampMoved(t, path, "default_namespace: zqk:other\nnamespaces:\n  zqk:other:\n    kinds: [decision]\n")
	second, err := LoadNamespacesConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if second.GetNamespaceForKind("decision") != "zqk:other" {
		t.Fatalf("expected reload after stamp move, got %q", second.GetNamespaceForKind("decision"))
	}
}

func TestLoadPathsConfig_reloadsWhenStampMoves(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), paths.PathsConfigFile)
	writeStampMoved(t, path, "paths:\n  object_specs: one\n")
	first, err := LoadPathsConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if first.Paths["object_specs"] != "one" {
		t.Fatalf("got %#v", first.Paths)
	}
	first.Paths["object_specs"] = "mutated"
	writeStampMoved(t, path, "paths:\n  object_specs: two\n")
	second, err := LoadPathsConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if second.Paths["object_specs"] != "two" {
		t.Fatalf("expected reload after stamp move, got %#v", second.Paths)
	}
}

func TestLoadValidationTimeoutConfig_reloadsWhenStampMoves(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "zqk.yaml")
	writeStampMoved(t, path, "validation:\n  per_object_timeout:\n    default_seconds: 3\n")
	first, err := LoadValidationTimeoutConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if first.DefaultSeconds != 3 {
		t.Fatalf("got %d", first.DefaultSeconds)
	}
	first.DefaultSeconds = 99
	writeStampMoved(t, path, "validation:\n  per_object_timeout:\n    default_seconds: 7\n")
	second, err := LoadValidationTimeoutConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if second.DefaultSeconds != 7 {
		t.Fatalf("expected reload after stamp move, got %d", second.DefaultSeconds)
	}
}

func TestLoadValidationTierConfig_reloadsWhenStampMoves(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "zqk.yaml")
	writeStampMoved(t, path, "validation:\n  tier_config:\n    blocking_tiers: [1]\n")
	first, err := LoadValidationTierConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.BlockingTiers) != 1 || first.BlockingTiers[0] != 1 {
		t.Fatalf("got %#v", first.BlockingTiers)
	}
	first.BlockingTiers[0] = 9
	writeStampMoved(t, path, "validation:\n  tier_config:\n    blocking_tiers: [1, 2]\n")
	second, err := LoadValidationTierConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.BlockingTiers) != 2 {
		t.Fatalf("expected reload after stamp move, got %#v", second.BlockingTiers)
	}
}
