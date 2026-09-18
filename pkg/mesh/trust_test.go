package mesh_test

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"errors"
	"testing"

	"github.com/zqk-os/zqk/pkg/mesh"
)

type mockRegistry struct {
	kernels map[string]*mesh.RemoteKernel
}

func (m *mockRegistry) Verify(ctx context.Context, kernelID string) (*mesh.RemoteKernel, error) {
	if k, ok := m.kernels[kernelID]; ok {
		return k, nil
	}
	return nil, errors.New("not found")
}

func (m *mockRegistry) Register(ctx context.Context, kernel mesh.RemoteKernel) error {
	m.kernels[kernel.ID] = &kernel
	return nil
}

func TestTrustVerifier_VerifyState(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	pubHex := hex.EncodeToString(pub)

	reg := &mockRegistry{
		kernels: map[string]*mesh.RemoteKernel{
			"node-1": {
				ID:        "node-1",
				PublicKey: pubHex,
				Endpoint:  "http://node-1",
			},
		},
	}

	verifier := mesh.NewTrustVerifier(reg)

	payload := []byte("hive mind state update")
	sig := ed25519.Sign(priv, payload)
	sigHex := hex.EncodeToString(sig)

	t.Run("ValidSignature", func(t *testing.T) {
		state := mesh.SyncedState{
			NodeID:    "node-1",
			Payload:   payload,
			Signature: sigHex,
		}

		if err := verifier.VerifyState(context.Background(), state); err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("InvalidSignature", func(t *testing.T) {
		badSig := make([]byte, ed25519.SignatureSize)
		copy(badSig, sig)
		badSig[0] ^= 0xff // flip bits
		badSigHex := hex.EncodeToString(badSig)

		state := mesh.SyncedState{
			NodeID:    "node-1",
			Payload:   payload,
			Signature: badSigHex,
		}

		if err := verifier.VerifyState(context.Background(), state); !errors.Is(err, mesh.ErrInvalidSignature) {
			t.Errorf("expected ErrInvalidSignature, got %v", err)
		}
	})

	t.Run("UnknownNode", func(t *testing.T) {
		state := mesh.SyncedState{
			NodeID:    "unknown-node",
			Payload:   payload,
			Signature: sigHex,
		}

		if err := verifier.VerifyState(context.Background(), state); err == nil {
			t.Error("expected error for unknown node")
		}
	})

	t.Run("InvalidPubKey", func(t *testing.T) {
		reg.kernels["node-bad"] = &mesh.RemoteKernel{
			ID:        "node-bad",
			PublicKey: "not-a-hex-string",
		}
		state := mesh.SyncedState{
			NodeID:    "node-bad",
			Payload:   payload,
			Signature: sigHex,
		}

		if err := verifier.VerifyState(context.Background(), state); !errors.Is(err, mesh.ErrInvalidPublicKey) {
			t.Errorf("expected ErrInvalidPublicKey, got %v", err)
		}
	})
}
