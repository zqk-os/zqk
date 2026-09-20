package brand

import (
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"

	"gopkg.in/yaml.v3"
)

// CanonicalExecutableToken is the standard upstream binary name ("zqk").
const CanonicalExecutableToken = defaultExecutableNameValue

// LegacyMCPToolPrefix is the legacy MCP tool name prefix recognized for normalization.
// Tool names starting with this prefix are rewritten to use the current brand prefix
// (e.g. for backward compatibility with scenarios that used the old brand).
const LegacyMCPToolPrefix = "zqk_"

const (
	defaultProductNameValue     = "ZQK"
	defaultNamespacePrefixValue = "zqk"
	defaultExecutableNameValue  = "zqk"
	emptyBrandValue             = ""
	ZqkStableName               = "zqk-stable"
	ZqkStablePrefix             = "zqk-stable-"
)

var (
	executableName  atomic.Value // string
	productName     atomic.Value // string
	namespacePrefix atomic.Value // string
)

func init() {
	// Defaults should be safe and not require config/project root.
	SetExecutableName(defaultExecutableName())
	SetProductName(defaultProductNameValue)
	SetNamespacePrefix(defaultNamespacePrefixValue)
}

func defaultExecutableName() string {
	if len(os.Args) > 0 && os.Args[0] != emptyBrandValue {
		base := filepath.Base(os.Args[0])
		if base == ZqkStableName || strings.HasPrefix(base, ZqkStablePrefix) {
			return defaultExecutableNameValue
		}
		// go test binaries are named <pkg>.test (and often live under go-build/).
		// Never treat them as the product CLI name — that made MCP CLI bridging
		// resolve "mcp.test" as the zqk binary and fork-bomb (self-exec chains).
		// Similarly, "go run" produces a binary named "main" (or "main.exe").
		if isGoTestExecutableBase(base) || isGoRunExecutableBase(base) || strings.Contains(os.Args[0], "go-build") || strings.Contains(os.Args[0], "___go_build") {
			return defaultExecutableNameValue
		}
		return base
	}
	return defaultExecutableNameValue
}

func isGoRunExecutableBase(base string) bool {
	b := strings.ToLower(strings.TrimSpace(base))
	return b == "main" || b == "main.exe"
}

func isGoTestExecutableBase(base string) bool {
	b := strings.ToLower(strings.TrimSpace(base))
	return strings.HasSuffix(b, ".test")
}

// ExecutableName returns the current CLI executable name used in user-facing command examples.
// It is intended to be set during CLI initialization after config is loaded.
func ExecutableName() string {
	if v := executableName.Load(); v != nil {
		if s, ok := v.(string); ok && s != emptyBrandValue {
			return s
		}
	}
	return defaultExecutableName()
}

func SetExecutableName(name string) {
	if name == emptyBrandValue {
		return
	}
	if name == ZqkStableName || strings.HasPrefix(name, ZqkStablePrefix) {
		name = defaultExecutableNameValue
	}
	if isGoTestExecutableBase(name) || isGoRunExecutableBase(name) {
		name = defaultExecutableNameValue
	}
	executableName.Store(name)
}

// ProductName returns the current product name for branding (e.g., "ZQK", "AcmeOS").
func ProductName() string {
	if v := productName.Load(); v != nil {
		if s, ok := v.(string); ok && s != emptyBrandValue {
			return s
		}
	}
	return defaultProductNameValue
}

// SetProductName sets the product name for branding.
func SetProductName(name string) {
	if name == emptyBrandValue {
		return
	}
	productName.Store(name)
}

// NamespacePrefix returns the configured kernel namespace prefix (e.g., "zqk", "acme").
func NamespacePrefix() string {
	if v := namespacePrefix.Load(); v != nil {
		if s, ok := v.(string); ok && s != emptyBrandValue {
			return s
		}
	}
	return defaultNamespacePrefixValue
}

// SetNamespacePrefix sets the kernel namespace prefix.
// Channel binaries (zqk-stable) map to the product namespace so MCP tools
// stay zqk_* — not zqk-stable_*. Seat-workers that exec as zqk-stable would
// otherwise require mutation evidence the MCP child never advertises.
func SetNamespacePrefix(prefix string) {
	if prefix == emptyBrandValue {
		return
	}
	namespacePrefix.Store(ProductNamespacePrefix(prefix))
}

// ProductNamespacePrefix maps channel/role executable names to the kernel
// namespace (zqk-stable → zqk, mybrand-stable → mybrand). Same suffix list as
// env-prefix derivation so ZQK_* and zqk_* tools stay aligned.
func ProductNamespacePrefix(prefix string) string {
	stripped := stripEnvChannelSuffix(prefix)
	for _, suffix := range []string{"-admin-admin", "-admin"} {
		lower := strings.ToLower(strings.TrimSpace(stripped))
		if strings.HasSuffix(lower, suffix) {
			trimmed := strings.TrimSuffix(lower, suffix)
			if trimmed != emptyBrandValue {
				stripped = trimmed
			}
		}
	}
	if stripped == emptyBrandValue {
		return defaultNamespacePrefixValue
	}
	return stripped
}

// ProjectBrandConfig holds project-level brand overrides loaded from config files.
type ProjectBrandConfig struct {
	ExecutableName  string `yaml:"executable_name"`
	ProductName     string `yaml:"product_name"`
	NamespacePrefix string `yaml:"namespace_prefix"`
}

// LoadFromProject loads brand settings from config/zqk.yaml then config/zqk-local.yaml.
func LoadFromProject(root string) ProjectBrandConfig {
	cfg := ProjectBrandConfig{
		ExecutableName:  defaultExecutableNameValue,
		ProductName:     defaultProductNameValue,
		NamespacePrefix: defaultNamespacePrefixValue,
	}

	loadConfigFile := func(p string) {
		data, err := os.ReadFile(p)
		if err != nil {
			return
		}
		var parsed struct {
			Brand struct {
				ExecutableName  string `yaml:"executable_name"`
				ProductName     string `yaml:"product_name"`
				NamespacePrefix string `yaml:"namespace_prefix"`
			} `yaml:"brand"`
		}
		if err := yaml.Unmarshal(data, &parsed); err == nil {
			if parsed.Brand.ExecutableName != "" {
				cfg.ExecutableName = parsed.Brand.ExecutableName
			}
			if parsed.Brand.ProductName != "" {
				cfg.ProductName = parsed.Brand.ProductName
			}
			if parsed.Brand.NamespacePrefix != "" {
				cfg.NamespacePrefix = parsed.Brand.NamespacePrefix
			}
		}
	}

	loadConfigFile(filepath.Join(root, "config", "zqk.yaml"))
	loadConfigFile(filepath.Join(root, "config", "zqk-local.yaml"))
	return cfg
}

// ApplyCanonicalExecutable substitutes the canonical executable token with the custom name.
func ApplyCanonicalExecutable(content, exe string) string {
	if exe == "" || exe == CanonicalExecutableToken {
		return content
	}
	return strings.ReplaceAll(content, CanonicalExecutableToken, exe)
}
