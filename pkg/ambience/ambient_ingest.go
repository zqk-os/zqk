// Traceability: ITEM-SYM-031 / ITEM-EXAMPLE
package ambience

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/lanceman/zqk/pkg/ambient"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
)

// AmbientIngestService handles ingesting ambient events directly into the Knowledge Kernel (Graph).
// It maps ambient events to discrete system objects and emits audit_event objects.
type AmbientIngestService struct {
	projectRoot string
	secCtx      *pkgctx.SecurityContext
}

// NewAmbientIngestService creates a new AmbientIngestService.
func NewAmbientIngestService(projectRoot string, secCtx *pkgctx.SecurityContext) *AmbientIngestService {
	return &AmbientIngestService{
		projectRoot: projectRoot,
		secCtx:      secCtx,
	}
}

// MapToSystemObject maps an ambient event payload to a discrete system object kind and target URI.
func (s *AmbientIngestService) MapToSystemObject(event ambient.Event) (string, string) {
	if payloadMap, ok := event.Payload.(map[string]any); ok {
		if filePath, ok := payloadMap["file"].(string); ok && filePath != "" {
			if strings.HasSuffix(filePath, ".md") {
				return objects.KindDocEntry, filePath
			}
			if strings.HasSuffix(filePath, ".yaml") || strings.HasSuffix(filePath, ".json") {
				return objects.KindTechnicalSpec, filePath
			}
		}
	}
	return objects.KindAuditEvent, ""
}

// Ingest ingests an ambient event into the Knowledge Kernel by mapping to discrete system objects and emitting an audit_event.
func (s *AmbientIngestService) Ingest(ctx context.Context, event ambient.Event) error {
	targetKind, targetURI := s.MapToSystemObject(event)

	metadata := map[string]any{
		"timestamp":   event.Timestamp.Format(time.RFC3339),
		"payload":     event.Payload,
		"target_kind": targetKind,
	}
	if targetURI != "" {
		metadata["target_uri"] = targetURI
	}

	opts := &storage.AuditEventOptions{
		EventType: string(event.Type),
		Operation: fmt.Sprintf("Ambient event: %s", event.Type),
		Severity:  "low",
		Metadata:  metadata,
		CreatedAt: event.Timestamp.Format(time.RFC3339),
	}

	if err := storage.CreateAuditEventWithBuilder(ctx, s.projectRoot, s.secCtx, nil, opts); err != nil {
		return fmt.Errorf("failed to emit audit event for ambient event: %w", err)
	}

	return nil
}

// BindToHub binds the service to an EventHub to automatically ingest events.
func (s *AmbientIngestService) BindToHub(hub ambient.EventHub) {
	handler := func(ctx context.Context, event ambient.Event) error {
		return s.Ingest(ctx, event)
	}

	// Subscribe to known event types
	hub.Subscribe(ambient.EventTypeFilesystem, handler)
	hub.Subscribe(ambient.EventTypeSession, handler)
	hub.Subscribe(ambient.EventTypeGit, handler)
}

// IngestAmbientEvent ingests an AmbientEvent from EventMesh.
func (s *AmbientIngestService) IngestAmbientEvent(ctx context.Context, event AmbientEvent) error {
	targetKind := objects.KindAuditEvent
	targetURI := event.URI

	if event.URI != "" {
		if strings.HasSuffix(event.URI, ".md") {
			targetKind = objects.KindDocEntry
		} else if strings.HasSuffix(event.URI, ".yaml") || strings.HasSuffix(event.URI, ".json") {
			targetKind = objects.KindTechnicalSpec
		}
	}

	var payloadMap map[string]any
	_ = json.Unmarshal(event.Payload, &payloadMap)

	metadata := map[string]any{
		"ambient_id":  event.ID,
		"timestamp":   time.Unix(event.Timestamp, 0).Format(time.RFC3339),
		"payload":     payloadMap,
		"target_kind": targetKind,
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

	if err := storage.CreateAuditEventWithBuilder(ctx, s.projectRoot, s.secCtx, nil, opts); err != nil {
		return fmt.Errorf("failed to emit audit event for ambient event: %w", err)
	}

	return nil
}

// BindToMesh binds the service to an EventMesh to automatically ingest events.
func (s *AmbientIngestService) BindToMesh(mesh EventMesh) {
	// Subscribe to known event types
	ctx := context.Background()
	ch, err := mesh.Subscribe(ctx, []EventType{
		EventFileModified,
		EventFocusChanged,
		EventTestFailed,
	})
	if err != nil {
		return
	}

	goroutinelabels.NewGoroutine("ambient_ingest_loop", "Process ambient events from EventMesh").StartSimple(func() {
		for event := range ch {
			_ = s.IngestAmbientEvent(ctx, event)
		}
	})
}
