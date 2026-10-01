package integrity

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

type mockIntegrityStorage struct {
	storage.ObjectStorageProvider
	objectsByKind map[string][]map[string]any
	existingIDs   map[string]bool
}

func (m *mockIntegrityStorage) List(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter storage.ListFilter) (*storage.QueryResult, error) {
	if objs, ok := m.objectsByKind[filter.Kind]; ok {
		return &storage.QueryResult{Objects: objs}, nil
	}
	return &storage.QueryResult{}, nil
}

func (m *mockIntegrityStorage) Exists(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (bool, error) {
	return m.existingIDs[id], nil
}

func TestIsRefFieldName(t *testing.T) {
	assert.True(t, IsRefFieldName("goal_ref"))
	assert.True(t, IsRefFieldName("criteria_refs"))
	assert.True(t, IsRefFieldName("dependencies"))
	assert.False(t, IsRefFieldName("title"))
	assert.False(t, IsRefFieldName("description"))
}

func TestRefIDsFromValue(t *testing.T) {
	assert.Nil(t, RefIDsFromValue(nil))
	assert.Nil(t, RefIDsFromValue(""))
	assert.Equal(t, []string{"GOAL-1"}, RefIDsFromValue("GOAL-1"))
	assert.Equal(t, []string{"GOAL-1", "GOAL-2"}, RefIDsFromValue([]string{"GOAL-1", "GOAL-2"}))
	assert.Equal(t, []string{"GOAL-1", "GOAL-2"}, RefIDsFromValue([]any{"GOAL-1", "GOAL-2", 123}))
}

func TestBuildUnlinkUpdates(t *testing.T) {
	obj := map[string]any{
		"goal_ref": "GOAL-GONE",
		"criteria_refs": []string{
			"CRIT-EXISTS",
			"CRIT-GONE",
		},
	}

	missing := map[string]struct{}{
		"GOAL-GONE": {},
		"CRIT-GONE": {},
	}

	// Single ref unset
	updates1, err := BuildUnlinkUpdates(obj, "goal_ref", missing)
	require.NoError(t, err)
	assert.Equal(t, storage.FieldUnset, updates1["goal_ref"])

	// Multi ref slice filtration
	updates2, err := BuildUnlinkUpdates(obj, "criteria_refs", missing)
	require.NoError(t, err)
	assert.Equal(t, []string{"CRIT-EXISTS"}, updates2["criteria_refs"])
}

func TestScanDanglingRefs(t *testing.T) {
	mockStore := &mockIntegrityStorage{
		objectsByKind: map[string][]map[string]any{
			objects.KindBacklogItem: {
				{
					objects.FieldKeyID:   "BLI-001",
					objects.FieldKeyKind: objects.KindBacklogItem,
					"goal_ref":           "GOAL-MISSING",
					"criteria_refs":      []string{"CRIT-EXISTS", "CRIT-MISSING"},
				},
			},
		},
		existingIDs: map[string]bool{
			"CRIT-EXISTS": true,
		},
	}

	secCtx := pkgctx.NewSystemSecurityContext()
	hits := ScanDanglingRefs(context.Background(), secCtx, mockStore, 0)
	require.Len(t, hits, 2)

	assert.Equal(t, "BLI-001", hits[0].ObjectID)
	assert.Contains(t, []string{"goal_ref", "criteria_refs"}, hits[0].Field)
}
