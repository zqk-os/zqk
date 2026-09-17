package vds

import (
	"strings"

	"github.com/lanceman/zqk/pkg/errfmt"
)

// Durable instruction export formats (renderer keys — not CLI surface names).
const (
	FormatIDEMDC = "ide_mdc"
)

// DefaultProvidersRel is optional standalone providers file (merged under customization).
const DefaultProvidersRel = "docs/quality/vds_vendor_providers.yaml"

// VendorProvidersConfig lists instruction export targets for this project.
type VendorProvidersConfig struct {
	SchemaVersion int              `yaml:"schema_version" json:"schema_version"`
	Default       string           `yaml:"default" json:"default,omitempty"`
	Providers     []VendorProvider `yaml:"providers" json:"providers"`
}

// VendorProvider is one configured export target (id is stable; path/format vary).
type VendorProvider struct {
	ID      string `yaml:"id" json:"id"`
	Format  string `yaml:"format" json:"format"`
	Out     string `yaml:"out" json:"out"`
	Enabled *bool  `yaml:"enabled" json:"enabled,omitempty"` // nil = true
}

// IsEnabled reports whether the provider should be projected.
func (p VendorProvider) IsEnabled() bool {
	if p.Enabled == nil {
		return true
	}
	return *p.Enabled
}

// DefaultVendorProviders returns the stock IDE pack when customization omits providers.
func DefaultVendorProviders() VendorProvidersConfig {
	return VendorProvidersConfig{
		SchemaVersion: 1,
		Default:       "ide",
		Providers: []VendorProvider{
			{
				ID:     "ide",
				Format: FormatIDEMDC,
				Out:    ".ide/rules/verifiable-decomposition-spine.mdc",
			},
		},
	}
}

// ResolveVendorProviders picks customization.vendor_providers or defaults.
func ResolveVendorProviders(cust *Customization) VendorProvidersConfig {
	if cust != nil && len(cust.VendorProviders.Providers) > 0 {
		cfg := cust.VendorProviders
		if strings.TrimSpace(cfg.Default) == "" && len(cfg.Providers) > 0 {
			cfg.Default = cfg.Providers[0].ID
		}
		return cfg
	}
	return DefaultVendorProviders()
}

// LookupProvider returns a provider by id (case-sensitive trim).
func LookupProvider(cfg VendorProvidersConfig, id string) (VendorProvider, error) {
	want := strings.TrimSpace(id)
	if want == "" {
		want = strings.TrimSpace(cfg.Default)
	}
	if want == "" {
		return VendorProvider{}, errfmt.Errorf("vds project: no --provider and no default in vendor_providers config")
	}
	for _, p := range cfg.Providers {
		if p.ID == want {
			if !p.IsEnabled() {
				return VendorProvider{}, errfmt.Errorf("vds project: provider %q is disabled in config", want)
			}
			if strings.TrimSpace(p.Format) == "" || strings.TrimSpace(p.Out) == "" {
				return VendorProvider{}, errfmt.Errorf("vds project: provider %q needs format and out", want)
			}
			return p, nil
		}
	}
	var ids []string
	for _, p := range cfg.Providers {
		ids = append(ids, p.ID)
	}
	return VendorProvider{}, errfmt.Errorf("vds project: unknown provider %q (configured: %s)", want, strings.Join(ids, ", "))
}

// EnabledProviders returns enabled providers in config order.
func EnabledProviders(cfg VendorProvidersConfig) []VendorProvider {
	var out []VendorProvider
	for _, p := range cfg.Providers {
		if p.IsEnabled() && strings.TrimSpace(p.ID) != "" && strings.TrimSpace(p.Out) != "" {
			out = append(out, p)
		}
	}
	return out
}
