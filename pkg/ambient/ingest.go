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

func mapFileSuffixToKind(filePath string) string {
	if strings.HasSuffix(filePath, ".md") {
		return objects.KindDocEntry
	}
	if strings.HasSuffix(filePath, ".yaml") || strings.HasSuffix(filePath, ".json") {
		return objects.KindTechnicalSpec
	}
	return objects.KindAuditEvent
}

func makeAmbientAuditOpts(eventType, createdAt, targetKind, targetURI string, metadata map[string]any) *storage.AuditEventOptions {
	metadata["timestamp"] = createdAt
	metadata[objects.FieldKeyTargetKind] = targetKind
	if targetURI != "" {
		metadata["target_uri"] = targetURI
	}
	return &storage.AuditEventOptions{
		EventType:  eventType,
		Operation:  fmt.Sprintf("Ambient event: %s", eventType),
		Severity:   "low",
		TargetKind: targetKind,
		Metadata:   metadata,
		CreatedAt:  createdAt,
	}
}

// MapToSystemObject maps an ambient event payload to a discrete system object kind and target URI.
func (s *AmbientIngestService) MapToSystemObject(event Event) (string, string) {
	if payloadMap, ok := event.Payload.(map[string]any); ok {
		if filePath, ok := payloadMap["file"].(string); ok && filePath != "" {
			kind := mapFileSuffixToKind(filePath)
			if kind != objects.KindAuditEvent {
				return kind, filePath
			}
		}
	}
	return objects.KindAuditEvent, ""
}

// Ingest ingests an ambient event into the Knowledge Kernel by mapping to discrete system objects and emitting an audit_event.
func (s *AmbientIngestService) Ingest(ctx context.Context, event Event) error {
	if event.Type == EventTypeFilesystem {
		if payloadMap, ok := event.Payload.(map[string]any); ok {
			if target, ok := payloadMap[objects.FieldKeyTargetID].(string); ok {
				if isIgnoredFSPath(target) {
					return nil
				}
			}
		}
	}

	targetKind, targetURI := s.MapToSystemObject(event)
	metadata := map[string]any{
		objects.FieldKeyPayload: event.Payload,
	}
	opts := makeAmbientAuditOpts(string(event.Type), event.Timestamp.Format(time.RFC3339), targetKind, targetURI, metadata)

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
		if kind := mapFileSuffixToKind(event.URI); kind != objects.KindAuditEvent {
			targetKind = kind
		}
	}

	var payloadMap map[string]any
	if err := json.Unmarshal(event.Payload, &payloadMap); err != nil {
		return fmt.Errorf("failed to unmarshal ambient event payload: %w", err)
	}

	metadata := map[string]any{
		"ambient_id":            event.ID,
		objects.FieldKeyPayload: payloadMap,
	}
	opts := makeAmbientAuditOpts(string(event.Type), time.Unix(event.Timestamp, 0).Format(time.RFC3339), targetKind, targetURI, metadata)

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
