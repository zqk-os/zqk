package federation

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/lanceman/zqk/pkg/execwrap"
	"github.com/lanceman/zqk/pkg/zqkenv"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/objects"
)

// Transport defines the communication channel for federation protocols.
type Transport interface {
	// SendHandshake sends a handshake request to a remote endpoint.
	SendHandshake(ctx context.Context, endpoint string, req HandshakeRequest) (*HandshakeResponse, error)

	// SendHeartbeat sends a heartbeat signal to a remote endpoint.
	SendHeartbeat(ctx context.Context, endpoint string, kernelID string) error

	// ExecuteTool calls a specialized tool on a remote kernel using the MCP protocol.
	ExecuteTool(ctx context.Context, endpoint string, toolName string, arguments map[string]any) (json.RawMessage, error)
}

var defaultTransport Transport

// SetDefaultTransport allows application-level injection of the preferred mesh transport.
func SetDefaultTransport(t Transport) {
	defaultTransport = t
}

// GetDefaultTransport returns the configured transport, falling back to LocalCLITransport.
func GetDefaultTransport() Transport {
	if defaultTransport != nil {
		return defaultTransport
	}
	return NewLocalCLITransport("")
}

// LocalCLITransport implements Transport by executing the local 'zqk' binary.
// This is used for simulation, testing, and in-process kernel federation.
type LocalCLITransport struct {
	// BinaryPath is the path to the zqk binary. Defaults to zqkenv.Bin().Get().
	BinaryPath string
}

// NewLocalCLITransport creates a new LocalCLITransport.
func NewLocalCLITransport(binaryPath string) *LocalCLITransport {
	if binaryPath == "" {
		binaryPath = zqkenv.Bin().Get()
	}
	return &LocalCLITransport{
		BinaryPath: binaryPath,
	}
}

func (t *LocalCLITransport) SendHandshake(ctx context.Context, endpoint string, req HandshakeRequest) (*HandshakeResponse, error) {
	reqBytes, err := json.Marshal(req)
	if err != nil {
		return nil, errfmt.Newf("failed to marshal handshake request").Wrap(err)
	}

	// Simulation: Execute system federate handshake locally.
	// This proves the protocol works through a CLI boundary before moving to real TCP/HTTP.
	cmd := execwrap.CommandContext(ctx, t.BinaryPath, "system", "federate", "handshake", string(reqBytes), "--format", "json")
	cmd.Env = os.Environ()
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, errfmt.Newf("remote handshake simulation failed: %s\nOutput:\n%s", err, string(output)).Wrap(err)
	}

	var resp HandshakeResponse
	if err := json.Unmarshal(output, &resp); err != nil {
		return nil, errfmt.Newf("failed to parse simulated handshake response").Wrap(err)
	}

	return &resp, nil
}

func (t *LocalCLITransport) SendHeartbeat(ctx context.Context, endpoint string, kernelID string) error {
	// Simulation: No-op for now.
	return nil
}

func (t *LocalCLITransport) ExecuteTool(ctx context.Context, endpoint string, toolName string, arguments map[string]any) (json.RawMessage, error) {
	// Simulation: Execute zqk command locally based on tool name
	// This is a proof-of-concept for the Federated Execute model.

	// Map MCP tool names to CLI commands
	var cmdArgs []string
	switch toolName {
	case "object_list":
		kind, _ := arguments[objects.FieldKeyKind].(string)
		cmdArgs = []string{"object", "list", kind, "--format", "json"}
		// Map filters if any
		if filters, ok := arguments["filters"].(map[string]any); ok {
			for k, v := range filters {
				cmdArgs = append(cmdArgs, "--filter", fmt.Sprintf("%s=%v", k, v))
			}
		}
	case "object_read":
		id, _ := arguments[objects.FieldKeyID].(string)
		cmdArgs = []string{"object", "get", id, "--format", "json"}
	default:
		return nil, errfmt.Errorf("unsupported mesh tool: %s", toolName)
	}

	cmd := execwrap.CommandContext(ctx, t.BinaryPath, cmdArgs...)
	// In simulation, endpoint IS the project root path
	if endpoint != "" {
		cmd.Env = append(os.Environ(), zqkenv.ProjectRoot().Name()+"="+endpoint)
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, errfmt.Newf("remote tool execution simulation failed: %s (args: %v)", err, cmdArgs).Wrap(err)
	}

	// Wrap output in RawMessage to mimic MCP response
	return json.RawMessage(output), nil
}
