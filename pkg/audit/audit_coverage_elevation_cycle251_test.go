package audit

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestPolicyViolationError_Error(t *testing.T) {
	err := &PolicyViolationError{
		PolicyName: "POL-SEC-001",
		Reason:     "access unauthorized for role guest",
	}
	expected := "policy POL-SEC-001 violated: access unauthorized for role guest"
	if err.Error() != expected {
		t.Fatalf("expected %q, got %q", expected, err.Error())
	}
}

func TestPolicyEngine_Validate_EmptyPolicies(t *testing.T) {
	engine := NewPolicyEngine()
	record := AuditRecord{
		ID:        "rec-001",
		Action:    "read",
		Target:    "node/123",
		Timestamp: time.Now().Format(time.RFC3339),
	}
	if err := engine.Validate(context.Background(), record); err != nil {
		t.Fatalf("expected nil error on empty policies, got %v", err)
	}
}

func TestPolicyEngine_Validate_ContextCancelled(t *testing.T) {
	engine := NewPolicyEngine()

	// Slow policy that blocks until context cancels
	engine.RegisterPolicy(&blockingMockPolicy{
		name: "POL-SLOW-001",
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	record := AuditRecord{
		ID:        "rec-002",
		Action:    "write",
		Target:    "node/456",
		Timestamp: time.Now().Format(time.RFC3339),
	}

	err := engine.Validate(ctx, record)
	if err == nil {
		t.Fatal("expected error due to cancelled context, got nil")
	}
	if err != context.Canceled {
		t.Logf("received error: %v (expected context.Canceled)", err)
	}
}

type blockingMockPolicy struct {
	name string
}

func (b *blockingMockPolicy) Name() string { return b.name }
func (b *blockingMockPolicy) Evaluate(ctx context.Context, record AuditRecord) error {
	<-ctx.Done()
	return ctx.Err()
}

func TestPolicyEngine_Validate_MultiplePolicies(t *testing.T) {
	engine := NewPolicyEngine()

	engine.RegisterPolicy(&mockPolicy{
		name: "POL-ALLOW-001",
		isValid: func(r AuditRecord) bool {
			return true
		},
	})
	engine.RegisterPolicy(&mockPolicy{
		name: "POL-DENY-002",
		isValid: func(r AuditRecord) bool {
			return false
		},
	})
	engine.RegisterPolicy(&mockPolicy{
		name: "POL-ALLOW-003",
		isValid: func(r AuditRecord) bool {
			return true
		},
	})

	record := AuditRecord{
		ID:        "rec-003",
		Action:    "update",
		Target:    "node/789",
		Timestamp: time.Now().Format(time.RFC3339),
	}

	err := engine.Validate(context.Background(), record)
	if err == nil {
		t.Fatal("expected error from denying policy, got nil")
	}
}

func TestPolicyEngine_Validate_Concurrent(t *testing.T) {
	engine := NewPolicyEngine()
	engine.RegisterPolicy(&mockPolicy{
		name: "POL-CONC-001",
		isValid: func(r AuditRecord) bool {
			return r.Action != "forbidden"
		},
	})

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			action := "allowed"
			wantErr := false
			if idx%2 == 0 {
				action = "forbidden"
				wantErr = true
			}
			rec := AuditRecord{
				ID:        fmt.Sprintf("rec-conc-%d", idx),
				Action:    action,
				Target:    "node",
				Timestamp: time.Now().Format(time.RFC3339),
			}
			err := engine.Validate(context.Background(), rec)
			if (err != nil) != wantErr {
				t.Errorf("worker %d: unexpected validation result: %v", idx, err)
			}
		}(i)
	}
	wg.Wait()
}

func TestAuditStream_NewAuditStream_WithProjectRoot(t *testing.T) {
	customRoot := "/tmp/test-audit-root"
	stream := NewAuditStream(customRoot)
	if stream == nil {
		t.Fatal("expected non-nil AuditStream")
	}
	if stream.projectRoot != customRoot {
		t.Fatalf("expected projectRoot %q, got %q", customRoot, stream.projectRoot)
	}
}

