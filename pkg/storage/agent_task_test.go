package storage

import (
	"context"
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAgentTaskCreateRoundTrip(t *testing.T) {
	root := t.TempDir()
	MustEnsureProcessSpecsLayoutForTest(t, root)
	f, err := NewFileObjectStorageForTest(root)

	require.NoError(t, err)
	defer func() { _ = f.Shutdown(context.Background()) }()

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	obj := map[string]any{
		objects.FieldKeyKind:   "agent_task",
		objects.FieldKeyID:     "ATK-TEST-123",
		objects.FieldKeyStatus: objects.ObjectStatusInProgress,
		objects.FieldKeyTitle:  "Test",
	}

	err = f.Create(ctx, secCtx, obj)
	require.NoError(t, err)

	readObj, err := f.Read(ctx, secCtx, "ATK-TEST-123")
	require.NoError(t, err)

	assert.Equal(t, "ATK-TEST-123", readObj[objects.FieldKeyID])
	assert.Equal(t, "agent_task", readObj[objects.FieldKeyKind])
	assert.Equal(t, "Test", readObj[objects.FieldKeyTitle])
	assert.Equal(t, objects.ObjectStatusInProgress, readObj[objects.FieldKeyStatus])
}
