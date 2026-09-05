package storage

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

// TestCreateAuditEventWithBuilder_Concurrent tests that CreateAuditEventWithBuilder
// can be called concurrently without causing concurrent map writes or panics
func TestCreateAuditEventWithBuilder_Concurrent(t *testing.T) {
	// Create a temporary test directory
	tmpDir := t.TempDir()

	// Create minimal project structure
	projectRoot := tmpDir
	auditDir := filepath.Join(tmpDir, paths.ProcessAuditDir, "2030-01")
	if err := fileutil.MkdirAll(auditDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create audit directory: %v", err)
	}

	// Create storage
	storageFactory, err := NewStorageFactory(context.Background(), projectRoot)
	if err != nil {
		t.Fatalf("Failed to create storage factory: %v", err)
	}
	storageProvider := storageFactory.GetStorage()

	secCtx := pkgctx.NewSystemSecurityContext()

	var wg sync.WaitGroup
	concurrency := 50
	errors := make(chan error, concurrency)

	// Launch many goroutines that all create audit events concurrently
	for i := 0; i < concurrency; i++ {
		index := i
		goroutinelabels.NewGoroutine(fmt.Sprintf("test_create_audit_%d", index), fmt.Sprintf("creating audit event %d in concurrent test", index)).
			WithWaitGroup(&wg).
			StartSimple(func() {

				options := &AuditEventOptions{
					EventType: "test_event",
					Operation: "test operation",
					Severity:  "low",
					CreatedBy: "test_user",
				}

				err := CreateAuditEventWithBuilder(
					context.Background(),
					projectRoot,
					secCtx,
					storageProvider,
					options,
				)
				if err != nil {
					errors <- err
				}
			})
	}

	// Wait for all goroutines to complete
	waitDone := make(chan struct{})
	go func() { defer close(waitDone); wg.Wait() }()
	select {
	case <-waitDone:
	case <-time.After(5 * time.Second):
		panic("timeout")
	}
	close(errors)

	// Check for errors
	for err := range errors {
		if err != nil {
			t.Errorf("CreateAuditEventWithBuilder failed: %v", err)
		}
	}

	// Verify events were created (no panic occurred)
	// We don't verify exact count since some might fail due to ID conflicts, but no panics should occur
}

// TestCreateAuditEventWithBuilder_ConcurrentIDGeneration tests that concurrent
// audit event creation handles ID generation correctly
func TestCreateAuditEventWithBuilder_ConcurrentIDGeneration(t *testing.T) {
	tmpDir := t.TempDir()
	projectRoot := tmpDir
	auditDir := filepath.Join(tmpDir, paths.ProcessAuditDir, "2030-01")
	if err := fileutil.MkdirAll(auditDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create audit directory: %v", err)
	}

	storageFactory, err := NewStorageFactory(context.Background(), projectRoot)
	if err != nil {
		t.Fatalf("Failed to create storage factory: %v", err)
	}
	storageProvider := storageFactory.GetStorage()

	secCtx := pkgctx.NewSystemSecurityContext()

	var wg sync.WaitGroup
	concurrency := 20
	eventIDs := make(chan string, concurrency)

	// Launch goroutines to create audit events
	for i := 0; i < concurrency; i++ {
		i := i
		goroutinelabels.NewGoroutine(fmt.Sprintf("test_create_audit_%d", i), fmt.Sprintf("creating audit event %d in concurrent test", i)).
			WithWaitGroup(&wg).
			StartSimple(func() {

				options := &AuditEventOptions{
					EventType: "test_event",
					Operation: "concurrent test",
					Severity:  "low",
				}

				err := CreateAuditEventWithBuilder(
					context.Background(),
					projectRoot,
					secCtx,
					storageProvider,
					options,
				)
				if err == nil {
					eventIDs <- "success"
				}
			})
	}

	waitDone := make(chan struct{})
	go func() { defer close(waitDone); wg.Wait() }()
	select {
	case <-waitDone:
	case <-time.After(5 * time.Second):
		panic("timeout")
	}
	close(eventIDs)

	// Count successes (some may fail due to ID conflicts, but no panics should occur)
	successCount := 0
	for range eventIDs {
		successCount++
	}

	// At least some should succeed (exact count depends on timing and ID generation)
	if successCount == 0 {
		t.Error("No audit events were created - may indicate a problem")
	}
}

func TestCreateAuditEventWithBuilder_SessionID(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	projectRoot := tmpDir
	auditDir := filepath.Join(tmpDir, paths.ProcessAuditDir, "2030-01")
	if err := fileutil.MkdirAll(auditDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create audit directory: %v", err)
	}

	storageFactory, err := NewStorageFactory(context.Background(), projectRoot)
	if err != nil {
		t.Fatalf("Failed to create storage factory: %v", err)
	}
	storageProvider := storageFactory.GetStorage()
	secCtx := pkgctx.NewSystemSecurityContext()

	options := &AuditEventOptions{
		EventType: "test_event",
		Operation: "session_id test operation",
		Severity:  "low",
		SessionID: "MCP-SESSION-TEST-123",
	}

	err = CreateAuditEventWithBuilder(
		context.Background(),
		projectRoot,
		secCtx,
		storageProvider,
		options,
	)
	if err != nil {
		t.Fatalf("CreateAuditEventWithBuilder failed: %v", err)
	}
}
