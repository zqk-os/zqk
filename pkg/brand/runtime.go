package brand

import (
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
)

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
		return base
	}
	return defaultExecutableNameValue
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
func SetNamespacePrefix(prefix string) {
	if prefix == emptyBrandValue {
		return
	}
	namespacePrefix.Store(prefix)
}
