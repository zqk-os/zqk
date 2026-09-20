package logging

import (
	"context"
	"io"
	"os"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

// GetCommandOutputWriter returns the io.Writer to use for CLI command result output
// (JSON/YAML/table). Compliant with POL-CODE-007: application code must not call
// os.Stdout.Write/os.Stderr.Write directly; this package is the single place that
// selects the appropriate writer. Returns:
//   - context override when ctx has WithCommandOutputWriter (e.g. tests)
//   - io.Discard when MCP server is actively serving (stdout reserved for JSON-RPC)
//   - os.Stdout when in MCP subprocess mode (ZQK_MCP_ACCOUNT_ID set) so the parent
//     MCP server can capture the command result on stdout; logs go to stderr via the
//     logging framework
//   - os.Stdout for normal CLI usage
func GetCommandOutputWriter(ctx context.Context) io.Writer {
	if w, ok := pkgctx.GetCommandOutputWriterFromContext(ctx); ok && w != nil {
		return w
	}
	if pkgctx.GetMCPServerContext().IsServing() {
		return io.Discard
	}
	// MCP subprocess: parent captures stdout for the JSON result; use stdout so
	// parseCommandOutput in the bridge sees the result (avoids empty_output errors).
	if zqkenv.MCPAccountID().Get() != emptyValue {
		return os.Stdout
	}
	return os.Stdout
}
