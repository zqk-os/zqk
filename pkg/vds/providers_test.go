package vds

import (
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestLookupProvider_unknown(t *testing.T) {
	_, err := LookupProvider(DefaultVendorProviders(), "nope")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLookupProvider_disabled(t *testing.T) {
	off := false
	cfg := VendorProvidersConfig{
		Default: "x",
		Providers: []VendorProvider{{
			ID: "x", Format: FormatIDEMDC, Out: "a.mdc", Enabled: &off,
		}},
	}
	_, err := LookupProvider(cfg, "x")
	if err == nil {
		t.Fatal("expected disabled error")
	}
}

func TestLookupProvider_missingFormatOrOut(t *testing.T) {
	cfg := VendorProvidersConfig{Providers: []VendorProvider{{ID: "x", Format: FormatIDEMDC}}}
	_, err := LookupProvider(cfg, "x")
	if err == nil {
		t.Fatal("expected missing out")
	}
}

func TestEnabledProviders_skipsDisabled(t *testing.T) {
	off := false
	cfg := VendorProvidersConfig{Providers: []VendorProvider{
		{ID: "a", Format: FormatIDEMDC, Out: "a.mdc"},
		{ID: "b", Format: FormatIDEMDC, Out: "b.mdc", Enabled: &off},
	}}
	got := EnabledProviders(cfg)
	if len(got) != 1 || got[0].ID != "a" {
		t.Fatalf("%+v", got)
	}
}

func TestResolveVendorProviders_fromCustomization(t *testing.T) {
	cust := &Customization{VendorProviders: VendorProvidersConfig{
		Providers: []VendorProvider{{ID: "claude", Format: "claude_md", Out: "x.md"}},
	}}
	cfg := ResolveVendorProviders(cust)
	if cfg.Default != "claude" || len(cfg.Providers) != 1 {
		t.Fatalf("%+v", cfg)
	}
}

func TestResolveVendorProviders_defaultsWhenEmpty(t *testing.T) {
	cfg := ResolveVendorProviders(&Customization{})
	if cfg.Default != "ide" || len(cfg.Providers) == 0 {
		t.Fatalf("%+v", cfg)
	}
}

func TestLoadCustomization_parsesVendorProviders(t *testing.T) {
	root := t.TempDir()
	const rel = "customization.yaml"
	body := []byte(`vendor_providers:
  default: cursor
  providers:
    - id: cursor
      format: ide_mdc
      out: .cursor/rules/vds.mdc
`)
	if err := fileutil.WriteFile(filepath.Join(root, rel), body, paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	cust, path, err := LoadCustomization(root, rel)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(path) == "" {
		t.Fatal("empty path")
	}
	if len(cust.VendorProviders.Providers) == 0 {
		t.Fatal("expected vendor_providers in repo customization")
	}
	p, err := LookupProvider(ResolveVendorProviders(cust), "")
	if err != nil || p.ID != "cursor" {
		t.Fatalf("%+v %v", p, err)
	}
}
