package system

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

type mockAlignStorage struct {
	storage.ObjectStorageProvider
	objects map[string]map[string]any
}

func (m *mockAlignStorage) Read(ctx context.Context, sec *pkgctx.SecurityContext, id string) (map[string]any, error) {
	if obj, ok := m.objects[id]; ok {
		return obj, nil
	}
	return nil, fmt.Errorf("object %s not found", id)
}

func (m *mockAlignStorage) List(ctx context.Context, sec *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter storage.ListFilter) (*storage.QueryResult, error) {
	var result []map[string]any
	for _, obj := range m.objects {
		kind, _ := obj["_kind"].(string)
		if kind == "" {
			if id, ok := obj[objects.FieldKeyID].(string); ok {
				if len(id) >= 3 {
					switch id[:3] {
					case "BLI":
						kind = objects.KindBacklogItem
					case "REQ":
						kind = objects.KindRequirement
					case "GOA":
						kind = objects.KindGoal
					}
				}
			}
		}
		if filter.Kind != "" && kind != filter.Kind {
			continue
		}
		result = append(result, obj)
	}
	return &storage.QueryResult{
		Objects: result,
	}, nil
}

func initTestAlignContext(t *testing.T, cmd *cobra.Command, root string, store storage.ObjectStorageProvider, out *bytes.Buffer) {
	stdCtx := cmd.Context()
	if stdCtx == nil {
		stdCtx = context.Background()
	}
	if out != nil {
		stdCtx = pkgctx.WithCommandOutputWriter(stdCtx, out)
	}
	if store != nil {
		stdCtx = cli.WithStorageProvider(stdCtx, store)
	}
	cmd.SetContext(stdCtx)
	initCtx := pkgctx.NewCliInitializationContext(func(s string) string { return root }, root)
	cliCtx, err := cli.GetContextFromCommand(cmd, initCtx)
	require.NoError(t, err)
	cli.SetContext(cmd, cliCtx)
}

// TestAlign_TransitiveRequirementGoalLineage verifies GH-981 / CRIT-aad5b48a:
// Backlog items without direct goal_refs inherit strategic alignment from their linked requirement.
func TestAlign_TransitiveRequirementGoalLineage(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	store := &mockAlignStorage{
		objects: map[string]map[string]any{
			"GOAL-100": {
				objects.FieldKeyID:     "GOAL-100",
				"_kind":                objects.KindGoal,
				objects.FieldKeyStatus: objects.ObjectStatusActive,
			},
			"REQ-100": {
				objects.FieldKeyID:       "REQ-100",
				"_kind":                  objects.KindRequirement,
				objects.FieldKeyStatus:   objects.ObjectStatusOriginated,
				objects.FieldKeyGoalRefs: []string{"GOAL-100"},
			},
			"BLI-TRANSITIVE": {
				objects.FieldKeyID:              "BLI-TRANSITIVE",
				"_kind":                         objects.KindBacklogItem,
				objects.FieldKeyStatus:          objects.ObjectStatusPlanned,
				objects.FieldKeyRequirementRefs: []string{"REQ-100"},
				objects.FieldKeyGoalRefs:        []string{}, // Empty direct goal_refs
			},
			"BLI-DIRECT": {
				objects.FieldKeyID:       "BLI-DIRECT",
				"_kind":                  objects.KindBacklogItem,
				objects.FieldKeyStatus:   objects.ObjectStatusPlanned,
				objects.FieldKeyGoalRefs: []string{"GOAL-200"},
			},
			"BLI-ORPHAN": {
				objects.FieldKeyID:              "BLI-ORPHAN",
				"_kind":                         objects.KindBacklogItem,
				objects.FieldKeyStatus:          objects.ObjectStatusPlanned,
				objects.FieldKeyRequirementRefs: []string{},
				objects.FieldKeyGoalRefs:        []string{},
			},
		},
	}

	t.Run("OverallAlignment_RecognizesTransitiveLineage", func(t *testing.T) {
		cmd := NewAlignCmd()
		var outBuf bytes.Buffer
		cmd.SetOut(&outBuf)
		initTestAlignContext(t, cmd, proj.Root, store, &outBuf)
		_ = cmd.Flags().Set("format", "json")

		err := runAlign(cmd, false, "", false, false)
		require.NoError(t, err)

		var res map[string]any
		err = json.Unmarshal(outBuf.Bytes(), &res)
		require.NoError(t, err)

		alignScore, ok := res["alignment_score"].(float64)
		require.True(t, ok)
		// 2 aligned out of 3 total items = 66.67%
		assert.Equal(t, 66.67, alignScore)

		alignment, _ := res["alignment"].(map[string]any)
		goalWork, _ := alignment["goal_work"].(map[string]any)
		assert.Equal(t, float64(3), goalWork["total_work_items"])
		assert.Equal(t, float64(2), goalWork["items_with_goals"])
		assert.Equal(t, float64(1), goalWork["items_without_goals"])
	})

	t.Run("GoalFilter_RecognizesTransitiveLineage", func(t *testing.T) {
		cmd := NewAlignCmd()
		var outBuf bytes.Buffer
		cmd.SetOut(&outBuf)
		initTestAlignContext(t, cmd, proj.Root, store, &outBuf)
		_ = cmd.Flags().Set("format", "json")

		err := runAlign(cmd, false, "GOAL-100", false, false)
		require.NoError(t, err)

		var res map[string]any
		err = json.Unmarshal(outBuf.Bytes(), &res)
		require.NoError(t, err)

		alignedItems, ok := res["aligned_items"].([]any)
		require.True(t, ok)
		require.Len(t, alignedItems, 1)

		item := alignedItems[0].(map[string]any)
		assert.Equal(t, "BLI-TRANSITIVE", item[objects.FieldKeyID])
	})

	t.Run("GapsOnly_ExcludesTransitivelyAlignedItems", func(t *testing.T) {
		cmd := NewAlignCmd()
		var outBuf bytes.Buffer
		cmd.SetOut(&outBuf)
		initTestAlignContext(t, cmd, proj.Root, store, &outBuf)
		_ = cmd.Flags().Set("format", "json")

		err := runAlign(cmd, true, "", false, false)
		require.NoError(t, err)

		var res map[string]any
		err = json.Unmarshal(outBuf.Bytes(), &res)
		require.NoError(t, err)

		gaps, ok := res["gaps"].([]any)
		require.True(t, ok)
		require.Len(t, gaps, 1)

		gapItem := gaps[0].(map[string]any)
		assert.Equal(t, "BLI-ORPHAN", gapItem[objects.FieldKeyID])
	})
}
