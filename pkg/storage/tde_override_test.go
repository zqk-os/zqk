package storage

import (
	"context"
	"os"
	"testing"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdateStatusOverrideTriggersTDE(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "zqk-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	mustEnsureProcessSpecsLayout(t, tmpDir)

	storage, err := NewFileObjectStorageForTest(tmpDir)
	require.NoError(t, err)
	defer func() { _ = storage.Shutdown(context.Background()) }()

	baseCtx := context.Background()
	secCtx := pkgctx.NewSecurityContext("account:test-agent", []string{"agent"}, []string{"read:*", "write:*"})

	testID := "TEST-12345-0001"

	err = storage.Create(baseCtx, secCtx, map[string]interface{}{
		objects.FieldKeyID:            "CRIT-0001",
		objects.FieldKeyKind:          "criteria",
		objects.FieldKeyStatus:        "not_started",
		objects.FieldKeyCategory:      "functional",
		objects.FieldKeyTitle:         "dummy",
		objects.FieldKeySchemaVersion: "2.0.0",
	})
	require.NoError(t, err)

	testCaseData := map[string]interface{}{
		objects.FieldKeyID:            testID,
		objects.FieldKeyKind:          "test_case",
		objects.FieldKeyTitle:         "Test TDE",
		objects.FieldKeyStatus:        "draft",
		objects.FieldKeyCreatedAt:     time.Now().Format(time.RFC3339),
		objects.FieldKeyUpdatedAt:     time.Now().Format(time.RFC3339),
		objects.FieldKeyCreatedBy:     "account:test-agent",
		objects.FieldKeySchemaVersion: "2.0.0",
		objects.FieldKeyCriteriaRefs:  []string{"CRIT-0001"},
		objects.FieldKeyScope:         "unit",
	}

	err = storage.Create(baseCtx, secCtx, testCaseData)
	require.NoError(t, err)

	updateData := map[string]interface{}{
		objects.FieldKeyStatus: "complete",
	}

	err = storage.Update(baseCtx, secCtx, testID, updateData)
	require.NoError(t, err)

	updatedObj, err := storage.Read(baseCtx, secCtx, testID)
	require.NoError(t, err)

	status, _ := updatedObj[objects.FieldKeyStatus].(string)
	assert.NotEqual(t, "complete", status, "Status should not be 'complete' without scheduler auth")
	assert.Equal(t, "pending_verification", status, "Status should be transitioned to 'pending_verification'")
}
