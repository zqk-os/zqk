package objects

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type dummyUnsetMarker struct{}

func TestParseNumericValue_Types(t *testing.T) {
	t.Parallel()
	val, ok := ParseNumericValue(10)
	assert.True(t, ok)
	assert.Equal(t, 10.0, val)

	val, ok = ParseNumericValue(int64(20))
	assert.True(t, ok)
	assert.Equal(t, 20.0, val)

	val, ok = ParseNumericValue(int32(30))
	assert.True(t, ok)
	assert.Equal(t, 30.0, val)

	val, ok = ParseNumericValue(float64(40.5))
	assert.True(t, ok)
	assert.Equal(t, 40.5, val)

	val, ok = ParseNumericValue(float32(50.5))
	assert.True(t, ok)
	assert.Equal(t, float64(float32(50.5)), val)

	val, ok = ParseNumericValue("  60.75  ")
	assert.True(t, ok)
	assert.Equal(t, 60.75, val)

	_, ok = ParseNumericValue("not_a_number")
	assert.False(t, ok)

	_, ok = ParseNumericValue(true)
	assert.False(t, ok)
}

func TestIsFieldUnsetMarker(t *testing.T) {
	t.Parallel()
	assert.False(t, isFieldUnsetMarker(nil))
	assert.True(t, isFieldUnsetMarker("__UNSET__"))
	assert.True(t, isFieldUnsetMarker("UNSET"))
	assert.True(t, isFieldUnsetMarker(dummyUnsetMarker{}))
	assert.False(t, isFieldUnsetMarker("regular_value"))
	assert.False(t, isFieldUnsetMarker(123))
}

func TestParseEnumSliceAndChecklist(t *testing.T) {
	t.Parallel()
	// parseEnumSlice
	assert.Nil(t, parseEnumSlice("not_a_slice"))
	sliceStrings := parseEnumSlice([]string{"a", "b"})
	assert.Equal(t, []string{"a", "b"}, sliceStrings)

	sliceAny := parseEnumSlice([]any{"x", 42, nil})
	assert.Equal(t, []string{"x", "42"}, sliceAny)

	// parseChecklistEnumString
	assert.Nil(t, parseChecklistEnumString("no allowed values here"))
	assert.Nil(t, parseChecklistEnumString("enum(must be one of [a, b])"))
	assert.Nil(t, parseChecklistEnumString("enum(defined categories [a, b])"))
	assert.Nil(t, parseChecklistEnumString("enum(controlled vocabulary [a, b])"))

	res := parseChecklistEnumString(`enum("foo", "bar", \"baz\")`)
	assert.Equal(t, []string{"foo", "bar", "baz"}, res)
}

func TestValidateObjectFieldRestrictions_Skips(t *testing.T) {
	t.Parallel()
	require.NoError(t, ValidateObjectFieldRestrictions(nil, "criteria", nil))
	require.NoError(t, ValidateObjectFieldRestrictions(nil, "criteria", map[string]any{}))

	skips := map[string]any{
		FieldKeyID:            "ID-1",
		FieldKeyKind:          "criteria",
		FieldKeyStatus:        "active",
		"expected_updated_at": "tomorrow",
		"_dry_run":            true,
		"some_unset":          "__UNSET__",
		"nil_val":             nil,
	}
	require.NoError(t, ValidateObjectFieldRestrictions(nil, "criteria", skips))
}

func TestValidateFieldValue_SlicesAndBounds(t *testing.T) {
	t.Parallel()
	restrEnum := &FieldValueRestriction{
		FieldName:  "tags",
		EnumValues: []string{"tag1", "tag2"},
	}
	assert.NoError(t, restrEnum.ValidateFieldValue([]string{"tag1", "tag2"}))
	assert.Error(t, restrEnum.ValidateFieldValue([]string{"tag1", "tag3"}))
	assert.NoError(t, restrEnum.ValidateFieldValue([]any{"tag1"}))
	assert.Error(t, restrEnum.ValidateFieldValue([]any{"tag9"}))

	restrMinOnly := &FieldValueRestriction{
		FieldName: "age",
		HasMin:    true,
		Min:       18,
	}
	assert.NoError(t, restrMinOnly.ValidateFieldValue(18))
	assert.NoError(t, restrMinOnly.ValidateFieldValue(25))
	assert.Error(t, restrMinOnly.ValidateFieldValue(17))

	restrMaxOnly := &FieldValueRestriction{
		FieldName: "score",
		HasMax:    true,
		Max:       100,
	}
	assert.NoError(t, restrMaxOnly.ValidateFieldValue(100))
	assert.NoError(t, restrMaxOnly.ValidateFieldValue(50))
	assert.Error(t, restrMaxOnly.ValidateFieldValue(101))
}
