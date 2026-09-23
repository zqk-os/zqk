package mcp

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/paths"
)

// CPCPContractID defines the canonical CPCP contract identifier.
const CPCPContractID = "CPCP-MEMBRANE-001"

// CPCPInterceptor validates MCP tool calls against the CPCP Boundary Contract invariants.
type CPCPInterceptor struct {
	ContractID string
	FailClosed bool
}

// NewCPCPInterceptor returns a new interceptor enforcing CPCP-MEMBRANE-001.
func NewCPCPInterceptor() *CPCPInterceptor {
	return &CPCPInterceptor{
		ContractID: CPCPContractID,
		FailClosed: true,
	}
}

// ValidateToolCall checks if a tool call satisfies CPCP boundary invariants.
// Invariant 1: Direct file writes to .zqk/process/ bypass the cellular membrane and are strictly prohibited.
// Invariant 2: Missing or malformed parameters fail-closed.
// Invariant 3: Tampering with protected internal storage (.zqk/streams, .zqk/cas, .zqk/wal) is blocked.
func (ci *CPCPInterceptor) ValidateToolCall(ctx context.Context, params *ToolCallParams) error {
	if params == nil || strings.TrimSpace(params.Name) == "" {
		return errfmt.Errorf("CPCP-MEMBRANE-001 fail_closed: invalid or missing tool call parameters")
	}

	name := strings.ToLower(params.Name)

	// Check file mutation tools
	isWriteTool := strings.Contains(name, "write") ||
		strings.Contains(name, "create") ||
		strings.Contains(name, "edit") ||
		strings.Contains(name, "replace") ||
		strings.Contains(name, "delete") ||
		strings.Contains(name, "modify") ||
		strings.Contains(name, "patch")

	if !isWriteTool || params.Arguments == nil {
		return nil
	}

	// Scan arguments for target file paths
	for key, val := range params.Arguments {
		k := strings.ToLower(key)
		if k == "path" || k == "targetfile" || k == "target_file" || k == "filepath" || k == "file_path" || k == "filename" || k == "file" {
			if pathStr, ok := val.(string); ok {
				if err := ci.checkProtectedPath(pathStr); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func (ci *CPCPInterceptor) checkProtectedPath(p string) error {
	clean := filepath.Clean(filepath.ToSlash(p))

	protectedSegments := []string{
		paths.ProjectDataDir + "/process",
		paths.ProjectDataDir + "/streams",
		paths.ProjectDataDir + "/cas",
		paths.ProjectDataDir + "/wal",
	}

	for _, seg := range protectedSegments {
		if strings.Contains(clean, seg) {
			return errfmt.Errorf("CPCP-MEMBRANE-001 invariant violation: direct file mutation to protected kernel membrane path %q is prohibited; use native zqk intake/mutation commands instead", p)
		}
	}

	return nil
}
