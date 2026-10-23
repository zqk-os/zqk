package objects

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStatusHistory_Extraction(t *testing.T) {
	t.Parallel()
	now := time.Now().Truncate(time.Second)

	emptyObj := map[string]any{}
	hist, err := GetStatusHistory(emptyObj)
	require.NoError(t, err)
	assert.Empty(t, hist)

	typedObj := map[string]any{
		FieldKeyStatusHistory: []StatusHistoryEntry{
			{Status: "planned", Timestamp: now},
		},
	}
	hist, err = GetStatusHistory(typedObj)
	require.NoError(t, err)
	assert.Len(t, hist, 1)

	mapObj := map[string]any{
		FieldKeyStatusHistory: []any{
			map[string]any{
				FieldKeyStatus: "active",
				"timestamp":    now.Format(time.RFC3339),
				"reason":       "work started",
				"changed_by":   "agent_1",
			},
			map[string]any{
				FieldKeyStatus: "paused",
				"timestamp":    now,
			},
			StatusHistoryEntry{Status: "complete", Timestamp: now},
		},
	}
	hist, err = GetStatusHistory(mapObj)
	require.NoError(t, err)
	assert.Len(t, hist, 3)
	assert.Equal(t, "active", hist[0].Status)
	assert.Equal(t, "work started", hist[0].Reason)
	assert.Equal(t, "agent_1", hist[0].ChangedBy)
	assert.Equal(t, "paused", hist[1].Status)
	assert.Equal(t, "complete", hist[2].Status)
}

func TestStatusHistory_ErrorsAndMutation(t *testing.T) {
	t.Parallel()
	errObj := map[string]any{FieldKeyStatusHistory: "not_a_slice"}
	_, err := GetStatusHistory(errObj)
	assert.Error(t, err)

	badSliceObj := map[string]any{
		FieldKeyStatusHistory: []any{123},
	}
	_, err = GetStatusHistory(badSliceObj)
	assert.Error(t, err)

	badTimeObj := map[string]any{
		FieldKeyStatusHistory: []any{
			map[string]any{"timestamp": "bad_time"},
		},
	}
	_, err = GetStatusHistory(badTimeObj)
	assert.Error(t, err)

	badTimeTypeObj := map[string]any{
		FieldKeyStatusHistory: []any{
			map[string]any{"timestamp": 12345},
		},
	}
	_, err = GetStatusHistory(badTimeTypeObj)
	assert.Error(t, err)

	mutObj := map[string]any{}
	require.NoError(t, AddStatusHistoryEntry(mutObj, "active", "started", "dev"))
	assert.NotEmpty(t, mutObj[FieldKeyStatusHistory])

	SetStatusHistory(mutObj, nil)
	assert.Nil(t, mutObj[FieldKeyStatusHistory])
}

func TestChangeLog_ExtractionAndMutation(t *testing.T) {
	t.Parallel()
	now := time.Now().Truncate(time.Second)

	emptyObj := map[string]any{}
	cl, err := GetChangeLog(emptyObj)
	require.NoError(t, err)
	assert.Empty(t, cl)

	_, err = GetChangeLog(map[string]any{FieldKeyChangeLog: 123})
	assert.Error(t, err)

	typedObj := map[string]any{
		FieldKeyChangeLog: []ChangeLogEntry{
			{ChangedBy: "user", Timestamp: now},
		},
	}
	cl, err = GetChangeLog(typedObj)
	require.NoError(t, err)
	assert.Len(t, cl, 1)

	mapAnyObj := map[string]any{
		FieldKeyChangeLog: []any{
			map[string]any{
				"changed_by": "admin",
				"timestamp":  now.Format(time.RFC3339),
				"changes": map[any]any{
					"key1": "val1",
				},
				FieldKeyReason: "admin edit",
			},
			map[string]any{
				"changed_by": "bot",
				"timestamp":  now,
				"changes": map[string]any{
					"key2": "val2",
				},
			},
			ChangeLogEntry{ChangedBy: "reviewer", Timestamp: now},
		},
	}
	cl, err = GetChangeLog(mapAnyObj)
	require.NoError(t, err)
	assert.Len(t, cl, 3)
	assert.Equal(t, "val1", cl[0].Changes["key1"])
	assert.Equal(t, "admin edit", cl[0].Reason)

	_, err = GetChangeLog(map[string]any{FieldKeyChangeLog: []any{"bad_entry"}})
	assert.Error(t, err)

	_, err = GetChangeLog(map[string]any{
		FieldKeyChangeLog: []any{map[string]any{"timestamp": "not_time"}},
	})
	assert.Error(t, err)

	_, err = GetChangeLog(map[string]any{
		FieldKeyChangeLog: []any{map[string]any{"timestamp": true}},
	})
	assert.Error(t, err)

	mutObj := map[string]any{}
	AddChangeLogEntry(mutObj, map[string]any{"status": "done"}, "finished", "dev")
	assert.NotEmpty(t, mutObj[FieldKeyChangeLog])

	SetChangeLog(mutObj, nil)
	assert.Nil(t, mutObj[FieldKeyChangeLog])
}
