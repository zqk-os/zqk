// Traceability: BLI-SYM-008, BLI-SYM-010, REQ-SYM-005
package ambient

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

// EventType defines the type of event the hub processes.
type EventType string

const (
	// EventTypeFilesystem represents an event related to the filesystem.
	EventTypeFilesystem EventType = "filesystem"
	// EventTypeSession represents an event related to an active session.
	EventTypeSession EventType = "session"
	// EventTypeGit represents an event related to version control.
	EventTypeGit EventType = "git"
)

// Event represents a single ambient event.
type Event struct {
	Type      EventType
	Payload   any
	Timestamp time.Time
}

// ExtractFilesystemEventPayload extracts target, operation, and source fields from a filesystem event payload.
func ExtractFilesystemEventPayload(event Event) (target string, op string, source string, ok bool) {
	payload, ok := event.Payload.(map[string]any)
	if !ok {
		return "", "", "", false
	}
	target, _ = payload[objects.FieldKeyTargetID].(string)
	op, _ = payload[objects.FieldKeyOperation].(string)
	source, _ = payload[objects.FieldKeySource].(string)
	return target, op, source, true
}

// EventHandler processes an event.
type EventHandler func(ctx context.Context, event Event) error

// EventHub is the interface for the ambient event hub.
// Target latency < 100ms.
type EventHub interface {
	Subscribe(eventType EventType, handler EventHandler)
	Publish(ctx context.Context, event Event) error
	Status() string
	EnableEventSourcing(projectRoot string, secCtx *pkgctx.SecurityContext)
}

// eventHubImpl implements EventHub.
type eventHubImpl struct {
	mu       sync.RWMutex
	handlers map[EventType][]EventHandler

	projectRoot string
	secCtx      *pkgctx.SecurityContext
}

// NewEventHub creates a new EventHub.
func NewEventHub() EventHub {
	return &eventHubImpl{
		handlers: make(map[EventType][]EventHandler),
	}
}

// EnableEventSourcing configures the hub to emit audit events.
func (h *eventHubImpl) EnableEventSourcing(projectRoot string, secCtx *pkgctx.SecurityContext) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.projectRoot = projectRoot
	h.secCtx = secCtx
}

// Subscribe adds an event handler for a specific event type.
func (h *eventHubImpl) Subscribe(eventType EventType, handler EventHandler) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.handlers[eventType] = append(h.handlers[eventType], handler)
}

// Publish distributes an event to all subscribed handlers.
// Mock implementation delegates synchronously; target latency < 100ms.
func (h *eventHubImpl) Publish(ctx context.Context, event Event) error {
	h.mu.RLock()
	handlers := h.handlers[event.Type]
	h.mu.RUnlock()

	for _, handler := range handlers {
		if err := handler(ctx, event); err != nil {
			return err
		}
	}

	h.mu.RLock()
	root := h.projectRoot
	sCtx := h.secCtx
	h.mu.RUnlock()

	if root != "" && sCtx != nil {
		goroutinelabels.NewGoroutine("ambient_audit", "emit audit for ambient event").StartSimple(func() {
			targetKind := objects.KindAuditEvent
			targetURI := ""

			if payloadMap, ok := event.Payload.(map[string]any); ok {
				if filePath, ok := payloadMap["file"].(string); ok && filePath != "" {
					if strings.HasSuffix(filePath, ".md") {
						targetKind = objects.KindDocEntry
						targetURI = filePath
					} else if strings.HasSuffix(filePath, ".yaml") || strings.HasSuffix(filePath, ".json") {
						targetKind = objects.KindTechnicalSpec
						targetURI = filePath
					}
				}
			}

			metadata := map[string]any{
				"timestamp":                event.Timestamp.Format(time.RFC3339),
				objects.FieldKeyPayload:    event.Payload,
				objects.FieldKeyTargetKind: targetKind,
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
				CreatedAt:  event.Timestamp.Format(time.RFC3339),
			}

			_ = storage.CreateAuditEventWithBuilder(context.Background(), root, sCtx, nil, opts)
		})
	}

	return nil
}

// Status returns the current activity status of the EventHub.
func (h *eventHubImpl) Status() string {
	h.mu.RLock()
	defer h.mu.RUnlock()

	totalHandlers := 0
	for _, hl := range h.handlers {
		totalHandlers += len(hl)
	}

	if totalHandlers > 0 {
		return "active"
	}
	return "idle"
}
