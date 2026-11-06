package ambient

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/zqk-os/zqk/pkg/ambience"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

// AmbientIngestService handles ingesting ambient events directly into the Knowledge Kernel (Graph).
// It maps ambient events to discrete system objects and emits audit_event objects, functioning as
// an autonomous middle-brain holon maintaining subsystem state.
type AmbientIngestService struct {
	projectRoot string
	secCtx      *pkgctx.SecurityContext

	mu         sync.Mutex
	ctx        context.Context
	cancel     context.CancelFunc
	loopWg     sync.WaitGroup
	inFlight   atomic.Int64
	ingestCond *sync.Cond
	ingested   uint64
	channels   []<-chan ambience.AmbientEvent
}

// NewAmbientIngestService creates a new AmbientIngestService.
func NewAmbientIngestService(projectRoot string, secCtx *pkgctx.SecurityContext) *AmbientIngestService {
	ctx, cancel := context.WithCancel(context.Background())
	s := &AmbientIngestService{
		projectRoot: projectRoot,
		secCtx:      secCtx,
		ctx:         ctx,
		cancel:      cancel,
	}
	s.ingestCond = sync.NewCond(&s.mu)
	return s
}

func (s *AmbientIngestService) initLifecycle() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ctx == nil {
		s.ctx, s.cancel = context.WithCancel(context.Background())
	}
	if s.ingestCond == nil {
		s.ingestCond = sync.NewCond(&s.mu)
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
	s.inFlight.Add(1)
	defer s.inFlight.Add(-1)

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

	s.mu.Lock()
	s.ingested++
	if s.ingestCond != nil {
		s.ingestCond.Broadcast()
	}
	s.mu.Unlock()

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
	s.inFlight.Add(1)
	defer s.inFlight.Add(-1)

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

	s.mu.Lock()
	s.ingested++
	if s.ingestCond != nil {
		s.ingestCond.Broadcast()
	}
	s.mu.Unlock()

	return nil
}

// BindToMesh binds the service to an EventMesh to automatically ingest events using internal lifecycle context.
func (s *AmbientIngestService) BindToMesh(mesh ambience.EventMesh) {
	_ = s.BindToMeshWithContext(context.Background(), mesh)
}

// BindToMeshWithContext binds the service to an EventMesh with an optional parent context.
func (s *AmbientIngestService) BindToMeshWithContext(parentCtx context.Context, mesh ambience.EventMesh) error {
	if mesh == nil {
		return nil
	}
	s.initLifecycle()

	s.mu.Lock()
	baseCtx := s.ctx
	s.mu.Unlock()

	loopCtx, loopCancel := context.WithCancel(baseCtx)

	if parentCtx != nil && parentCtx.Done() != nil {
		s.loopWg.Add(1)
		goroutinelabels.NewGoroutine("ambient_ingest_cancel_forwarder", "propagate parent context cancellation to ingest loop").StartSimple(func() {
			defer s.loopWg.Done()
			select {
			case <-parentCtx.Done():
				loopCancel()
			case <-loopCtx.Done():
			}
		})
	}

	ch, err := mesh.Subscribe(loopCtx, []ambience.EventType{
		ambience.EventFileModified,
		ambience.EventFocusChanged,
		ambience.EventTestFailed,
	})
	if err != nil {
		loopCancel()
		return err
	}

	s.mu.Lock()
	s.channels = append(s.channels, ch)
	s.mu.Unlock()

	s.loopWg.Add(1)
	goroutinelabels.NewGoroutine("ambient_ingest_loop", "Process ambient events from EventMesh").StartSimple(func() {
		defer s.loopWg.Done()
		defer loopCancel()

		for {
			select {
			case <-loopCtx.Done():
				return
			case event, ok := <-ch:
				if !ok {
					return
				}
				_ = s.IngestAmbientEvent(loopCtx, event)
			}
		}
	})

	return nil
}

// WaitForCount blocks until at least expectedCount events have been ingested, or timeout expires.
// Returns true if expected count was reached, false on timeout.
func (s *AmbientIngestService) WaitForCount(expectedCount uint64, timeout time.Duration) bool {
	s.initLifecycle()
	deadline := time.Now().Add(timeout)

	s.mu.Lock()
	defer s.mu.Unlock()

	for s.ingested < expectedCount {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return false
		}

		timer := time.AfterFunc(remaining, func() {
			s.mu.Lock()
			if s.ingestCond != nil {
				s.ingestCond.Broadcast()
			}
			s.mu.Unlock()
		})

		s.ingestCond.Wait()
		timer.Stop()
	}

	return true
}

// WaitForIdle blocks until all subscribed channels are drained and all in-flight ingests complete,
// or timeout expires.
func (s *AmbientIngestService) WaitForIdle(timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		s.mu.Lock()
		empty := true
		for _, ch := range s.channels {
			if len(ch) > 0 {
				empty = false
				break
			}
		}
		s.mu.Unlock()

		if empty && s.inFlight.Load() == 0 {
			return true
		}

		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(2 * time.Millisecond)
	}
}

// WaitForIngests waits for any currently in-flight disk audit write operations to finish.
func (s *AmbientIngestService) WaitForIngests() {
	deadline := time.Now().Add(10 * time.Second)
	for s.inFlight.Load() > 0 && time.Now().Before(deadline) {
		time.Sleep(1 * time.Millisecond)
	}
}

// IngestedCount returns the count of completed event ingests.
func (s *AmbientIngestService) IngestedCount() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ingested
}

// Stop cancels all background loops and waits for active loops and in-flight disk writes to complete.
func (s *AmbientIngestService) Stop() {
	s.mu.Lock()
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	s.mu.Unlock()

	s.loopWg.Wait()
	s.WaitForIngests()
}
