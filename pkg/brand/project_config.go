package brand

import (
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// ProjectBrandRelPaths is the load order for brand.executable_name.
// config/zqk-local.yaml wins over committed config/zqk.yaml.
// .zqk/config/config.yaml is leftover and is not SSOT.
func ProjectBrandRelPaths() []string {
	return []string{
		filepath.Join("config", "zqk-local.yaml"),
		filepath.Join("config", "zqk.yaml"),
		filepath.Join(".zqk", "config", "config.yaml"),
	}
}

// ProjectFile is the brand subset of config/zqk.yaml and config/zqk-local.yaml.
type ProjectFile struct {
	Brand *ProjectBrandBlock `yaml:"brand,omitempty"`
	// Top-level keys are leftover compatibility, not the public shape.
	ExecutableName  string `yaml:"executable_name,omitempty"`
	ProductName     string `yaml:"product_name,omitempty"`
	NamespacePrefix string `yaml:"namespace_prefix,omitempty"`
}

// ProjectBrandBlock is brand: in config/zqk.yaml / config/zqk-local.yaml.
type ProjectBrandBlock struct {
	ExecutableName  string `yaml:"executable_name,omitempty"`
	ProductName     string `yaml:"product_name,omitempty"`
	NamespacePrefix string `yaml:"namespace_prefix,omitempty"`
}

// LoadedProject is the first brand block found in ProjectBrandRelPaths.
type LoadedProject struct {
	ExecutableName  string
	ProductName     string
	NamespacePrefix string
}

// LoadFromProject reads brand from config/zqk-local.yaml, then config/zqk.yaml.
func LoadFromProject(root string) LoadedProject {
	root = strings.TrimSpace(root)
	if root == emptyBrandValue {
		return LoadedProject{}
	}
	for _, rel := range ProjectBrandRelPaths() {
		data, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			continue
		}
		var cfg ProjectFile
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			continue
		}
		out := LoadedProject{}
		if b := cfg.Brand; b != nil {
			out.ExecutableName = strings.TrimSpace(b.ExecutableName)
			out.ProductName = strings.TrimSpace(b.ProductName)
			out.NamespacePrefix = strings.TrimSpace(b.NamespacePrefix)
		}
		if out.ExecutableName == emptyBrandValue {
			out.ExecutableName = strings.TrimSpace(cfg.ExecutableName)
		}
		if out.ProductName == emptyBrandValue {
			out.ProductName = strings.TrimSpace(cfg.ProductName)
		}
		if out.NamespacePrefix == emptyBrandValue {
			out.NamespacePrefix = strings.TrimSpace(cfg.NamespacePrefix)
		}
		if out.ExecutableName != emptyBrandValue || out.ProductName != emptyBrandValue || out.NamespacePrefix != emptyBrandValue {
			return out
		}
	}
	return LoadedProject{}
}
