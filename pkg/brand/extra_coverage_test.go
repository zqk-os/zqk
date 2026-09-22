package brand

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBrand_ExtraCoverage(t *testing.T) {
	// 1. EnvPrefix and EnvVar
	pfx := EnvPrefix()
	if pfx == "" {
		t.Errorf("expected non-empty EnvPrefix")
	}
	ev := EnvVar("TEST_KEY")
	if ev != pfx+"_TEST_KEY" {
		t.Errorf("unexpected EnvVar: %s", ev)
	}

	// 2. ProductName and SetProductName
	origProd := ProductName()
	t.Cleanup(func() { SetProductName(origProd) })
	SetProductName("AcmeCorp")
	if ProductName() != "AcmeCorp" {
		t.Errorf("expected AcmeCorp, got %s", ProductName())
	}
	SetProductName("") // should be ignored
	if ProductName() != "AcmeCorp" {
		t.Errorf("expected AcmeCorp after empty SetProductName, got %s", ProductName())
	}

	// 3. NamespacePrefix and SetNamespacePrefix
	origNS := NamespacePrefix()
	t.Cleanup(func() { SetNamespacePrefix(origNS) })
	SetNamespacePrefix("acme")
	if NamespacePrefix() != "acme" {
		t.Errorf("expected acme, got %s", NamespacePrefix())
	}
	SetNamespacePrefix("") // should be ignored
	if NamespacePrefix() != "acme" {
		t.Errorf("expected acme after empty SetNamespacePrefix, got %s", NamespacePrefix())
	}

	// 4. ProductNamespacePrefix suffix checks
	if p := ProductNamespacePrefix("brand-admin"); p != "brand" {
		t.Errorf("expected brand, got %s", p)
	}
	if p := ProductNamespacePrefix("brand-admin-admin"); p != "brand" {
		t.Errorf("expected brand, got %s", p)
	}
	if p := ProductNamespacePrefix(""); p != defaultNamespacePrefixValue {
		t.Errorf("expected default namespace for empty prefix, got %s", p)
	}

	// 5. ApplyCanonicalExecutable
	content := "Run zqk command now"
	if res := ApplyCanonicalExecutable(content, ""); res != content {
		t.Errorf("expected unchanged content for empty exe, got %s", res)
	}
	if res := ApplyCanonicalExecutable(content, CanonicalExecutableToken); res != content {
		t.Errorf("expected unchanged content for canonical token, got %s", res)
	}
	if res := ApplyCanonicalExecutable(content, "mytool"); res != "Run mytool command now" {
		t.Errorf("expected mytool replacement, got %s", res)
	}

	// 6. LoadFromProject
	tempDir := t.TempDir()
	cfgDir := filepath.Join(tempDir, "config")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}

	yamlContent := []byte(`
brand:
  executable_name: "acme-cli"
  product_name: "Acme"
  namespace_prefix: "acme"
`)
	if err := os.WriteFile(filepath.Join(cfgDir, "zqk.yaml"), yamlContent, 0o644); err != nil {
		t.Fatalf("write zqk.yaml: %v", err)
	}

	localYaml := []byte(`
brand:
  executable_name: "acme-local"
`)
	if err := os.WriteFile(filepath.Join(cfgDir, "zqk-local.yaml"), localYaml, 0o644); err != nil {
		t.Fatalf("write zqk-local.yaml: %v", err)
	}

	cfg := LoadFromProject(tempDir)
	if cfg.ExecutableName != "acme-local" || cfg.ProductName != "Acme" || cfg.NamespacePrefix != "acme" {
		t.Errorf("unexpected loaded brand config: %+v", cfg)
	}
}
