package ambience

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/lanceman/zqk/pkg/concurrency"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
)

type EventType string

const (
	EventFileModified EventType = "file_modified"
	EventFocusChanged EventType = "focus_changed"
	EventTestFailed   EventType = "test_failed"
)

type AmbientEvent struct {
	ID        string
	Type      EventType
	URI       string
	Timestamp int64
	Payload   []byte
}

type EventMesh interface {
	Publish(ctx context.Context, event AmbientEvent) error
	Subscribe(ctx context.Context, types []EventType) (<-chan AmbientEvent, error)
}

// InMemoryEventMesh is a simple in-memory implementation of EventMesh.
type InMemoryEventMesh struct {
	mu          sync.RWMutex
	subscribers map[EventType][]chan AmbientEvent
	projectRoot string
	secCtx      *pkgctx.SecurityContext
}

// NewInMemoryEventMesh creates a new InMemoryEventMesh.
func NewInMemoryEventMesh() *InMemoryEventMesh {
	return &InMemoryEventMesh{
		subscribers: make(map[EventType][]chan AmbientEvent),
	}
}

// EnableEventSourcing configures the mesh to emit audit events.
func (m *InMemoryEventMesh) EnableEventSourcing(projectRoot string, secCtx *pkgctx.SecurityContext) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.projectRoot = projectRoot
	m.secCtx = secCtx
}

// Publish broadcasts an event to all subscribers interested in its type.
func (m *InMemoryEventMesh) Publish(ctx context.Context, event AmbientEvent) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	var channels []chan AmbientEvent
	_ = concurrency.RunInRLock(&m.mu, func() error {
		subs, ok := m.subscribers[event.Type]
		if !ok {
			return nil
		}
		channels = make([]chan AmbientEvent, len(subs))
		copy(channels, subs)
		return nil
	})

	for _, ch := range channels {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case ch <- event:
		}
	}

	var root string
	var sCtx *pkgctx.SecurityContext
	_ = concurrency.RunInRLock(&m.mu, func() error {
		root = m.projectRoot
		sCtx = m.secCtx
		return nil
	})

	if root != "" && sCtx != nil {
		goroutinelabels.NewGoroutine("ambient_audit", "emit audit for ambient event").StartSimple(func() {
			targetKind := objects.KindAuditEvent
			targetURI := event.URI

			if event.URI != "" {
				if strings.HasSuffix(event.URI, ".md") {
					targetKind = objects.KindDocEntry
				} else if strings.HasSuffix(event.URI, ".yaml") || strings.HasSuffix(event.URI, ".json") {
					targetKind = objects.KindTechnicalSpec
				}
			}

			// Parse JSON payload or use raw if unparseable
			var payloadMap map[string]any
			if err := json.Unmarshal(event.Payload, &payloadMap); err != nil {
				// Fallback to storing raw bytes or string
			}

			metadata := map[string]any{
				"ambient_id":               event.ID,
				"timestamp":                time.Unix(event.Timestamp, 0).Format(time.RFC3339),
				objects.FieldKeyTargetKind: targetKind,
			}
			if payloadMap != nil {
				metadata[objects.FieldKeyPayload] = payloadMap
			} else if len(event.Payload) > 0 {
				metadata[objects.FieldKeyPayload] = string(event.Payload)
			}
			if targetURI != "" {
				metadata["target_uri"] = targetURI
			}

			opts := &storage.AuditEventOptions{
				EventType:  string(event.Type),
				Operation:  fmt.Sprintf("Ambient event: %s", event.Type),
				Severity:   "low",
				TargetKind: targetKind,
				Metadata:   metadata,
				CreatedAt:  time.Unix(event.Timestamp, 0).Format(time.RFC3339),
			}

			_ = storage.CreateAuditEventWithBuilder(context.Background(), root, sCtx, nil, opts)
		})
	}

	return nil
}

// Subscribe returns a channel that will receive events of the specified types.
// The provided context controls the lifetime of the subscription.
func (m *InMemoryEventMesh) Subscribe(ctx context.Context, types []EventType) (<-chan AmbientEvent, error) {
	ch := make(chan AmbientEvent, 100)

	_ = concurrency.RunInLock(&m.mu, func() error {
		for _, t := range types {
			m.subscribers[t] = append(m.subscribers[t], ch)
		}
		return nil
	})

	// Managed background cleanup per concurrency guidelines
	concurrency.RunWithMaxWait(func() {
		<-ctx.Done()
		_ = concurrency.RunInLock(&m.mu, func() error {
			for _, t := range types {
				subs := m.subscribers[t]
				for i, sub := range subs {
					if sub == ch {
						m.subscribers[t] = append(subs[:i], subs[i+1:]...)
						break
					}
				}
			}
			return nil
		})
	}, 0)

	return ch, nil
}
