package ambient

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/ambience"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
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
func (s *AmbientIngestService) MapToSystemObject(event Event) (string, string) {
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
func (s *AmbientIngestService) Ingest(ctx context.Context, event Event) error {
	targetKind, targetURI := s.MapToSystemObject(event)

	metadata := map[string]any{
		"timestamp":                event.Timestamp.Format(time.RFC3339),
		objects.FieldKeyPayload:    event.Payload,
		objects.FieldKeyTargetKind: targetKind,
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
func (s *AmbientIngestService) BindToHub(hub EventHub) {
	handler := func(ctx context.Context, event Event) error {
		return s.Ingest(ctx, event)
	}

	// Subscribe to known event types
	hub.Subscribe(EventTypeFilesystem, handler)
	hub.Subscribe(EventTypeSession, handler)
	hub.Subscribe(EventTypeGit, handler)
}

// IngestAmbientEvent ingests an ambience.AmbientEvent from EventMesh.
func (s *AmbientIngestService) IngestAmbientEvent(ctx context.Context, event ambience.AmbientEvent) error {
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
	if err := json.Unmarshal(event.Payload, &payloadMap); err != nil {
		return fmt.Errorf("failed to unmarshal ambient event payload: %w", err)
	}

	metadata := map[string]any{
		"ambient_id":               event.ID,
		"timestamp":                time.Unix(event.Timestamp, 0).Format(time.RFC3339),
		objects.FieldKeyPayload:    payloadMap,
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
		CreatedAt:  time.Unix(event.Timestamp, 0).Format(time.RFC3339),
	}

	if err := storage.CreateAuditEventWithBuilder(ctx, s.projectRoot, s.secCtx, nil, opts); err != nil {
		return fmt.Errorf("failed to emit audit event for ambient event: %w", err)
	}

	return nil
}

// BindToMesh binds the service to an EventMesh to automatically ingest events.
func (s *AmbientIngestService) BindToMesh(mesh ambience.EventMesh) {
	ctx := context.Background()
	ch, err := mesh.Subscribe(ctx, []ambience.EventType{
		ambience.EventFileModified,
		ambience.EventFocusChanged,
		ambience.EventTestFailed,
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
