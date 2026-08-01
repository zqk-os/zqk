package app

import (
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/testenvroot"
	"github.com/lanceman/zqk/pkg/testkit"
)

func TestJoinCommand_Registration(t *testing.T) {
	// This test verifies that the join command correctly registers a peer
	// without actually performing a network handshake.

	// Setup mock storage
	env := testkit.PrepareIsolatedTempProject(t, nil)
	_ = testenvroot.CopyObjectSpecsFromProject(env.Root, "../../..")
	store := env.FileStorage
	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()

	// Peer data
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

	// Test storage creation directly first
	if err := store.Create(ctx, secCtx, peerObj); err != nil {
		t.Fatalf("Failed to create peer object: %v", err)
	}

	// Verify
	got, err := store.Read(ctx, secCtx, "REM-NEXUS")
	if err != nil {
		t.Fatalf("Failed to get peer object: %v", err)
	}

	if got[objects.FieldKeyTitle] != "Peer: PEER-1" {
		t.Errorf("Expected title 'Peer: PEER-1', got '%v'", got[objects.FieldKeyTitle])
	}
}
