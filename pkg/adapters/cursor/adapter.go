package cursor

import (
	"path/filepath"
	"strings"

	"github.com/zqk-os/zqk/pkg/nodesovereignty"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

const (
	// VendorID is the canonical identifier for the Cursor adapter.
	VendorID = "cursor"

	// ConfigDir is the Cursor configuration directory relative to home or workspace.
	ConfigDir = ".cursor"

	// ConfigFile is the Cursor MCP configuration filename.
	ConfigFile = "mcp.json"
)

// Adapter implements nodesovereignty.IDEPinAdapter for Cursor IDE.
type Adapter struct{}

var _ nodesovereignty.IDEPinAdapter = (*Adapter)(nil)

// New returns a new Cursor adapter.
func New() *Adapter {
	return &Adapter{}
}

// DefaultAdapter is the singleton Cursor adapter instance.
var DefaultAdapter = &Adapter{}

func init() {
	nodesovereignty.RegisterIDEPinAdapter(DefaultAdapter)
}

// Vendor returns the vendor identifier "cursor".
func (a *Adapter) Vendor() string {
	return VendorID
}

// IsGlobalConfig reports whether configPath is the Cursor host-global
// ~/.cursor/mcp.json (and not a workspace .cursor/mcp.json).
func (a *Adapter) IsGlobalConfig(projectRoot, configPath string) bool {
	return IsGlobalMCPConfig(projectRoot, configPath)
}

// ReadPin extracts the pinned project root from Cursor's mcp.json.
func (a *Adapter) ReadPin(configPath string) string {
	return ReadMCPPin(configPath)
}

// GlobalConfigPath returns the canonical path to the user's host-global Cursor mcp.json.
func (a *Adapter) GlobalConfigPath() string {
	home, err := fileutil.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ConfigDir, ConfigFile)
}

// IsGlobalMCPConfig reports whether configPath is the Cursor host-global
// ~/.cursor/mcp.json pin (not a project workspace .cursor/mcp.json).
func IsGlobalMCPConfig(projectRoot, configPath string) bool {
	clean := filepath.Clean(strings.TrimSpace(configPath))
	if clean == "" || clean == "." {
		return false
	}
	if filepath.Base(clean) != ConfigFile || filepath.Base(filepath.Dir(clean)) != ConfigDir {
		return false
	}
	if projectRoot != "" {
		ws := filepath.Clean(filepath.Join(projectRoot, ConfigDir, ConfigFile))
		if clean == ws {
			return false
		}
	}
	return true
}

// ReadMCPPin extracts ZQK_PROJECT_ROOT (or cwd that looks like a kernel root)
// from a Cursor mcp.json document.
func ReadMCPPin(configPath string) string {
	return nodesovereignty.ReadGlobalMCPPin(configPath)
}

// RefuseGlobalMCPPinRewrite fails closed when rewriting Cursor's host-global
// mcp.json away from an existing seated pin without human opt-in or recent node_hop.
func RefuseGlobalMCPPinRewrite(projectRoot, configPath string) error {
	return nodesovereignty.RefuseGlobalMCPPinRewrite(projectRoot, configPath, DefaultAdapter)
}
