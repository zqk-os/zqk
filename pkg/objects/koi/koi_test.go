package koi

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/objects"
)

func TestKOI_NilSafety(t *testing.T) {
	t.Parallel()

	var nilMap map[string]any

	assert.Equal(t, "", Status(nilMap))
	assert.False(t, IsStatus(nilMap, "in_progress"))
	assert.False(t, InProgress(nilMap))
	assert.False(t, IsTerminal(nilMap))
	assert.False(t, IsWorkDone(nilMap))
	assert.False(t, IsPreliminary(nilMap))
	assert.False(t, IsArchive(nilMap))
	assert.False(t, IsSatisfied(nilMap))
	assert.Equal(t, "", ID(nilMap))
	assert.Equal(t, "", Kind(nilMap))
	assert.Equal(t, "", Title(nilMap))
	assert.Equal(t, "", Namespace(nilMap))
	assert.Equal(t, "", GetString(nilMap, "title"))
	assert.Equal(t, "fallback", GetStringOr(nilMap, "title", "fallback"))
	assert.Nil(t, GetStringSlice(nilMap, "tags"))
	assert.Nil(t, GetMap(nilMap, "metadata"))
	assert.Equal(t, 42, GetIntOr(nilMap, "count", 42))
	assert.Equal(t, int64(100), GetInt64Or(nilMap, "count", 100))
	assert.True(t, GetBoolOr(nilMap, "enabled", true))

	tm, ok := GetTime(nilMap, "created_at")
	assert.False(t, ok)
	assert.True(t, tm.IsZero())

	// Setters should not panic on nil
	assert.NotPanics(t, func() {
		SetStatus(nilMap, "active")
		Set(nilMap, "key", "val")
	})

	// Wrap nil
	w := Wrap(nilMap)
	assert.True(t, w.IsNil())
	assert.Nil(t, w.Raw())
	assert.Equal(t, "", w.ID())
	assert.Equal(t, "", w.Kind())
	assert.Equal(t, "", w.Title())
	assert.Equal(t, "", w.Status())
	assert.Equal(t, "", w.Namespace())
	assert.False(t, w.IsStatus("active"))
	assert.False(t, w.InProgress())
	assert.True(t, w.NotInProgress())
	assert.False(t, w.IsTerminal())
	assert.False(t, w.IsWorkDone())
	assert.False(t, w.IsPreliminary())
	assert.False(t, w.IsArchive())
	assert.False(t, w.IsSatisfied())
	assert.Equal(t, "", w.GetString("foo"))
	assert.Equal(t, "def", w.GetStringOr("foo", "def"))
	assert.Nil(t, w.GetStringSlice("tags"))
	assert.Nil(t, w.GetMap("sub"))
	assert.Equal(t, 5, w.GetIntOr("n", 5))
	assert.Equal(t, int64(50), w.GetInt64Or("n", 50))
	assert.False(t, w.GetBoolOr("b", false))
	_, timeOk := w.GetTime("t")
	assert.False(t, timeOk)

	assert.NotPanics(t, func() {
		w.SetStatus("active").Set("key", "val")
	})
}

func TestKOI_StatusAndLifecyclePredicates(t *testing.T) {
	t.Parallel()

	m := map[string]any{
		objects.FieldKeyID:          "BLI-001",
		objects.FieldKeyKind:        objects.KindBacklogItem,
		objects.FieldKeyTitle:       "Test Backlog Item",
		objects.FieldKeyNamespaceID: "zqk:kernel",
		objects.FieldKeyStatus:      objects.ObjectStatusInProgress,
	}

	assert.Equal(t, "BLI-001", ID(m))
	assert.Equal(t, objects.KindBacklogItem, Kind(m))
	assert.Equal(t, "Test Backlog Item", Title(m))
	assert.Equal(t, "zqk:kernel", Namespace(m))
	assert.Equal(t, objects.ObjectStatusInProgress, Status(m))
	assert.True(t, IsStatus(m, "IN_PROGRESS")) // Case-insensitive
	assert.True(t, InProgress(m))
	assert.False(t, NotInProgress(m))
	assert.False(t, IsTerminal(m))
	assert.False(t, IsWorkDone(m))

	// Fluent Inspector
	w := Wrap(m)
	assert.False(t, w.IsNil())
	assert.Equal(t, "BLI-001", w.ID())
	assert.Equal(t, objects.KindBacklogItem, w.Kind())
	assert.Equal(t, "Test Backlog Item", w.Title())
	assert.Equal(t, "zqk:kernel", w.Namespace())
	assert.Equal(t, objects.ObjectStatusInProgress, w.Status())
	assert.True(t, w.IsStatus(objects.ObjectStatusInProgress))
	assert.True(t, w.InProgress())
	assert.False(t, w.NotInProgress())
	assert.False(t, w.IsTerminal())
	assert.False(t, w.IsWorkDone())

	// Transition to Complete
	w.SetStatus(objects.ObjectStatusComplete)
	assert.Equal(t, objects.ObjectStatusComplete, w.Status())
	assert.False(t, w.InProgress())
	assert.True(t, w.NotInProgress())
	assert.True(t, w.IsTerminal())
	assert.True(t, w.IsWorkDone())

	// Check status checker alignment
	checker := objects.GetGlobalStatusChecker()
	assert.Equal(t, checker.IsTerminal(objects.KindBacklogItem, objects.ObjectStatusComplete), IsTerminal(m))
	assert.Equal(t, checker.IsWorkDone(objects.KindBacklogItem, objects.ObjectStatusComplete), IsWorkDone(m))
	assert.Equal(t, checker.IsArchive(objects.KindBacklogItem, objects.ObjectStatusComplete), IsArchive(m))
	assert.Equal(t, checker.IsPreliminary(objects.KindBacklogItem, objects.ObjectStatusComplete), IsPreliminary(m))

	// Archived status
	m[objects.FieldKeyStatus] = objects.ObjectStatusArchived
	assert.True(t, IsArchive(m))
	assert.True(t, Wrap(m).IsArchive())

	// Preliminary / Exploring status
	m[objects.FieldKeyStatus] = objects.ObjectStatusExploring
	assert.True(t, IsPreliminary(m))
	assert.True(t, Wrap(m).IsPreliminary())
}