func TestAuditStream_Publish_SlowSubscriberDrop(t *testing.T) {
	tmpDir := t.TempDir()
	stream := NewAuditStream(tmpDir)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Subscribe without reading: channel buffer size is 100
	ch := stream.Subscribe(ctx)

	// Publish 100 items to fill the buffer
	for i := 0; i < 100; i++ {
		stream.Publish(context.Background(), AuditRecord{
			ID:        fmt.Sprintf("rec-%d", i),
			Action:    "fill",
			Target:    "target",
			Timestamp: time.Now().Format(time.RFC3339),
		})
	}

	// 101st publish must hit default case (slow subscriber drop)
	stream.Publish(context.Background(), AuditRecord{
		ID:        "rec-overflow",
		Action:    "overflow",
		Target:    "target",
		Timestamp: time.Now().Format(time.RFC3339),
	})

	// Wait for cleanup goroutine to process ctx.Done() and delete subscriber
	var closed bool
	timeout := time.After(2 * time.Second)
	// Drain the channel until closed
	for !closed {
		select {
		case _, ok := <-ch:
			if !ok {
				closed = true
			}
		case <-timeout:
			t.Fatal("timed out waiting for subscriber channel to be closed after drop")
		}
	}

	stream.mu.RLock()
	subCount := len(stream.subscribers)
	stream.mu.RUnlock()

	if subCount != 0 {
		t.Fatalf("expected 0 subscribers remaining after drop, got %d", subCount)
	}
}

func TestAuditStream_Publish_EnsureDirFailure(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a regular file at filepath.Join(tmpDir, paths.ProjectDataDir)
	// so EnsureDir fails because .zqk is a file, not a directory
	badDir := filepath.Join(tmpDir, paths.ProjectDataDir)
	if err := fileutil.WriteFile(badDir, []byte("regular file collision"), paths.FilePerm644); err != nil {
		t.Fatalf("failed to create colliding file: %v", err)
	}

	stream := NewAuditStream(tmpDir)

	// Publish should handle EnsureDir error gracefully without panic
	stream.Publish(context.Background(), AuditRecord{
		ID:        "rec-ensuredir-fail",
		Action:    "test",
		Target:    "test",
		Timestamp: time.Now().Format(time.RFC3339),
	})
}

func TestAuditStream_Publish_OpenFileFailure(t *testing.T) {
	tmpDir := t.TempDir()

	// Create the directory structure up to walDir
	walDir := filepath.Join(tmpDir, paths.ProjectDataDir, paths.LogsDir, "audit")
	if err := fileutil.EnsureDir(walDir); err != nil {
		t.Fatalf("failed to ensure walDir: %v", err)
	}

	// Create a DIRECTORY named wal.jsonl inside walDir so OpenFile fails with EISDIR
	walDirPath := filepath.Join(walDir, "wal.jsonl")
	if err := fileutil.EnsureDir(walDirPath); err != nil {
		t.Fatalf("failed to create directory collision: %v", err)
	}

	stream := NewAuditStream(tmpDir)

	// Publish should handle OpenFile error gracefully without panic
	stream.Publish(context.Background(), AuditRecord{
		ID:        "rec-openfile-fail",
		Action:    "test",
		Target:    "test",
		Timestamp: time.Now().Format(time.RFC3339),
	})
}

func TestAuditStream_Multiplexing_MultipleSubscribers(t *testing.T) {
	tmpDir := t.TempDir()
	stream := NewAuditStream(tmpDir)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	const numSubscribers = 3
	const numRecords = 5

	channels := make([]<-chan AuditRecord, numSubscribers)
	for i := 0; i < numSubscribers; i++ {
		channels[i] = stream.Subscribe(ctx)
	}

	for i := 0; i < numRecords; i++ {
		stream.Publish(ctx, AuditRecord{
			ID:        fmt.Sprintf("multi-rec-%d", i),
			Action:    "multiplex",
			Target:    fmt.Sprintf("target-%d", i),
			Timestamp: time.Now().Format(time.RFC3339),
		})
	}

	for subIdx, ch := range channels {
		for recIdx := 0; recIdx < numRecords; recIdx++ {
			select {
			case rec := <-ch:
				expectedID := fmt.Sprintf("multi-rec-%d", recIdx)
				if rec.ID != expectedID {
					t.Errorf("sub %d: expected record ID %s, got %s", subIdx, expectedID, rec.ID)
				}
			case <-ctx.Done():
				t.Fatalf("sub %d: timeout waiting for record %d", subIdx, recIdx)
			}
		}
	}
}

func TestAuditStream_ConcurrentPublishAndSubscribe(t *testing.T) {
	tmpDir := t.TempDir()
	stream := NewAuditStream(tmpDir)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var wg sync.WaitGroup

	// Concurrently subscribe and drain
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ch := stream.Subscribe(ctx)
			for {
				select {
				case _, ok := <-ch:
					if !ok {
						return
					}
				case <-ctx.Done():
					return
				}
			}
		}()
	}

	// Concurrently publish
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				stream.Publish(ctx, AuditRecord{
					ID:        fmt.Sprintf("conc-%d-%d", idx, j),
					Action:    "publish",
					Target:    "target",
					Timestamp: time.Now().Format(time.RFC3339),
				})
			}
		}(i)
	}

	wg.Wait()
}
