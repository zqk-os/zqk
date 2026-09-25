package koi_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/objects/koi"
)

// TestTypeAssertRemediation verifies that KOI accessors provide safe, typed extraction
// without triggering runtime panics on mismatched, absent, or malformed map types (F-ARCH-005).
func TestTypeAssertRemediation(t *testing.T) {
	sample := map[string]any{
		objects.FieldKeyID:          "REQ-001",
		objects.FieldKeyKind:        "requirement",
		objects.FieldKeyStatus:      "originated",
		objects.FieldKeyTags:        []any{"core", "launch", 123},
		"effort_count":              "42",
		"is_active":                 "true",
		objects.FieldKeyCreatedAt:   "2026-09-25T16:00:00Z",
		"invalid_time":              12345,
		"nil_field":                 nil,
	}

	t.Run("Safe string extraction without panic", func(t *testing.T) {
		assert.Equal(t, "REQ-001", koi.GetString(sample, objects.FieldKeyID))
		assert.Equal(t, "default", koi.GetStringOr(sample, "missing_key", "default"))
		assert.Equal(t, "", koi.GetString(sample, "nil_field"))
	})

	t.Run("Safe slice extraction with mixed types", func(t *testing.T) {
		tags := koi.GetStringSlice(sample, objects.FieldKeyTags)
		assert.Equal(t, []string{"core", "launch", "123"}, tags)
		assert.Empty(t, koi.GetStringSlice(sample, "missing_key"))
	})

	t.Run("Safe numeric and boolean parsing", func(t *testing.T) {
		assert.Equal(t, 42, koi.GetIntOr(sample, "effort_count", 0))
		assert.True(t, koi.GetBoolOr(sample, "is_active", false))
	})

	t.Run("Safe timestamp extraction", func(t *testing.T) {
		ts, ok := koi.GetTime(sample, objects.FieldKeyCreatedAt)
		assert.True(t, ok)
		assert.Equal(t, 2026, ts.Year())

		_, ok = koi.GetTime(sample, "invalid_time")
		assert.False(t, ok)
	})
}
