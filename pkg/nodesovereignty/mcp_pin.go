package nodesovereignty

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"sync"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

// IDEPinAdapter specifies how an IDE adapter detects and inspects its host-global MCP configuration.
type IDEPinAdapter interface {
	Vendor() string
	IsGlobalConfig(projectRoot, configPath string) bool
	ReadPin(configPath string) string
	GlobalConfigPath() string
}

var (
	idePinAdaptersMu sync.RWMutex
	idePinAdapters   []IDEPinAdapter
)

// RegisterIDEPinAdapter registers an IDEPinAdapter for global MCP pin sovereignty checks.
func RegisterIDEPinAdapter(a IDEPinAdapter) {
	if a == nil {
		return
	}
	idePinAdaptersMu.Lock()
	defer idePinAdaptersMu.Unlock()
	for _, existing := range idePinAdapters {
		if existing.Vendor() == a.Vendor() {
			return
		}
	}
	idePinAdapters = append(idePinAdapters, a)
}

// IsGlobalMCPConfig reports whether configPath is a host-global MCP configuration
// pin rather than a project workspace configuration.
// If an adapter is provided, detection is delegated to that adapter; otherwise
// registered adapters are consulted, falling back to generic path heuristics.
func IsGlobalMCPConfig(projectRoot, configPath string, adapter ...IDEPinAdapter) bool {
	clean := filepath.Clean(strings.TrimSpace(configPath))
	if clean == "" || clean == "." {
		return false
	}
	if len(adapter) > 0 && adapter[0] != nil {
		return adapter[0].IsGlobalConfig(projectRoot, clean)
	}
	idePinAdaptersMu.RLock()
	for _, a := range idePinAdapters {
		if a.IsGlobalConfig(projectRoot, clean) {
			idePinAdaptersMu.RUnlock()
			return true
		}
	}
	idePinAdaptersMu.RUnlock()

	if projectRoot != "" {
		root := filepath.Clean(strings.TrimSpace(projectRoot))
		rel, err := filepath.Rel(root, clean)
		if err == nil && !strings.HasPrefix(rel, "..") && rel != "." {
			return false
		}
	}
	return filepath.Base(clean) == "mcp.json"
}

// ReadGlobalMCPPin extracts ZQK_PROJECT_ROOT (or cwd that looks like a kernel root)
// from an MCP JSON configuration document. Empty when absent/unparseable.
func ReadGlobalMCPPin(configPath string, adapter ...IDEPinAdapter) string {
	if len(adapter) > 0 && adapter[0] != nil {
		return adapter[0].ReadPin(configPath)
	}
	b, err := fileutil.ReadFile(configPath)
	if err != nil {
		return ""
	}
	var doc map[string]any
	if err := json.Unmarshal(b, &doc); err != nil {
		return ""
	}
	servers, _ := doc["mcpServers"].(map[string]any)
	if servers == nil {
		return ""
	}
	for _, raw := range servers {
		cfg, _ := raw.(map[string]any)
		if cfg == nil {
			continue
		}
		if env, ok := cfg["env"].(map[string]any); ok {
			for k, v := range env {
				if strings.EqualFold(k, "ZQK_PROJECT_ROOT") || strings.EqualFold(k, zqkenv.ProjectRoot().Name()) {
					if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
						return strings.TrimSpace(s)
					}
				}
			}
		}
		if cwd, ok := cfg["cwd"].(string); ok && looksLikeZqkRoot(cwd) {
			return strings.TrimSpace(cwd)
		}
	}
	return ""
}

// RefuseGlobalMCPPinRewrite fails closed when InstallToIDE would rewrite a
// host-global MCP configuration away from an existing seated pin (studio /
// another kernel) without human opt-in or a recent intentional node_hop.
func RefuseGlobalMCPPinRewrite(projectRoot, configPath string, adapter ...IDEPinAdapter) error {
	var activeAdapter IDEPinAdapter
	if len(adapter) > 0 && adapter[0] != nil {
		activeAdapter = adapter[0]
	} else {
		idePinAdaptersMu.RLock()
		for _, a := range idePinAdapters {
			if a.IsGlobalConfig(projectRoot, configPath) {
				activeAdapter = a
				break
			}
		}
		idePinAdaptersMu.RUnlock()
	}

	if activeAdapter != nil {
		if !activeAdapter.IsGlobalConfig(projectRoot, configPath) {
			return nil
		}
	} else if !IsGlobalMCPConfig(projectRoot, configPath) {
		return nil
	}

	if zqkenv.AllowGlobalMCPPinRewrite().Get() == "1" {
		return nil
	}

	var existing string
	if activeAdapter != nil {
		existing = strings.TrimSpace(activeAdapter.ReadPin(configPath))
	} else {
		existing = strings.TrimSpace(ReadGlobalMCPPin(configPath))
	}
	if existing == "" {
		return nil
	}
	existAbs, err := filepath.Abs(existing)
	if err != nil {
		return nil
	}
	existAbs = filepath.Clean(existAbs)
	want := strings.TrimSpace(projectRoot)
	if want == "" {
		return errfmt.Errorf("node sovereignty: refuse global mcp.json rewrite with empty project root (pin=%s)", existAbs)
	}
	wantAbs, err := filepath.Abs(want)
	if err != nil {
		return errfmt.Newf("node sovereignty: abs project root for mcp pin").Wrap(err)
	}
	wantAbs = filepath.Clean(wantAbs)
	if existAbs == wantAbs {
		return nil
	}
	if HasRecentHop(wantAbs, 0) || HasRecentHop(existAbs, 0) {
		return nil
	}
	hint := paths.RewriteCanonicalCLIInvocations(
		"set " + zqkenv.AllowGlobalMCPPinRewrite().Name() + "=1 (human opt-in) or record node_hop / " + paths.CLIUsage("use"),
	)
	return errfmt.Errorf(
		"node sovereignty: refuse global mcp.json rewrite away from seated pin %s toward %s (%s)",
		existAbs, wantAbs, hint,
	)
}
