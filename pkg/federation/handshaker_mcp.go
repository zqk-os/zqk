package federation

import (
	"context"

	"github.com/zqk-os/zqk/pkg/errfmt"
)

// MCPHandshaker implements the Handshaker interface using the MCP protocol as transport.
type MCPHandshaker struct {
	LocalKernelID  string
	LocalPublicKey string
	Transport      Transport
}

// NewMCPHandshaker creates a new MCPHandshaker.
func NewMCPHandshaker(kernelID, publicKey string, transport Transport) *MCPHandshaker {
	if transport == nil {
		transport = GetDefaultTransport()
	}
	return &MCPHandshaker{
		LocalKernelID:  kernelID,
		LocalPublicKey: publicKey,
		Transport:      transport,
	}
}

// Initiate starts a handshake with a remote kernel using the configured transport.
func (h *MCPHandshaker) Initiate(ctx context.Context, remoteEndpoint string, req HandshakeRequest) (*HandshakeResponse, error) {
	// Ensure local identity is set in the request
	req.KernelID = h.LocalKernelID
	req.PublicKey = h.LocalPublicKey

	return h.Transport.SendHandshake(ctx, remoteEndpoint, req)
}

// Accept processes an incoming handshake request.
func (h *MCPHandshaker) Accept(ctx context.Context, req HandshakeRequest) (*HandshakeResponse, error) {
	// Logic to decide if we accept the handshake (e.g. policy check)
	// For now, this is still a placeholder handled by the CLI command.
	return nil, errfmt.Errorf("use CLI command for Accept")
}
