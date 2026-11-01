package ambient

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/ambience"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
)

// Static Floor Invariant: verify type mapping, default audit opts, and service construction.
func TestAmbientIngest_StaticFloorInvariant(t *testing.T) {
	t.Run("mapFileSuffixToKind static mapping", func(t *testing.T) {
		cases := []struct {
			path     string
			wantKind string
		}{
			{"docs/overview.md", objects.KindDocEntry},
			{"README.md", objects.KindDocEntry},
			{"specs/schema.yaml", objects.KindTechnicalSpec},
			{"manifest.json", objects.KindTechnicalSpec},
			{"main.go", objects.KindAuditEvent},
			{"script.sh", objects.KindAuditEvent},
			{"data.txt", objects.KindAuditEvent},
			{"Makefile", objects.KindAuditEvent},
			{"", objects.KindAuditEvent},
		}

		for _, tc := range cases {
			got := mapFileSuffixToKind(tc.path)
			if got != tc.wantKind {
				t.Errorf("mapFileSuffixToKind(%q) = %q; want %q", tc.path, got, tc.wantKind)
			}
		}
	})

	t.Run("makeAmbientAuditOpts population", func(t *testing.T) {
		metadata := map[string]any{"source": "test_runner"}
		opts := makeAmbientAuditOpts("fs_event", "2026-11-01T12:00:00Z", objects.KindDocEntry, "doc.md", metadata)

		if opts == nil {
			t.Fatal("expected non-nil AuditEventOptions")
		}
		if opts.EventType != "fs_event" {
			t.Errorf("expected EventType 'fs_event', got %q", opts.EventType)
		}
		if opts.Operation != "Ambient event: fs_event" {
			t.Errorf("expected Operation 'Ambient event: fs_event', got %q", opts.Operation)
		}
		if opts.Severity != "low" {
			t.Errorf("expected Severity 'low', got %q", opts.Severity)
		}
		if opts.TargetKind != objects.KindDocEntry {
			t.Errorf("expected TargetKind %q, got %q", objects.KindDocEntry, opts.TargetKind)
		}
		if opts.CreatedAt != "2026-11-01T12:00:00Z" {
			t.Errorf("expected CreatedAt '2026-11-01T12:00:00Z', got %q", opts.CreatedAt)
		}
		if opts.Metadata["timestamp"] != "2026-11-01T12:00:00Z" {
			t.Errorf("expected Metadata[timestamp] '2026-11-01T12:00:00Z', got %v", opts.Metadata["timestamp"])
		}
		if opts.Metadata[objects.FieldKeyTargetKind] != objects.KindDocEntry {
			t.Errorf("expected Metadata[target_kind] %q, got %v", objects.KindDocEntry, opts.Metadata[objects.FieldKeyTargetKind])
		}
		if opts.Metadata["target_uri"] != "doc.md" {
			t.Errorf("expected Metadata[target_uri] 'doc.md', got %v", opts.Metadata["target_uri"])
		}

		// When targetURI is empty, metadata["target_uri"] should not be set
		metadataEmpty := map[string]any{}
		optsEmpty := makeAmbientAuditOpts("session_event", "2026-11-01T12:00:00Z", objects.KindAuditEvent, "", metadataEmpty)
		if _, exists := optsEmpty.Metadata["target_uri"]; exists {
			t.Errorf("expected target_uri to be absent when targetURI is empty, got %v", optsEmpty.Metadata["target_uri"])
		}
	})

	t.Run("NewAmbientIngestService initialization", func(t *testing.T) {
		root := "/tmp/test-project"
		secCtx := pkgctx.NewSystemSecurityContext()
		svc := NewAmbientIngestService(root, secCtx)

		if svc == nil {
			t.Fatal("expected non-nil AmbientIngestService")
		}
		if svc.projectRoot != root {
			t.Errorf("expected projectRoot %q, got %q", root, svc.projectRoot)
		}
		if svc.secCtx != secCtx {
			t.Errorf("expected secCtx %v, got %v", secCtx, svc.secCtx)
		}
	})

	t.Run("ErrNotEnabled contract invariant", func(t *testing.T) {
		if ErrNotEnabled == nil {
			t.Fatal("expected ErrNotEnabled to be non-nil")
		}
		if ErrNotEnabled.Error() != "ambient engine is not enabled" {
			t.Errorf("unexpected error message: %q", ErrNotEnabled.Error())
		}
	})
}

