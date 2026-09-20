package ambience

import (
	"context"
	"testing"
	"time"
)

func TestFSEventsEngine_PredictIntent(t *testing.T) {
	mesh := NewInMemoryEventMesh()
	engine := NewFSEventsEngine(mesh)

	tests := []struct {
		name       string
		event      AmbientEvent
		wantAction string
		wantTarget string
		wantConf   float64
	}{
		{
			name: "Go test file",
			event: AmbientEvent{
				Type: EventFileModified,
				URI:  "foo_test.go",
			},
			wantAction: "run_tests",
			wantTarget: "foo_test.go",
			wantConf:   0.8,
		},
		{
			name: "Go source file",
			event: AmbientEvent{
				Type: EventFileModified,
				URI:  "foo.go",
			},
			wantAction: "build",
			wantTarget: "foo.go",
			wantConf:   0.7,
		},
		{
			name: "Go mod file",
			event: AmbientEvent{
				Type: EventFileModified,
				URI:  "go.mod",
			},
			wantAction: "mod_tidy",
			wantTarget: "go.mod",
			wantConf:   0.9,
		},
		{
			name: "Other file",
			event: AmbientEvent{
				Type: EventFileModified,
				URI:  "README.md",
			},
			wantAction: "edit",
			wantTarget: "README.md",
			wantConf:   0.5,
		},
		{
			name: "Not file modified event",
			event: AmbientEvent{
				Type: EventFocusChanged,
				URI:  "foo.go",
			},
			wantAction: "",
			wantTarget: "",
			wantConf:   0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			intent, err := engine.PredictIntent(tt.event)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if intent.Action != tt.wantAction {
				t.Errorf("got Action %q, want %q", intent.Action, tt.wantAction)
			}
			if intent.Target != tt.wantTarget {
				t.Errorf("got Target %q, want %q", intent.Target, tt.wantTarget)
			}
			if intent.Confidence != tt.wantConf {
				t.Errorf("got Confidence %f, want %f", intent.Confidence, tt.wantConf)
			}
		})
	}
}

func TestFSEventsEngine_StartStop(t *testing.T) {
	mesh := NewInMemoryEventMesh()
	engine := NewFSEventsEngine(mesh)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := engine.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Double start should not error, just do nothing
	if err := engine.Start(ctx); err != nil {
		t.Fatalf("Second Start failed: %v", err)
	}

	// Verify we are subscribed by publishing an event
	// We don't have a direct way to observe loop execution without side effects,
	// but we can ensure no deadlocks or panics occur.
	event := AmbientEvent{
		Type:      EventFileModified,
		URI:       "test.go",
		Timestamp: time.Now().Unix(),
	}
	if err := mesh.Publish(ctx, event); err != nil {
		t.Fatalf("Publish failed: %v", err)
	}

	// Give it a tiny bit of time to process
	time.Sleep(50 * time.Millisecond)

	if err := engine.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
}
