package ambient

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestFSWatcher_FiltersNoise(t *testing.T) {
	// Setup
	tmpDir, err := fileutil.MkdirTemp("", "fswatcher-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer fileutil.RemoveAll(tmpDir)

	// Create noise directory
	gitDir := filepath.Join(tmpDir, ".git")
	_ = fileutil.EnsureDir(gitDir)

	hub := NewEventHub()
	watcher, err := NewFSWatcher(tmpDir, hub)
	if err != nil {
		t.Fatalf("failed to create watcher: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = watcher.Start(ctx)
	if err != nil {
		t.Fatalf("failed to start watcher: %v", err)
	}
	defer func() { _ = watcher.Stop() }()

	// Subscribe to events
	received := make(chan Event, 1)
	hub.Subscribe(EventTypeFilesystem, func(ctx context.Context, event Event) error {
		received <- event
		return nil
	})

	// Action 1: Write to a valid file
	validFile := filepath.Join(tmpDir, "main.go")
	_ = fileutil.WriteSecureFile(validFile, []byte("package main"))

	// Assert: Should receive event
	select {
	case evt := <-received:
		payload := evt.Payload.(map[string]any)
		if payload[objects.FieldKeyTargetID] != validFile {
			t.Errorf("expected event for %s, got %v", validFile, payload[objects.FieldKeyTargetID])
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for valid file event")
	}

	// Action 2: Write to ignored directory
	ignoredFile := filepath.Join(gitDir, "HEAD")
	_ = fileutil.WriteSecureFile(ignoredFile, []byte("ref: refs/heads/main"))

	// Assert: Should NOT receive event
	select {
	case evt := <-received:
		payload := evt.Payload.(map[string]any)
		t.Fatalf("received unexpected event for ignored file: %v", payload[objects.FieldKeyTargetID])
	case <-time.After(1 * time.Second):
		// Success: timeout means the event was properly filtered
	}
}
