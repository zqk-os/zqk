package ambient

import (
	"context"
	"fmt"
)

// Transceiver extends the local watcher to emit .csnap envelopes to the swarm stream.
type Transceiver struct {
	inbox   InboxPusher
	builder *CSnapBuilder
}

// NewTransceiver initializes a new Transceiver.
func NewTransceiver(inbox InboxPusher) *Transceiver {
	return &Transceiver{
		inbox:   inbox,
		builder: NewCSnapBuilder(),
	}
}

// EmitEnvelope synthesizes a .csnap (Compressed Snapshot) envelope and pushes it to the inbox.
func (t *Transceiver) EmitEnvelope(ctx context.Context, filePath string, fileContent []byte) error {
	payload, err := t.builder.BuildEnvelope(filePath, fileContent)
	if err != nil {
		return fmt.Errorf("failed to build csnap envelope: %w", err)
	}

	event := ContextEvent{
		ID:      "csnap-temp-id",
		Type:    "csnap",
		Payload: payload,
	}
	return t.inbox.Push(ctx, event)
}
