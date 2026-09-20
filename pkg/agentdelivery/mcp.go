package agentdelivery

import (
	"context"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/federation"
	"github.com/zqk-os/zqk/pkg/objects"
)

// MCPDeliverer delivers a prompt across the Sovereign Mesh to a remote kernel via MCP.
type MCPDeliverer struct {
	transport      federation.Transport
	remoteEndpoint string
	toolName       string
}

// NewMCPDeliverer creates a new deliverer that targets a remote kernel using an MCP transport.
func NewMCPDeliverer(transport federation.Transport, remoteEndpoint, toolName string) *MCPDeliverer {
	return &MCPDeliverer{
		transport:      transport,
		remoteEndpoint: remoteEndpoint,
		toolName:       toolName,
	}
}

func (m *MCPDeliverer) Name() string { return "mcp" }

func (m *MCPDeliverer) Deliver(ctx context.Context, p Prompt) (Result, error) {
	if m.transport == nil {
		return Result{}, errfmt.Errorf("mcp deliverer requires a transport")
	}

	args := map[string]any{
		"prompt":                  string(p.Markdown),
		objects.FieldKeySessionID: p.SessionID,
		objects.FieldKeyFormat:    p.Format,
	}

	_, err := m.transport.ExecuteTool(ctx, m.remoteEndpoint, m.toolName, args)
	if err != nil {
		return Result{}, errfmt.Newf("remote execution failed").Wrap(err)
	}

	return Result{
		DeliveredTo:    "mcp://" + m.remoteEndpoint + "/" + m.toolName,
		HTTPStatusCode: 200, // Success
	}, nil
}
