package coordination

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

func TestStorageAuditRouter_Emit_SkipsAuditEventRecursion(t *testing.T) {
	tmpDir := t.TempDir()
	fileStorage, err := storage.NewFileObjectStorage(tmpDir)
	require.NoError(t, err)
	t.Cleanup(func() {
		assert.NoError(t, fileStorage.Close())
	})

	router := NewStorageAuditRouter(tmpDir, fileStorage)

	callbackCalled := false
	router.SetCallback(func(_, _ string, _ error) {
		callbackCalled = true
	})

	eventCtx := &EventContext{
		OperationID:   "op-audit-recursion",
		OperationType: "create",
		Status:        "success",
		EventData: &EventData{
			AuditMetadata: map[string]interface{}{
				objects.FieldKeyEventType:  "object_created",
				objects.FieldKeyOperation:  "create: success",
				objects.FieldKeySeverity:   "info",
				objects.FieldKeyTargetKind: objects.KindAuditEvent,
				objects.FieldKeyTargetID:   "AUD-001",
			},
		},
	}

	err = router.Emit(context.Background(), eventCtx)
	require.NoError(t, err)
	assert.False(t, callbackCalled, "StorageAuditRouter.Emit must skip audit_event targets to prevent recursive audit-on-audit loops")
}