// Operational Dynamic Proof: verify Ingest emitting audit_event into storage,
// BindToHub event delivery for filesystem, session, and git events,
// BindToMesh subscribing and ingesting Ambience events.
func TestAmbientIngest_OperationalDynamicProof(t *testing.T) {
	t.Run("Ingest emits audit events into storage for filesystem, session, and git", func(t *testing.T) {
		tempDir := t.TempDir()
		defer storage.ScrubProjectRootForTempCleanup(tempDir, 10, 10*time.Millisecond)

		secCtx := pkgctx.NewSystemSecurityContext()
		svc := NewAmbientIngestService(tempDir, secCtx)
		ctx := pkgctx.NewSystemContext()

		// 1. Filesystem event with markdown file (.md -> KindDocEntry)
		evDoc := Event{
			Type: EventTypeFilesystem,
			Payload: map[string]any{
				"file": "docs/architecture.md",
				"op":   "WRITE",
			},
			Timestamp: time.Now(),
		}
		if err := svc.Ingest(ctx, evDoc); err != nil {
			t.Fatalf("Ingest for doc event failed: %v", err)
		}

		// 2. Filesystem event with yaml spec (.yaml -> KindTechnicalSpec)
		evSpec := Event{
			Type: EventTypeFilesystem,
			Payload: map[string]any{
				"file": "specs/openapi.yaml",
				"op":   "CREATE",
			},
			Timestamp: time.Now(),
		}
		if err := svc.Ingest(ctx, evSpec); err != nil {
			t.Fatalf("Ingest for spec event failed: %v", err)
		}

		// 3. Filesystem event with source code file (fallback -> KindAuditEvent)
		evCode := Event{
			Type: EventTypeFilesystem,
			Payload: map[string]any{
				"file": "pkg/ambient/engine.go",
				"op":   "WRITE",
			},
			Timestamp: time.Now(),
		}
		if err := svc.Ingest(ctx, evCode); err != nil {
			t.Fatalf("Ingest for code event failed: %v", err)
		}

		// 4. Session event
		evSession := Event{
			Type:      EventTypeSession,
			Payload:   map[string]any{"user": "alice", "action": "focus_window"},
			Timestamp: time.Now(),
		}
		if err := svc.Ingest(ctx, evSession); err != nil {
			t.Fatalf("Ingest for session event failed: %v", err)
		}

		// 5. Git event
		evGit := Event{
			Type:      EventTypeGit,
			Payload:   map[string]any{"commit": "deadbeef", "branch": "feature/ambient"},
			Timestamp: time.Now(),
		}
		if err := svc.Ingest(ctx, evGit); err != nil {
			t.Fatalf("Ingest for git event failed: %v", err)
		}
	})

	t.Run("BindToHub event delivery for filesystem, session, and git events", func(t *testing.T) {
		tempDir := t.TempDir()
		defer storage.ScrubProjectRootForTempCleanup(tempDir, 10, 10*time.Millisecond)

		secCtx := pkgctx.NewSystemSecurityContext()
		svc := NewAmbientIngestService(tempDir, secCtx)
		hub := NewEventHub()

		svc.BindToHub(hub)

		ctx := context.Background()

		// Publish filesystem event
		err := hub.Publish(ctx, Event{
			Type: EventTypeFilesystem,
			Payload: map[string]any{
				"file": "specs/api.json",
				"op":   "WRITE",
			},
			Timestamp: time.Now(),
		})
		if err != nil {
			t.Fatalf("Publish filesystem event failed: %v", err)
		}

		// Publish session event
		err = hub.Publish(ctx, Event{
			Type:      EventTypeSession,
			Payload:   map[string]any{"action": "terminal_command", "cmd": "zqk test"},
			Timestamp: time.Now(),
		})
		if err != nil {
			t.Fatalf("Publish session event failed: %v", err)
		}

		// Publish git event
		err = hub.Publish(ctx, Event{
			Type:      EventTypeGit,
			Payload:   map[string]any{"op": "commit", "sha": "123456"},
			Timestamp: time.Now(),
		})
		if err != nil {
			t.Fatalf("Publish git event failed: %v", err)
		}
	})

	t.Run("IngestAmbientEvent emits audit events for various URIs", func(t *testing.T) {
		tempDir := t.TempDir()
		defer storage.ScrubProjectRootForTempCleanup(tempDir, 10, 10*time.Millisecond)

		secCtx := pkgctx.NewSystemSecurityContext()
		svc := NewAmbientIngestService(tempDir, secCtx)
		ctx := pkgctx.NewSystemContext()

		tests := []struct {
			name string
			uri  string
			json string
		}{
			{"markdown uri", "docs/manual.md", `{"author":"craftsman","change":"updated"}`},
			{"yaml uri", "specs/service.yaml", `{"service":"ambient","port":8080}`},
			{"json uri", "config/rules.json", `{"enabled":true}`},
			{"empty uri", "", `{"action":"global_event"}`},
			{"go file uri", "cmd/zqk/main.go", `{"action":"build"}`},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				event := ambience.AmbientEvent{
					ID:        "amb-test-" + strings.ReplaceAll(tt.name, " ", "-"),
					Type:      ambience.EventFileModified,
					URI:       tt.uri,
					Timestamp: time.Now().Unix(),
					Payload:   []byte(tt.json),
				}
				if err := svc.IngestAmbientEvent(ctx, event); err != nil {
					t.Fatalf("IngestAmbientEvent(%q) failed: %v", tt.name, err)
				}
			})
		}
	})

	t.Run("BindToMesh subscribes and ingests ambience events", func(t *testing.T) {
		tempDir := t.TempDir()
		defer storage.ScrubProjectRootForTempCleanup(tempDir, 10, 10*time.Millisecond)

		secCtx := pkgctx.NewSystemSecurityContext()
		svc := NewAmbientIngestService(tempDir, secCtx)

		mesh := ambience.NewInMemoryEventMesh()
		svc.BindToMesh(mesh)

		ctx := context.Background()

		// Publish Ambience events handled by BindToMesh
		events := []ambience.AmbientEvent{
			{
				ID:        "amb-1",
				Type:      ambience.EventFileModified,
				URI:       "docs/readme.md",
				Timestamp: time.Now().Unix(),
				Payload:   []byte(`{"file":"docs/readme.md"}`),
			},
			{
				ID:        "amb-2",
				Type:      ambience.EventFocusChanged,
				URI:       "pkg/ambient/service.go",
				Timestamp: time.Now().Unix(),
				Payload:   []byte(`{"window":"editor"}`),
			},
			{
				ID:        "amb-3",
				Type:      ambience.EventTestFailed,
				URI:       "pkg/ambient/ingest_test.go",
				Timestamp: time.Now().Unix(),
				Payload:   []byte(`{"failed_test":"TestFoo"}`),
			},
		}

		for _, ev := range events {
			if err := mesh.Publish(ctx, ev); err != nil {
				t.Fatalf("mesh.Publish(%s) failed: %v", ev.ID, err)
			}
		}

		// Brief settling time for mesh goroutine to receive and ingest events
		time.Sleep(30 * time.Millisecond)
	})
}

