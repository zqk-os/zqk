package hivemind

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/lanceman/zqk/pkg/logging"

	"github.com/lanceman/zqk/pkg/infrastructure"
	"github.com/lanceman/zqk/pkg/infrastructure/crypto"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/zqktime"
)

// HiveEvent represents a signed, high-level signal of collective intelligence.
type HiveEvent struct {
	Timestamp string         `json:"timestamp"`
	Source    string         `json:"source"`
	Category  string         `json:"category"` // e.g., "fission", "economy", "specialization", "transfer"
	Message   string         `json:"message"`
	Metadata  map[string]any `json:"metadata"`
	Signature string         `json:"signature,omitempty"`
	PublicKey string         `json:"public_key,omitempty"`
}

// ActivityLog is the 'Black Box' for the Hive Mind.
type ActivityLog struct {
	spine  infrastructure.SpinalSpine
	signer crypto.Signer
	mu     sync.Mutex
	logs   []HiveEvent
}

func NewActivityLog(spine infrastructure.SpinalSpine, signer crypto.Signer) *ActivityLog {
	return &ActivityLog{
		spine:  spine,
		signer: signer,
	}
}

// Pulse broadcasts a signed, high-fidelity event to the hive and persists it locally.
func (l *ActivityLog) Pulse(ctx context.Context, source, category, message string, metadata map[string]any) {
	event := HiveEvent{
		Timestamp: zqktime.NowRFC3339UTC(),
		Source:    source,
		Category:  category,
		Message:   message,
		Metadata:  metadata,
	}

	// Sign the event if a signer is available
	if l.signer != nil {
		data, _ := json.Marshal(event)
		sig, _ := l.signer.Sign(data)
		event.Signature = sig
		event.PublicKey = l.signer.PublicKey()
	}

	l.mu.Lock()
	l.logs = append(l.logs, event)
	if len(l.logs) > 1000 {
		l.logs = l.logs[1:] // Rolling buffer
	}
	l.mu.Unlock()

	// Convert to infrastructure.Event for the Industrial Spine
	spineEvent := infrastructure.Event{
		ObjectID: fmt.Sprintf("HIVE-EVT-%d", time.Now().UnixNano()),
		Kind:     "hive_activity",
		Op:       "pulse",
		Payload: map[string]any{
			objects.FieldKeySource:    source,
			objects.FieldKeyCategory:  category,
			"message":                 message,
			objects.FieldKeyMetadata:  metadata,
			objects.FieldKeySignature: event.Signature,
			objects.FieldKeyPublicKey: event.PublicKey,
		},
	}

	_ = l.spine.Publish(ctx, spineEvent)

	sigStatus := "unsigned"
	if event.Signature != "" {
		sigStatus = "signed:" + event.Signature[:8]
	}
	logging.FluentEvent(logging.GetLogger()).Info(fmt.Sprintf("⚡ [HIVE PULSE] [%s] [%s] %s: %s\n", category, sigStatus, source, message)).Log()
}

// GetRecent returns the last N events.
func (l *ActivityLog) GetRecent(n int) []HiveEvent {
	l.mu.Lock()
	defer l.mu.Unlock()
	if n > len(l.logs) {
		n = len(l.logs)
	}
	return l.logs[len(l.logs)-n:]
}
