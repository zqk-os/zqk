package objects_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/objects"
)

func TestExtractGoalRefs(t *testing.T) {
	t.Run("nil and empty map", func(t *testing.T) {
		assert.Nil(t, objects.ExtractGoalRefs(nil))
		assert.Empty(t, objects.ExtractGoalRefs(map[string]any{}))
	})

	t.Run("FieldKeyGoalRefs as slice of string", func(t *testing.T) {
		obj := map[string]any{
			objects.FieldKeyGoalRefs: []string{"GOAL-001", "GOAL-002"},
		}
		res := objects.ExtractGoalRefs(obj)
		require.Equal(t, []string{"GOAL-001", "GOAL-002"}, res)
	})

	t.Run("FieldKeyGoalRefs as slice of any with prefix", func(t *testing.T) {
		obj := map[string]any{
			objects.FieldKeyGoalRefs: []any{"goal:GOAL-001", "GOAL-002", "   "},
		}
		res := objects.ExtractGoalRefs(obj)
		require.Equal(t, []string{"GOAL-001", "GOAL-002"}, res)
	})

	t.Run("FieldKeyGoalRefs comma separated string", func(t *testing.T) {
		obj := map[string]any{
			objects.FieldKeyGoalRefs: "GOAL-001, GOAL-002, GOAL-001",
		}
		res := objects.ExtractGoalRefs(obj)
		require.Equal(t, []string{"GOAL-001", "GOAL-002"}, res)
	})

	t.Run("singular goal_ref", func(t *testing.T) {
		obj := map[string]any{
			"goal_ref": "GOAL-SOLO",
		}
		res := objects.ExtractGoalRefs(obj)
		require.Equal(t, []string{"GOAL-SOLO"}, res)
	})

	t.Run("blocking and linked goal refs fallback", func(t *testing.T) {
		obj1 := map[string]any{
			objects.FieldKeyBlockingGoalRefs: []string{"GOAL-BLOCK"},
		}
		assert.Equal(t, []string{"GOAL-BLOCK"}, objects.ExtractGoalRefs(obj1))

		obj2 := map[string]any{
			objects.FieldKeyLinkedGoalRefs: []string{"GOAL-LINK"},
		}
		assert.Equal(t, []string{"GOAL-LINK"}, objects.ExtractGoalRefs(obj2))
	})

	t.Run("related_object_refs fallback", func(t *testing.T) {
		obj := map[string]any{
			objects.FieldKeyRelatedObjectRefs: []any{"CVS-1", "GOAL-REL-1", "BLI-1", "goal:GOAL-REL-2"},
		}
		res := objects.ExtractGoalRefs(obj)
		require.Equal(t, []string{"GOAL-REL-1", "GOAL-REL-2"}, res)
	})

	t.Run("deduplication and empty handling", func(t *testing.T) {
		obj := map[string]any{
			objects.FieldKeyGoalRefs: []any{"GOAL-1", "GOAL-1", "", "<nil>", nil},
		}
		res := objects.ExtractGoalRefs(obj)
		require.Equal(t, []string{"GOAL-1"}, res)
	})
}
