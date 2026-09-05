package storage

import (
	"context"
	"path/filepath"
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
)

func TestPendingAuditEvents_LifetimeCounters(t *testing.T) {
	projectRoot := filepath.Join(t.TempDir(), "test-project")
	mustEnsureProcessSpecsLayout(t, projectRoot)

	fileStorage, err := NewFileObjectStorageForTest(projectRoot)
	if err != nil {
		t.Fatalf("Failed to create FileObjectStorage: %v", err)
	}

	defer func() { _ = fileStorage.Shutdown(context.Background()) }()

	enqBefore, flushBefore := GetPendingAuditEventStats()

	item := PendingAuditItem{
		ProjectRoot: projectRoot,
		SecCtx:      pkgctx.NewSystemSecurityContext(),
		FileStorage: fileStorage,
		Options: &AuditEventOptions{
			EventType: "bulk_test",
			CreatedBy: "system",
		},
	}

	EnqueuePendingAuditEvent(item)

	enqAfter, flushAfter := GetPendingAuditEventStats()
	if enqAfter != enqBefore+1 {
		t.Errorf("expected enqueued to increase by 1, got before=%d after=%d", enqBefore, enqAfter)
	}

	FlushPendingAuditEvents(context.Background(), projectRoot, fileStorage, pkgctx.NewSystemSecurityContext())

	_, flushFinal := GetPendingAuditEventStats()
	if flushFinal != flushBefore+1 {
		t.Errorf("expected flushed to increase by 1, got before=%d after=%d", flushAfter, flushFinal)
	}
}