func TestKOI_TypeCoercions(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC().Truncate(time.Second)
	nowStr := now.Format(time.RFC3339)

	m := map[string]any{
		"str":        "hello",
		"int_val":    123,
		"int64_val":  int64(456),
		"float_val":  789.0,
		"json_num":   json.Number("999"),
		"str_num":    "42",
		"bool_val":   true,
		"str_bool":   "true",
		"str_bool_0": "0",
		"tags_slice": []string{"tag1", "tag2"},
		"tags_any":   []any{"a", 10, "b"},
		"single_str": "lone_tag",
		"meta":       map[string]any{"nested": "val"},
		"timestamp":  nowStr,
		"time_obj":   now,
	}

	w := Wrap(m)

	// String
	assert.Equal(t, "hello", w.GetString("str"))
	assert.Equal(t, "hello", w.GetStringOr("str", "fallback"))
	assert.Equal(t, "fallback", w.GetStringOr("missing", "fallback"))

	// Numeric coercions
	assert.Equal(t, 123, w.GetIntOr("int_val", 0))
	assert.Equal(t, 456, w.GetIntOr("int64_val", 0))
	assert.Equal(t, 789, w.GetIntOr("float_val", 0))
	assert.Equal(t, 999, w.GetIntOr("json_num", 0))
	assert.Equal(t, 42, w.GetIntOr("str_num", 0))
	assert.Equal(t, 77, w.GetIntOr("missing", 77))

	assert.Equal(t, int64(123), w.GetInt64Or("int_val", 0))
	assert.Equal(t, int64(456), w.GetInt64Or("int64_val", 0))
	assert.Equal(t, int64(789), w.GetInt64Or("float_val", 0))
	assert.Equal(t, int64(999), w.GetInt64Or("json_num", 0))
	assert.Equal(t, int64(42), w.GetInt64Or("str_num", 0))
	assert.Equal(t, int64(88), w.GetInt64Or("missing", 88))

	// Bool
	assert.True(t, w.GetBoolOr("bool_val", false))
	assert.True(t, w.GetBoolOr("str_bool", false))
	assert.False(t, w.GetBoolOr("str_bool_0", true))
	assert.True(t, w.GetBoolOr("missing", true))

	// Slice
	assert.Equal(t, []string{"tag1", "tag2"}, w.GetStringSlice("tags_slice"))
	assert.Equal(t, []string{"a", "10", "b"}, w.GetStringSlice("tags_any"))
	assert.Equal(t, []string{"lone_tag"}, w.GetStringSlice("single_str"))
	assert.Nil(t, w.GetStringSlice("missing"))

	// Map
	assert.Equal(t, map[string]any{"nested": "val"}, w.GetMap("meta"))
	assert.Nil(t, w.GetMap("str"))
	assert.Nil(t, w.GetMap("missing"))

	// Time
	parsedT, ok := w.GetTime("timestamp")
	require.True(t, ok)
	assert.Equal(t, now, parsedT)

	parsedT2, ok2 := w.GetTime("time_obj")
	require.True(t, ok2)
	assert.Equal(t, now, parsedT2)

	_, okFail := w.GetTime("missing")
	assert.False(t, okFail)

	// Set chaining
	w.Set("new_key", "new_val")
	assert.Equal(t, "new_val", w.GetString("new_key"))
}
