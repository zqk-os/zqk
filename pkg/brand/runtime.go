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
		if len(base) > 4 && strings.EqualFold(base[len(base)-4:], ".exe") {
			base = base[:len(base)-4]
		}
		if base == ZqkStableName || strings.HasPrefix(base, ZqkStablePrefix) ||
			base == "zqk-pw" || base == "zqk-amb" || base == "zqk-sched" || base == "zqk-overseer" {
			return defaultExecutableNameValue
		}
		// go test binaries are named <pkg>.test (and often live under go-build/).
		// Never treat them as the product CLI name — that made MCP CLI bridging
		// resolve "mcp.test" as the zqk binary and fork-bomb (self-exec chains).
		// Similarly, "go run" produces a binary named "main" (or "main.exe").
		if isGoTestExecutableBase(base) || isGoRunExecutableBase(base) || strings.Contains(os.Args[0], "go-build") || strings.Contains(os.Args[0], "___go_build") {
			return defaultExecutableNameValue
		}
		// If base has a role/channel suffix (e.g. foo-sched, foo-amb, foo-pw, foo-overseer),
		// strip it to get the core brand executable name.
		stripped := stripEnvChannelSuffix(base)
		if stripped != base && stripped != emptyBrandValue {
			return stripped
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

// StableExecutableName returns the stable channel binary name based on the current brand executable.
// Defaults to "zqk-stable", or "zcom" if brand is "zcom", or "<brand>-stable".
func StableExecutableName() string {
	exe := ExecutableName()
	if exe == "zcom" {
		return "zcom"
	}
	if exe == emptyBrandValue || exe == defaultExecutableNameValue {
		return ZqkStableName
	}
	return exe + "-stable"
}

// StablePrefix returns the prefix used by channel binaries for the brand (e.g. "zqk-stable-" or "<brand>-stable-").
func StablePrefix() string {
	exe := ExecutableName()
	if exe == "zcom" {
		return "zcom-"
	}
	if exe == emptyBrandValue || exe == defaultExecutableNameValue {
		return ZqkStablePrefix
	}
	return exe + "-stable-"
}

// MCPExecutableName returns the binary name for the MCP standalone server (e.g. "zqk-mcp" or "<brand>-mcp").
func MCPExecutableName() string {
	exe := ExecutableName()
	if exe == emptyBrandValue || exe == defaultExecutableNameValue {
		return "zqk-mcp"
	}
	return exe + "-mcp"
}

// MCPDaemonExecutableName returns the binary name for the MCP daemon role (e.g. "zqk-mcp-daemon" or "<brand>-mcp-daemon").
func MCPDaemonExecutableName() string {
	exe := ExecutableName()
	if exe == emptyBrandValue || exe == defaultExecutableNameValue {
		return "zqk-mcp-daemon"
	}
	return exe + "-mcp-daemon"
}

// MCPIdeAdapterExecutableName returns the binary name for the MCP IDE adapter (e.g. "zqk-mcp-ide-adapter" or "<brand>-mcp-ide-adapter").
func MCPIdeAdapterExecutableName() string {
	exe := ExecutableName()
	if exe == emptyBrandValue || exe == defaultExecutableNameValue {
		return "zqk-mcp-ide-adapter"
	}
	return exe + "-mcp-ide-adapter"
}

// SchedExecutableName returns the binary name for the scheduler daemon (e.g. "zqk-sched" or "<brand>-sched").
func SchedExecutableName() string {
	exe := ExecutableName()
	if exe == emptyBrandValue || exe == defaultExecutableNameValue {
		return "zqk-sched"
	}
	return exe + "-sched"
}

// AmbExecutableName returns the binary name for the ambient daemon (e.g. "zqk-amb" or "<brand>-amb").
func AmbExecutableName() string {
	exe := ExecutableName()
	if exe == emptyBrandValue || exe == defaultExecutableNameValue {
		return "zqk-amb"
	}
	return exe + "-amb"
}

// PWExecutableName returns the binary name for the privileged writer daemon (e.g. "zqk-pw" or "<brand>-pw").
func PWExecutableName() string {
	exe := ExecutableName()
	if exe == emptyBrandValue || exe == defaultExecutableNameValue {
		return "zqk-pw"
	}
	return exe + "-pw"
}

// OverseerExecutableName returns the binary name for the process group overseer daemon (e.g. "zqk-overseer" or "<brand>-overseer").
func OverseerExecutableName() string {
	exe := ExecutableName()
	if exe == emptyBrandValue || exe == defaultExecutableNameValue {
		return "zqk-overseer"
	}
	return exe + "-overseer"
}

// RoleDifferentiatorNames returns the candidate binary basenames for a daemon role under the current brand.
// For example, role "sched" -> ["foo-sched", "foo-scheduler", "zqk-sched", "zqk-scheduler"].
func RoleDifferentiatorNames(role string) []string {
	exe := ExecutableName()
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "sched", "scheduler", "scheduler-daemon":
		names := []string{exe + "-sched", exe + "-scheduler"}
		if exe != defaultExecutableNameValue {
			names = append(names, "zqk-sched", "zqk-scheduler")
		}
		return names
	case "amb", "ambient", "ambient-daemon":
		names := []string{exe + "-amb", exe + "-ambient"}
		if exe != defaultExecutableNameValue {
			names = append(names, "zqk-amb", "zqk-ambient")
		}
		return names
	case "pw", "privileged-writer", "writer":
		names := []string{exe + "-pw", exe + "-privileged-writer"}
		if exe != defaultExecutableNameValue {
			names = append(names, "zqk-pw", "zqk-privileged-writer")
		}
		return names
	case "overseer":
		names := []string{exe + "-overseer"}
		if exe != defaultExecutableNameValue {
			names = append(names, "zqk-overseer")
		}
		return names
	case "mcp", "mcp-daemon":
		names := []string{exe + "-mcp-daemon", exe + "-mcp"}
		if exe != defaultExecutableNameValue {
			names = append(names, "zqk-mcp-daemon", "zqk-mcp")
		}
		return names
	case "ide-adapter", "mcp-ide-adapter":
		names := []string{exe + "-mcp-ide-adapter"}
		if exe != defaultExecutableNameValue {
			names = append(names, "zqk-mcp-ide-adapter")
		}
		return names
	default:
		r := strings.ToLower(strings.TrimSpace(role))
		if r == "" {
			return nil
		}
		names := []string{exe + "-" + r}
		if exe != defaultExecutableNameValue {
			names = append(names, "zqk-"+r)
		}
		return names
	}
}

