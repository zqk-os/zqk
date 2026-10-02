package mcp

import (
	"path/filepath"

	"github.com/zqk-os/zqk/pkg/brand"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// MCP role names used in process display symlinks (bin/<brand>-mcp-<role>).
const (
	MCPRoleProxy      = "proxy"
	MCPRoleDaemon     = "daemon"
	MCPRoleIDEAdapter = "ide-adapter"
)

// MCPRoleProcessName returns the process/display basename for an MCP role
// (e.g. "zqk-mcp-proxy").
func MCPRoleProcessName(role string) string {
	return brand.ExecutableName() + "-mcp-" + role
}

// MCPRoleBinPath returns bin/<brand>-mcp-<role> under projectRoot.
func MCPRoleBinPath(projectRoot, role string) string {
	binDir := paths.ResolvePathFromCacheOrConstant(projectRoot, paths.PathAliasRepoBin, paths.RepoBinDir)
	if !filepath.IsAbs(binDir) {
		binDir = filepath.Join(projectRoot, binDir)
	}
	return filepath.Join(binDir, MCPRoleProcessName(role))
}

// IDEMCPRoles are role symlinks IDE/IDE mcp.json may point at. Rebuilds
// that only refresh zqk-mcp-daemon leave ide-adapter missing → red MCP.
// keep ensure + stable install in sync.
var IDEMCPRoles = []string{MCPRoleDaemon, MCPRoleIDEAdapter}

// EnsureMCPIDERoleSymlinks refreshes daemon + ide-adapter role links against
// targetBin. Best-effort: returns the first error after attempting all roles.
func EnsureMCPIDERoleSymlinks(projectRoot, targetBin string) error {
	var first error
	for _, role := range IDEMCPRoles {
		if _, err := EnsureMCPRoleSymlink(projectRoot, role, targetBin); err != nil && first == nil {
			first = err
		}
	}
	return first
}

// EnsureMCPRoleSymlink creates or updates a role symlink so exec/ps show the
// role name (zqk-mcp-proxy / zqk-mcp-daemon) instead of bare "zqk".
// targetBin must be an existing regular file (or symlink to one). Returns the
// absolute path of the role symlink.
func EnsureMCPRoleSymlink(projectRoot, role, targetBin string) (string, error) {
	if projectRoot == "" {
		return "", errfmt.Errorf("project root required for MCP role symlink")
	}
	if role != MCPRoleProxy && role != MCPRoleDaemon && role != MCPRoleIDEAdapter {
		return "", errfmt.Errorf("unknown MCP role %q", role)
	}
	absTarget, err := filepath.Abs(targetBin)
	if err != nil {
		return "", errfmt.Newf("resolve MCP role target").Wrap(err)
	}
	if resolved, err := filepath.EvalSymlinks(absTarget); err == nil {
		absTarget = resolved
	}
	if !fileutil.IsRegularFile(absTarget) {
		return "", errfmt.Errorf("MCP role symlink target is not a regular file: %s", absTarget)
	}

	linkPath := MCPRoleBinPath(projectRoot, role)
	if fi, err := fileutil.Lstat(linkPath); err == nil {
		if fi.Mode()&fileutil.ModeSymlink == 0 {
			return "", errfmt.Errorf("MCP role path exists and is not a symlink: %s", linkPath)
		}
	}

	absLink, err := fileutil.EnsureSymlink(linkPath, absTarget)
	if err != nil {
		return "", errfmt.Newf("create MCP role symlink %s -> %s", linkPath, absTarget).Wrap(err)
	}
	return absLink, nil
}
