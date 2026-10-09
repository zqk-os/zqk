package app

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/federation"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/testenvroot"
	"github.com/zqk-os/zqk/pkg/testkit"
)

type mockJoinTransport struct {
	response *federation.HandshakeResponse
	err      error
}

func (m *mockJoinTransport) SendHandshake(ctx context.Context, endpoint string, req federation.HandshakeRequest) (*federation.HandshakeResponse, error) {
	return m.response, m.err
}

func (m *mockJoinTransport) SendHeartbeat(ctx context.Context, endpoint string, kernelID string) error {
	return nil
}

func (m *mockJoinTransport) ExecuteTool(ctx context.Context, endpoint string, toolName string, arguments map[string]any) (json.RawMessage, error) {
	return nil, nil
}

func TestJoinCommand_Registration(t *testing.T) {
	env := testkit.PrepareIsolatedTempProject(t, nil)
	_ = testenvroot.CopyObjectSpecsFromProject(env.Root, "../../..")
	store := env.FileStorage
	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()

	peerID := "PEER-1"
	peerURL := "http://peer-1.mesh/mcp"

	peerObj := map[string]any{
		objects.FieldKeyKind:         objects.KindRemoteKernel,
		objects.FieldKeyTitle:        "Peer: " + peerID,
		objects.FieldKeyID:           "REM-NEXUS",
		objects.FieldKeyEndpoint:     peerURL,
		objects.FieldKeyPublicKey:    "MOCK-KEY",
		objects.FieldKeyCapabilities: []string{"storage"},
		objects.FieldKeyStatus:       objects.ObjectStatusImplemented,
		objects.FieldKeyTrustLevel:   "verified",
	}

	if err := store.Create(ctx, secCtx, peerObj); err != nil {
		t.Fatalf("Failed to create peer object: %v", err)
	}

	got, err := store.Read(ctx, secCtx, "REM-NEXUS")
	if err != nil {
		t.Fatalf("Failed to get peer object: %v", err)
	}

	if got[objects.FieldKeyTitle] != "Peer: PEER-1" {
		t.Errorf("Expected title 'Peer: PEER-1', got '%v'", got[objects.FieldKeyTitle])
	}
}

func TestJoinCommand_ExecutionAttempt(t *testing.T) {
	cmd := NewJoinCmd()
	cmd.SetArgs([]string{"http://127.0.0.1:65530/invalid"})
	err := cmd.Execute()
	if err == nil {
		t.Error("expected error attempting to join invalid peer URL, got nil")
	}
}

func TestJoinCommand_WithMockTransport_Declined(t *testing.T) {
	env := testkit.PrepareIsolatedTempProject(t, nil)
	_ = testenvroot.CopyObjectSpecsFromProject(env.Root, "../../..")
	origDir, _ := os.Getwd()
	require.NoError(t, os.Chdir(env.Root))
	defer func() { _ = os.Chdir(origDir) }()

	mock := &mockJoinTransport{
		response: &federation.HandshakeResponse{
			Accepted: false,
			Message:  "admission policy rejected handshake",
		},
	}
	federation.SetDefaultTransport(mock)
	defer federation.SetDefaultTransport(nil)

	cmd := NewJoinCmd()
	cmd.SetArgs([]string{"http://mock-peer.mesh/mcp"})
	err := cmd.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "peer declined handshake")
}

func TestJoinCommand_WithMockTransport_Accepted(t *testing.T) {
	env := testkit.PrepareIsolatedTempProject(t, nil)
	_ = testenvroot.CopyObjectSpecsFromProject(env.Root, "../../..")
	origDir, _ := os.Getwd()
	require.NoError(t, os.Chdir(env.Root))
	defer func() { _ = os.Chdir(origDir) }()

	mock := &mockJoinTransport{
		response: &federation.HandshakeResponse{
			Accepted:  true,
			KernelID:  "KER-REMOTE-999",
			PublicKey: "PUB-KEY-REMOTE",
			Capabilities: []federation.Capability{
				{Kind: "skill", ID: "storage", Name: "Remote Storage"},
			},
		},
	}
	federation.SetDefaultTransport(mock)
	defer federation.SetDefaultTransport(nil)

	// Execute with custom alias
	cmd := NewJoinCmd()
	cmd.SetArgs([]string{"http://mock-peer.mesh/mcp", "--alias", "remote-alpha"})
	err := cmd.Execute()
	assert.NoError(t, err)

	// Execute again to trigger update path (Upsert) without alias (auto-alias)
	cmd2 := NewJoinCmd()
	cmd2.SetArgs([]string{"http://mock-peer.mesh/mcp"})
	err = cmd2.Execute()
	assert.NoError(t, err)
}