// IsProductExecutable returns true if base matches any executable name belonging to the branded ecosystem.
func IsProductExecutable(name string) bool {
	base := filepath.Base(name)
	if len(base) > 4 && strings.EqualFold(base[len(base)-4:], ".exe") {
		base = base[:len(base)-4]
	}
	curExe := ExecutableName()
	curStable := StableExecutableName()
	curMCP := MCPExecutableName()
	curDaemon := MCPDaemonExecutableName()
	curAdapter := MCPIdeAdapterExecutableName()
	curSched := SchedExecutableName()
	curAmb := AmbExecutableName()
	curPW := PWExecutableName()
	curOverseer := OverseerExecutableName()

	if base == curExe || base == curStable || base == curMCP || base == curDaemon || base == curAdapter ||
		base == curSched || base == curAmb || base == curPW || base == curOverseer {
		return true
	}
	// Always recognize canonical upstream names and role aliases
	switch base {
	case "zqk", "zqk-stable", "zqk-mcp", "zqk-mcp-daemon", "zqk-mcp-ide-adapter", "zcom",
		"zqk-sched", "zqk-scheduler", "zqk-amb", "zqk-ambient", "zqk-pw", "zqk-privileged-writer", "zqk-overseer":
		return true
	}
	if strings.HasPrefix(base, curStable+"-") || strings.HasPrefix(base, ZqkStablePrefix) {
		return true
	}
	// Dynamic brand with role suffixes: e.g. foo-sched, foo-amb, foo-pw, foo-overseer
	trimmed := stripEnvChannelSuffix(base)
	if trimmed == curExe || trimmed == defaultExecutableNameValue || trimmed == "zcom" {
		return true
	}
	return false
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
