package nodesovereignty

import (
	"encoding/json"
	"path/filepath"
	"strings"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

// IsGlobalCursorMCPConfig reports whether configPath is the host-global
// ~/.cursor/mcp.json pin (not a project workspace .cursor/mcp.json).
func IsGlobalCursorMCPConfig(projectRoot, configPath string) bool {
	clean := filepath.Clean(strings.TrimSpace(configPath))
	if clean == "" || clean == "." {
		return false
	}
	if filepath.Base(clean) != "mcp.json" || filepath.Base(filepath.Dir(clean)) != ".cursor" {
		return false
	}
	if projectRoot != "" {
		ws := filepath.Clean(filepath.Join(projectRoot, ".cursor", "mcp.json"))
		if clean == ws {
			return false
		}
	}
	return true
}

// ReadCursorMCPPin extracts ZQK_PROJECT_ROOT (or cwd that looks like a kernel root)
// from a Cursor mcp.json document. Empty when absent/unparseable.
func ReadCursorMCPPin(configPath string) string {
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

// RefuseGlobalMCPPinRewrite fails closed when InstallToIDE would rewrite the
// host-global ~/.cursor/mcp.json away from an existing seated pin (studio /
// another kernel) without human opt-in or a recent intentional node_hop.
//
// TRACK: BLI-1794075314772743000-369a713f / BLI-1794029815318591000-4655e144
func RefuseGlobalMCPPinRewrite(projectRoot, configPath string) error {
	if !IsGlobalCursorMCPConfig(projectRoot, configPath) {
		return nil
	}
	if zqkenv.AllowGlobalMCPPinRewrite().Get() == "1" {
		return nil
	}
	existing := strings.TrimSpace(ReadCursorMCPPin(configPath))
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