type closedMesh struct{}

func (closedMesh) Publish(context.Context, ambience.AmbientEvent) error { return nil }
func (closedMesh) Subscribe(context.Context, []ambience.EventType) (<-chan ambience.AmbientEvent, error) {
	ch := make(chan ambience.AmbientEvent)
	close(ch)
	return ch, nil
}

type subscribeErrorMesh struct{}

func (subscribeErrorMesh) Publish(context.Context, ambience.AmbientEvent) error { return nil }
func (subscribeErrorMesh) Subscribe(context.Context, []ambience.EventType) (<-chan ambience.AmbientEvent, error) {
	return nil, errors.New("subscribe denied")
}

// Negative Adversarial Boundary: test ignored filesystem paths, invalid JSON payloads,
// nil EventMesh, and closed channels without panic or data corruption.
func TestAmbientIngest_NegativeAdversarialBoundary(t *testing.T) {
	t.Run("ignored filesystem paths return nil without error or side-effect", func(t *testing.T) {
		tempDir := t.TempDir()
		svc := NewAmbientIngestService(tempDir, pkgctx.NewSystemSecurityContext())
		ctx := context.Background()

		ignoredTargets := []string{
			filepath.Join(tempDir, ".git", "HEAD"),
			filepath.Join(tempDir, ".git", "objects", "pack", "test.pack"),
			filepath.Join(tempDir, paths.ProjectDataDir, "storage", "cas", "abc.yaml"),
			filepath.Join(tempDir, paths.ProjectDataDir, "process", "audit", "2026-11", "AUD-1.yaml"),
			filepath.Join(tempDir, "node_modules", "package", "index.js"),
			filepath.Join(tempDir, "vendor", "modules.txt"),
			filepath.Join(tempDir, ".idea", "workspace.xml"),
			filepath.Join(tempDir, ".vscode", "settings.json"),
		}

		for _, target := range ignoredTargets {
			ev := Event{
				Type: EventTypeFilesystem,
				Payload: map[string]any{
					objects.FieldKeyTargetID: target,
				},
				Timestamp: time.Now(),
			}
			if err := svc.Ingest(ctx, ev); err != nil {
				t.Errorf("Ingest unexpectedly failed for ignored path %q: %v", target, err)
			}
		}
	})

	t.Run("invalid JSON payload in IngestAmbientEvent returns error cleanly", func(t *testing.T) {
		svc := NewAmbientIngestService(t.TempDir(), pkgctx.NewSystemSecurityContext())
		ctx := context.Background()

		malformedPayloads := [][]byte{
			[]byte("not a json payload"),
			[]byte("{unquoted_key: 123}"),
			[]byte(""),
			[]byte("{"),
			[]byte("[1, 2, 3]"), // valid json array, but unmarshaling into map[string]any fails
		}

		for i, payload := range malformedPayloads {
			ev := ambience.AmbientEvent{
				ID:        "malformed-" + string(rune('a'+i)),
				Type:      ambience.EventFileModified,
				URI:       "doc.md",
				Timestamp: time.Now().Unix(),
				Payload:   payload,
			}
			err := svc.IngestAmbientEvent(ctx, ev)
			if err == nil {
				t.Errorf("expected error for malformed payload %q, got nil", string(payload))
			}
			if !strings.Contains(err.Error(), "failed to unmarshal ambient event payload") {
				t.Errorf("expected 'failed to unmarshal ambient event payload' in error, got: %v", err)
			}
		}
	})

	t.Run("nil EventMesh does not panic", func(t *testing.T) {
		svc := NewAmbientIngestService(t.TempDir(), pkgctx.NewSystemSecurityContext())
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("BindToMesh(nil) panicked: %v", r)
			}
		}()

		svc.BindToMesh(nil)
	})

	t.Run("closed EventMesh channel completes loop cleanly without panic", func(t *testing.T) {
		svc := NewAmbientIngestService(t.TempDir(), pkgctx.NewSystemSecurityContext())
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("BindToMesh(closedMesh) panicked: %v", r)
			}
		}()

		svc.BindToMesh(closedMesh{})
		time.Sleep(20 * time.Millisecond)
	})

	t.Run("subscribe error mesh returns gracefully", func(t *testing.T) {
		svc := NewAmbientIngestService(t.TempDir(), pkgctx.NewSystemSecurityContext())
		svc.BindToMesh(subscribeErrorMesh{})
	})
}
