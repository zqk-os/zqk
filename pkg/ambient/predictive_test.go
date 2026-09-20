package ambient

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// dummyHub implements EventHub for testing
type dummyHub struct {
	events []Event
	subs   map[EventType][]EventHandler
}

func (h *dummyHub) Publish(ctx context.Context, event Event) error {
	h.events = append(h.events, event)
	if handlers, ok := h.subs[event.Type]; ok {
		for _, handler := range handlers {
			_ = handler(ctx, event)
		}
	}
	return nil
}

func (h *dummyHub) Subscribe(eventType EventType, handler EventHandler) {
	if h.subs == nil {
		h.subs = make(map[EventType][]EventHandler)
	}
	h.subs[eventType] = append(h.subs[eventType], handler)
}

func (h *dummyHub) Status() string { return "dummy" }

func (h *dummyHub) EnableEventSourcing(projectRoot string, secCtx *pkgctx.SecurityContext) {}

func TestPredictiveTaskSpawner_CreatesTestStub(t *testing.T) {
	hub := &dummyHub{}
	NewPredictiveTaskSpawner(hub)

	// Create a temporary Go file
	tmpDir := t.TempDir()
	goFile := filepath.Join(tmpDir, "example.go")
	_ = fileutil.WriteFile(goFile, []byte("package example\n"), paths.FilePerm644)

	err := hub.Publish(context.Background(), Event{
		Type: EventTypeFilesystem,
		Payload: map[string]any{
			objects.FieldKeyTargetID:  goFile,
			objects.FieldKeyOperation: "CREATE",
			objects.FieldKeySource:    "fswatcher",
		},
		Timestamp: time.Now(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	testFile := filepath.Join(tmpDir, "example_test.go")
	if _, err := fileutil.Stat(testFile); fileutil.IsNotExist(err) {
		t.Fatalf("expected test file stub to be created at %s", testFile)
	}

	// Verify the background event was published
	found := false
	for _, e := range hub.events {
		if e.Type == EventTypeSession {
			if payload, ok := e.Payload.(map[string]any); ok {
				if source, _ := payload[objects.FieldKeySource].(string); source == "predictive_spawner" {
					if target, _ := payload[objects.FieldKeyTargetID].(string); target == goFile {
						found = true
						break
					}
				}
			}
		}
	}

	if !found {
		t.Fatalf("expected schedule_vet event to be published")
	}
}

func TestPredictiveTaskSpawner_IgnoresExistingTestFile(t *testing.T) {
	hub := &dummyHub{}
	NewPredictiveTaskSpawner(hub)

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "example_test.go")
	_ = fileutil.WriteFile(testFile, []byte("package example\n// custom test"), paths.FilePerm644)

	goFile := filepath.Join(tmpDir, "example.go")
	_ = fileutil.WriteFile(goFile, []byte("package example\n"), paths.FilePerm644)

	err := hub.Publish(context.Background(), Event{
		Type: EventTypeFilesystem,
		Payload: map[string]any{
			objects.FieldKeyTargetID:  goFile,
			objects.FieldKeyOperation: "CREATE",
			objects.FieldKeySource:    "fswatcher",
		},
		Timestamp: time.Now(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Make sure the custom test file wasn't overwritten
	content, _ := fileutil.ReadFile(testFile)
	if string(content) != "package example\n// custom test" {
		t.Fatalf("expected existing test file to be left intact, got: %s", string(content))
	}
}

func TestPredictiveTaskSpawner_StageConvergenceSession(t *testing.T) {
	hub := &dummyHub{}
	NewPredictiveTaskSpawner(hub)

	err := hub.Publish(context.Background(), Event{
		Type: EventTypeFilesystem,
		Payload: map[string]any{
			objects.FieldKeyTargetID:  filepath.Join(paths.ProcessDir, "priority_plans", "PRI-123.yaml"),
			objects.FieldKeyOperation: "WRITE",
			objects.FieldKeySource:    "fswatcher",
		},
		Timestamp: time.Now(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := false
	for _, e := range hub.events {
		if e.Type == EventTypeSession {
			if payload, ok := e.Payload.(map[string]any); ok {
				if source, _ := payload[objects.FieldKeySource].(string); source == "predictive_spawner" {
					if hType, _ := payload["heuristic_type"].(string); hType == "stage_convergence_session" {
						found = true
						break
					}
				}
			}
		}
	}

	if !found {
		t.Fatalf("expected stage_convergence_session event to be published")
	}
}
