package audit

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestAuditStream_PublishAndSubscribe(t *testing.T) {
	// Temporarily override the working directory so the WAL goes into a temp folder
	tmpDir := t.TempDir()
	originalWd, err := fileutil.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}
	err = fileutil.Chdir(tmpDir)
	if err != nil {
		t.Fatalf("Failed to change working directory: %v", err)
	}
	defer func() {
		_ = fileutil.Chdir(originalWd)
	}()

	stream := NewAuditStream()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	ch := stream.Subscribe(ctx)

	record := AuditRecord{
		ID:        "test-id",
		Action:    "create_node",
		Target:    "test-target",
		Timestamp: time.Now().Format(time.RFC3339),
		Status:    objects.ObjectStatusInProgress,
		Overlay:   "test-overlay",
	}

	stream.Publish(ctx, record)

	select {
	case received := <-ch:
		if received.ID != record.ID {
			t.Errorf("Expected ID %s, got %s", record.ID, received.ID)
		}
	case <-ctx.Done():
		t.Fatal("Timeout waiting for published record")
	}

	// Verify WAL file was created and written
	walPath := filepath.Join(tmpDir, paths.ProjectDataDir, "logs", "audit", "wal.jsonl")
	b, err := fileutil.ReadFile(walPath)
	if err != nil {
		t.Fatalf("Failed to read WAL file: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) != 1 {
		t.Fatalf("Expected 1 line in WAL, got %d", len(lines))
	}

	var parsedRecord AuditRecord
	if err := json.Unmarshal([]byte(lines[0]), &parsedRecord); err != nil {
		t.Fatalf("Failed to parse WAL JSON: %v", err)
	}

	if parsedRecord.ID != record.ID {
		t.Errorf("WAL record ID mismatch: expected %s, got %s", record.ID, parsedRecord.ID)
	}
}

func TestAuditStream_UnsubscribeOnCancel(t *testing.T) {
	stream := NewAuditStream()
	ctx, cancel := context.WithCancel(context.Background())

	ch := stream.Subscribe(ctx)

	stream.mu.RLock()
	if len(stream.subscribers) != 1 {
		t.Errorf("Expected 1 subscriber, got %d", len(stream.subscribers))
	}
	stream.mu.RUnlock()

	// Cancel context to trigger unsubscription
	cancel()

	// Wait a brief moment for the goroutine to process cancellation
	time.Sleep(50 * time.Millisecond)

	stream.mu.RLock()
	if len(stream.subscribers) != 0 {
		t.Errorf("Expected 0 subscribers after cancel, got %d", len(stream.subscribers))
	}
	stream.mu.RUnlock()

	// Channel should be closed
	_, ok := <-ch
	if ok {
		t.Error("Expected channel to be closed")
	}
}
// tdd refresh
