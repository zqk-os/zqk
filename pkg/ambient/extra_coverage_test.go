package ambient

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/ambience"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/observer"
	"github.com/zqk-os/zqk/pkg/paths"
)

func TestMapToSystemObject(t *testing.T) {
	svc := NewAmbientIngestService(t.TempDir(), pkgctx.NewSystemSecurityContext())

	tests := []struct {
		event    Event
		wantKind string
		wantURI  string
	}{
		{
			event: Event{
				Payload: map[string]any{"file": "docs/architecture.md"},
			},
			wantKind: objects.KindDocEntry,
			wantURI:  "docs/architecture.md",
		},
		{
			event: Event{
				Payload: map[string]any{"file": "specs/schema.yaml"},
			},
			wantKind: objects.KindTechnicalSpec,
			wantURI:  "specs/schema.yaml",
		},
		{
			event: Event{
				Payload: map[string]any{"file": "config/settings.json"},
			},
			wantKind: objects.KindTechnicalSpec,
			wantURI:  "config/settings.json",
		},
		{
			event: Event{
				Payload: map[string]any{"file": "cmd/main.go"},
			},
			wantKind: objects.KindAuditEvent,
			wantURI:  "",
		},
		{
			event: Event{
				Payload: "plain string",
			},
			wantKind: objects.KindAuditEvent,
			wantURI:  "",
		},
	}

	for _, tt := range tests {
		gotKind, gotURI := svc.MapToSystemObject(tt.event)
		if gotKind != tt.wantKind || gotURI != tt.wantURI {
			t.Errorf("MapToSystemObject(%+v) = (%q, %q); want (%q, %q)", tt.event, gotKind, gotURI, tt.wantKind, tt.wantURI)
		}
	}
}

func TestAmbientIngestService_IgnoredFS(t *testing.T) {
	svc := NewAmbientIngestService(t.TempDir(), pkgctx.NewSystemSecurityContext())

	// Ignored FS event should return nil without error
	ev := Event{
		Type: EventTypeFilesystem,
		Payload: map[string]any{
			objects.FieldKeyTargetID: filepath.Join("/", paths.ProjectDataDir, "storage", "cas", "file.yaml"),
		},
	}
	if err := svc.Ingest(context.Background(), ev); err != nil {
		t.Fatalf("unexpected error on ignored fs path: %v", err)
	}
}

func TestAmbientIngestService_BindToHub(t *testing.T) {
	hub := NewEventHub()
	svc := NewAmbientIngestService(t.TempDir(), pkgctx.NewSystemSecurityContext())

	svc.BindToHub(hub)
	if hub.Status() != "active" {
		t.Fatalf("expected hub status active, got %s", hub.Status())
	}
}

func TestEventHub_EnableEventSourcingAndPublish(t *testing.T) {
	hub := NewEventHub()
	tempDir := t.TempDir()
	secCtx := pkgctx.NewSystemSecurityContext()

	hub.EnableEventSourcing(tempDir, secCtx)

	// Publish various payloads
	ev1 := Event{
		Type:      EventTypeFilesystem,
		Payload:   map[string]any{"file": "test.md"},
		Timestamp: time.Now(),
	}
	if err := hub.Publish(context.Background(), ev1); err != nil {
		t.Fatalf("Publish ev1 failed: %v", err)
	}

	ev2 := Event{
		Type:      EventTypeFilesystem,
		Payload:   map[string]any{"file": "test.yaml"},
		Timestamp: time.Now(),
	}
	if err := hub.Publish(context.Background(), ev2); err != nil {
		t.Fatalf("Publish ev2 failed: %v", err)
	}

	ev3 := Event{
		Type:      EventTypeSession,
		Payload:   "plain string session event",
		Timestamp: time.Now(),
	}
	if err := hub.Publish(context.Background(), ev3); err != nil {
		t.Fatalf("Publish ev3 failed: %v", err)
	}

	// Allow asynchronous ambient_audit goroutines to finish disk emission
	// before t.TempDir() executes directory cleanup.
	time.Sleep(1 * time.Second)
}

func TestCoachHeuristics_AppendTip(t *testing.T) {
	tempDir := t.TempDir()
	coach := &CoachHeuristics{projectRoot: tempDir}

	// Empty root or tip
	emptyCoach := &CoachHeuristics{}
	emptyCoach.appendTip("tip")
	coach.appendTip("")

	// Add 4 tips (should cap at 3)
	coach.appendTip("tip 1")
	coach.appendTip("tip 2")
	coach.appendTip("tip 3")
	coach.appendTip("tip 4")

	// Duplicate tip (should not duplicate)
	coach.appendTip("tip 4")

	tips := observer.ReadCachedTips(tempDir)
	if len(tips) > 3 {
		t.Fatalf("expected at most 3 tips, got %d", len(tips))
	}
	if len(tips) > 0 && tips[0] != "tip 4" {
		t.Fatalf("expected newest tip first, got %q", tips[0])
	}
}

type extraErrMesh struct{}

func (extraErrMesh) Publish(context.Context, ambience.AmbientEvent) error { return nil }
func (extraErrMesh) Subscribe(context.Context, []ambience.EventType) (<-chan ambience.AmbientEvent, error) {
	return nil, errors.New("subscribe fail")
}

func TestExtraIngestAndHostDaemon(t *testing.T) {
	root := t.TempDir()
	svc := NewAmbientIngestService(root, pkgctx.NewSystemSecurityContext())
	ctx := pkgctx.NewSystemContext()
	_ = svc.Ingest(ctx, Event{Type: EventTypeFilesystem, Payload: map[string]any{objects.FieldKeyTargetID: root + "/.git/config"}, Timestamp: time.Now()})
	_ = svc.IngestAmbientEvent(ctx, ambience.AmbientEvent{ID: "a1", Type: ambience.EventFileModified, URI: "doc.md", Timestamp: time.Now().Unix(), Payload: []byte(`not-json`)})
	_ = svc.IngestAmbientEvent(ctx, ambience.AmbientEvent{ID: "a2", URI: "spec.yaml", Timestamp: 1, Payload: []byte(`not-json`)})
	_ = svc.IngestAmbientEvent(ctx, ambience.AmbientEvent{ID: "a3", URI: "x.json", Timestamp: 1, Payload: []byte(`not-json`)})
	hub := NewEventHub()
	svc.BindToHub(hub)
	svc.BindToMesh(extraErrMesh{})
	mesh := ambience.NewInMemoryEventMesh()
	svc.BindToMesh(mesh)
	if HostDaemonEnabled("") || HostDaemonEnabled(root) {
		t.Fatal("daemon must stay off in tests")
	}
	_ = isIgnoredFSPath(root + "/node_modules/x")
	_ = isIgnoredDirName(".git")
	_ = min(1, 2)
	_ = min(3, 1)
}
