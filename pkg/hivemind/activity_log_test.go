package hivemind_test

import (
	"context"
	"testing"

	"github.com/zqk-os/zqk/pkg/hivemind"
	"github.com/zqk-os/zqk/pkg/infrastructure"
	"github.com/zqk-os/zqk/pkg/infrastructure/crypto"
	"github.com/zqk-os/zqk/pkg/objects"
)

type mockSpine struct {
	infrastructure.SpinalSpine
	published []infrastructure.Event
}

func (m *mockSpine) Publish(_ context.Context, event infrastructure.Event) error {
	m.published = append(m.published, event)
	return nil
}

func TestActivityLog_SignedPulse(t *testing.T) {
	ctx := context.Background()
	spine := &mockSpine{}
	signer, _ := crypto.GenerateKeypair()

	log := hivemind.NewActivityLog(spine, signer)

	log.Pulse(ctx, "test-source", "test-category", "test message", nil)

	if len(spine.published) != 1 {
		t.Fatalf("expected 1 published event, got %d", len(spine.published))
	}

	event := spine.published[0]
	sig := event.Payload[objects.FieldKeySignature].(string)
	pub := event.Payload[objects.FieldKeyPublicKey].(string)

	if sig == "" {
		t.Errorf("expected signature to be populated")
	}

	if pub != signer.PublicKey() {
		t.Errorf("expected public key %s, got %s", signer.PublicKey(), pub)
	}
}
