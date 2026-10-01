package storage

import (
	"testing"

	"github.com/stretchr/testify/assert"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/validation"
)

func TestStorageExtended_Wave14_TestObjectBuilder(t *testing.T) {
	t.Run("populateRequiredFieldsFromConfig", func(t *testing.T) {
		// Non-existent kind -> early return
		objBogus := make(map[string]any)
		populateRequiredFieldsFromConfig(objBogus, "non_existent_kind_xyz", nil, 0)
		assert.Empty(t, objBogus)

		// Test various known kinds
		kinds := []string{
			objects.KindBacklogItem,
			objects.KindGoal,
			objects.KindAgentTask,
			objects.KindAccount,
			objects.KindWorkstream,
			objects.KindGlossaryTerm,
			objects.KindAuditEvent,
		}

		for _, k := range kinds {
			obj := make(map[string]any)
			populateRequiredFieldsFromConfig(obj, k, nil, 1)
			assert.NotEmpty(t, obj, "expected populated fields for kind %s", k)
		}

		// Ensure pre-existing fields are NOT overwritten
		objPre := map[string]any{
			"status": "custom_status",
		}
		populateRequiredFieldsFromConfig(objPre, objects.KindBacklogItem, nil, 2)
		assert.Equal(t, "custom_status", objPre["status"])
	})

	t.Run("generateTestValueForField_AllTypes", func(t *testing.T) {
		lifecycle := &objects.Lifecycle{
			Statuses: []objects.Status{
				{Value: "draft", Origin: false},
				{Value: "active", Origin: true},
			},
		}

		// 1. Status with lifecycle
		val := generateTestValueForField("status", "string", nil, nil, lifecycle, 0)
		assert.Equal(t, "active", val)

		// 2. Enum
		enumValidation := map[string]any{"enum": []any{"valA", "valB"}}
		val = generateTestValueForField("category", "string", nil, enumValidation, nil, 0)
		assert.Equal(t, "valA", val)

		// 3. Special field names
		assert.Equal(t, ConstMiscAccountTestuser, generateTestValueForField("owner_ref", "string", nil, nil, nil, 0))
		assert.Equal(t, "5m", generateTestValueForField(ConstMiscFlushInterval, "string", nil, nil, nil, 0))
		assert.NotEmpty(t, generateTestValueForField("collected_at", "string", nil, nil, nil, 0))
		assert.NotEmpty(t, generateTestValueForField(ConstMiscTargetResolutionDate, "string", nil, nil, nil, 0))
		assert.NotEmpty(t, generateTestValueForField(ConstMiscAggregationWindowStart, "string", nil, nil, nil, 0))
		assert.NotEmpty(t, generateTestValueForField(ConstMiscAggregationWindowEnd, "string", nil, nil, nil, 0))

		// 4. Types
		// string / text
		assert.NotEmpty(t, generateTestValueForField("my_field", "string", nil, nil, nil, 1))
		assert.NotEmpty(t, generateTestValueForField("my_field", "text", nil, nil, nil, 1))

		// datetime / date
		dtVal := generateTestValueForField("my_dt", "datetime", nil, nil, nil, 0)
		assert.Contains(t, dtVal.(string), "T")
		dVal := generateTestValueForField("my_d", "date", nil, nil, nil, 0)
		assert.NotContains(t, dVal.(string), "T")

		// integer / number / float
		assert.Equal(t, 2, generateTestValueForField("int_val", "integer", nil, nil, nil, 1))
		assert.Equal(t, 3.0, generateTestValueForField("flt_val", "number", nil, nil, nil, 2))
		assert.Equal(t, 3.0, generateTestValueForField("flt_val", "float", nil, nil, nil, 2))

		// list / array
		listVal := generateTestValueForField("items", "list", nil, nil, nil, 0)
		assert.IsType(t, []any{}, listVal)
		arrVal := generateTestValueForField("items", "array", nil, nil, nil, 0)
		assert.IsType(t, []any{}, arrVal)

		// object / map
		objVal := generateTestValueForField("obj_field", "object", nil, nil, nil, 0)
		assert.IsType(t, map[string]any{}, objVal)
		mapVal := generateTestValueForField("map_field", "map", nil, nil, nil, 0)
		assert.IsType(t, map[string]any{}, mapVal)

		// boolean / bool
		assert.Equal(t, true, generateTestValueForField("flag", "boolean", nil, nil, nil, 0))
		assert.Equal(t, false, generateTestValueForField("flag", "bool", nil, nil, nil, 1))

		// reference
		refVal := generateTestValueForField("parent_ref", "reference", nil, nil, nil, 0)
		assert.NotEmpty(t, refVal)

		// default fallback
		defVal := generateTestValueForField("custom", "unrecognized_type", nil, nil, nil, 2)
		assert.Equal(t, "test_custom_3", defVal)
	})

	t.Run("inferReferenceKind", func(t *testing.T) {
		assert.Equal(t, objects.KindAccount, inferReferenceKind("owner_ref"))
		assert.Equal(t, objects.KindAccount, inferReferenceKind("created_by"))
		assert.Equal(t, objects.KindAccount, inferReferenceKind("updated_by"))
		assert.Equal(t, objects.KindAccount, inferReferenceKind("archived_by"))
		assert.Equal(t, objects.KindWorkstream, inferReferenceKind(ConstMiscFromWorkstreamRef))
		assert.Equal(t, objects.KindWorkstream, inferReferenceKind(ConstMiscToWorkstreamRef))
		assert.Equal(t, objects.KindVocabularyScheme, inferReferenceKind("scheme_ref"))
		assert.Equal(t, objects.KindGlossaryTerm, inferReferenceKind("predicate_ref"))
		assert.Equal(t, objects.KindGlossaryTerm, inferReferenceKind(ConstMiscSourceTermRef))
		assert.Equal(t, objects.KindGlossaryTerm, inferReferenceKind(ConstMiscTargetTermRef))

		// Plural and singular suffixes
		assert.Equal(t, "organization", inferReferenceKind("organization_refs"))
		assert.Equal(t, "goal", inferReferenceKind("goal_ref"))
		assert.Equal(t, objects.KindAccount, inferReferenceKind("owner_ref"))
		assert.Equal(t, objects.KindWorkstream, inferReferenceKind("sub_workstream_ref"))
		assert.Equal(t, "goal", inferReferenceKind("goalRefs"))
		assert.Equal(t, "goal", inferReferenceKind("goalRef"))
		assert.Equal(t, objects.KindAccount, inferReferenceKind("ownerRef"))
		assert.Equal(t, "", inferReferenceKind("unrelated_field_name"))
	})

	t.Run("getStatusFromLifecycle", func(t *testing.T) {
		// Origin found
		lc1 := &objects.Lifecycle{
			Statuses: []objects.Status{
				{Value: "draft", Origin: false},
				{Value: "opened", Origin: true},
			},
		}
		assert.Equal(t, "opened", getStatusFromLifecycle(lc1))

		// Origin not found -> uses first
		lc2 := &objects.Lifecycle{
			Statuses: []objects.Status{
				{Value: "first_status", Origin: false},
				{Value: "second_status", Origin: false},
			},
		}
		assert.Equal(t, "first_status", getStatusFromLifecycle(lc2))
	})

	t.Run("getEnumValue", func(t *testing.T) {
		assert.Nil(t, getEnumValue(nil, 0))

		// Empty slices
		assert.Nil(t, getEnumValue(map[string]any{"enum": []any{}}, 0))
		assert.Nil(t, getEnumValue(map[string]any{"enum": []string{}}, 0))

		// []any with string and non-string
		valStr := getEnumValue(map[string]any{"enum": []any{"optA", "optB"}}, 1)
		assert.Equal(t, "optB", valStr)

		valInt := getEnumValue(map[string]any{"enum": []any{100, 200}}, 0)
		assert.Equal(t, 100, valInt)

		// []string
		valSliceStr := getEnumValue(map[string]any{"enum": []string{"x", "y", "z"}}, 4)
		assert.Equal(t, "y", valSliceStr)

		// Other type
		assert.Nil(t, getEnumValue(map[string]any{"enum": 123}, 0))
	})

	t.Run("getSpecialFieldValue", func(t *testing.T) {
		assert.Equal(t, ConstMiscAccountTestuser, getSpecialFieldValue("owner_ref"))
		assert.Equal(t, "5m", getSpecialFieldValue(ConstMiscFlushInterval))
		assert.NotEmpty(t, getSpecialFieldValue("collected_at"))
		assert.NotEmpty(t, getSpecialFieldValue(ConstMiscTargetResolutionDate))
		assert.NotEmpty(t, getSpecialFieldValue(ConstMiscAggregationWindowStart))
		assert.NotEmpty(t, getSpecialFieldValue(ConstMiscAggregationWindowEnd))
		assert.Nil(t, getSpecialFieldValue("random_field"))
	})
}

func TestStorageExtended_Wave14_TestObjectBuilderValues(t *testing.T) {
	t.Run("generateStringTestValue", func(t *testing.T) {
		// Pattern constraint
		valPattern := generateStringTestValue("custom_code", nil, map[string]any{
			"pattern": "^[a-z_]+$",
		}, 0)
		assert.Equal(t, "custom_code", valPattern)

		// Semantic type timestamp
		valTS := generateStringTestValue("ts_field", map[string]any{
			"semantic_type": "timestamp",
		}, nil, 0)
		assert.Contains(t, valTS.(string), "T")

		// Semantic type reference
		valRef := generateStringTestValue("goal_ref", map[string]any{
			"semantic_type": "reference",
		}, nil, 0)
		assert.NotEmpty(t, valRef)

		// Special field names
		assert.Equal(t, pkgctx.SystemAccountID, generateStringTestValue("created_by", nil, nil, 0))
		assert.Equal(t, validation.DefaultOriginProject, generateStringTestValue(ConstMiscOriginProject, nil, nil, 0))
		assert.Equal(t, validation.DefaultOriginSystem, generateStringTestValue("origin_system", nil, nil, 0))

		// Default string
		assert.Equal(t, "test_notes_1", generateStringTestValue("notes", map[string]any{}, nil, 0))
	})

	t.Run("matchPatternAndGenerate", func(t *testing.T) {
		// ISO timestamp
		assert.NotEmpty(t, matchPatternAndGenerate(ConstMiscD4D2D2TD2D2D2Z, "ts", 0))
		assert.NotEmpty(t, matchPatternAndGenerate("some_"+ConstMiscD4D2D2TD2D2D2Z1, "ts", 0))

		// Duration
		dur := matchPatternAndGenerate(ConstMiscDSmhd0, "interval", 0)
		assert.Equal(t, "5m", dur)

		// Lowercase identifier
		assert.Equal(t, "test_name", matchPatternAndGenerate("^[a-z_]+$", "Test-Name", 0))

		// Date pattern
		dateVal := matchPatternAndGenerate(ConstMiscD4D2D2, "date_field", 4)
		assert.Equal(t, "2025-01-05", dateVal)

		// Account ID pattern
		assert.Equal(t, pkgctx.SystemAccountID, matchPatternAndGenerate("account:.*", "user", 0))
		assert.Equal(t, pkgctx.SystemAccountID, matchPatternAndGenerate("ACC-.*", "user", 0))

		// Namespace ID pattern
		assert.Equal(t, "zqk:kernel", matchPatternAndGenerate(ConstMiscZqkDomainIntegration+":.*", "ns", 0))

		// Date range pattern
		assert.Equal(t, ConstMisc20250101To20271231, matchPatternAndGenerate(".*", "planning_horizon", 0))
		assert.Equal(t, ConstMisc20250101To20271231, matchPatternAndGenerate(".*", "date_range", 0))

		// No match
		assert.Nil(t, matchPatternAndGenerate("unknown_pattern_regex", "plain_field", 0))
	})

	t.Run("generateLowercaseIdentifier", func(t *testing.T) {
		assert.Equal(t, "hello_world", generateLowercaseIdentifier("Hello-World!"))
		// All non-letters
		assert.Equal(t, "test_field", generateLowercaseIdentifier("12345!@#"))
	})

	t.Run("generateReferenceFromString_and_ID", func(t *testing.T) {
		// Explicit reference kind
		ref1 := generateReferenceFromString("some_field", map[string]any{
			ConstMiscReferenceKind: "goal",
		})
		assert.NotEmpty(t, ref1)

		// Inferred reference kind
		ref2 := generateReferenceFromString("goal_ref", nil)
		assert.NotEmpty(t, ref2)

		// Account kind
		refAcc := generateReferenceID(objects.KindAccount)
		assert.Equal(t, ConstMiscAccountTestuser, refAcc)

		// Empty kind
		refEmpty := generateReferenceID("")
		assert.Equal(t, "REF-999", refEmpty)

		// Short kind < 3 chars
		refShort := generateReferenceID("ab")
		assert.Equal(t, "REF-999", refShort)

		// Long kind without patterns
		refLong := generateReferenceID("customthing")
		assert.NotEmpty(t, refLong)
	})

	t.Run("generateReferenceTestValue", func(t *testing.T) {
		// Special to_workstream_ref
		assert.Equal(t, "WS-998", generateReferenceTestValue(ConstMiscToWorkstreamRef, nil))

		// Explicit reference kind
		ref := generateReferenceTestValue("my_ref", map[string]any{
			ConstMiscReferenceKind: "backlog_item",
		})
		assert.NotEmpty(t, ref)
	})

	t.Run("generateDateTimeTestValue_and_DateTestValue", func(t *testing.T) {
		// DateTime with pattern
		dt1 := generateDateTimeTestValue(map[string]any{"pattern": ConstMiscD4D2D2T}, 2)
		assert.Equal(t, "2025-01-03T00:00:00Z", dt1)

		// DateTime without pattern
		dt2 := generateDateTimeTestValue(nil, 3)
		assert.Equal(t, "2025-01-04T00:00:00Z", dt2)

		// Date with pattern
		d1 := generateDateTestValue(map[string]any{"pattern": ConstMiscD4D2D2}, 2)
		assert.Equal(t, "2025-01-03", d1)

		// Date without pattern
		d2 := generateDateTestValue(nil, 4)
		assert.Equal(t, "2025-01-05", d2)
	})

	t.Run("generateListTestValue_and_generateReferenceList", func(t *testing.T) {
		// Plain list
		list1 := generateListTestValue("tags", nil, map[string]any{"minCount": float64(3)}, 0)
		assert.Len(t, list1, 3)
		assert.Equal(t, "item_1_1", list1[0])

		// Semantic type reference list
		listRef := generateListTestValue("related_items", map[string]any{"semantic_type": "reference"}, map[string]any{"minCount": float64(2)}, 0)
		assert.Len(t, listRef, 2)

		// Suffix _refs with account kind
		listAcc := generateReferenceList("owner_refs", map[string]any{ConstMiscReferenceKind: objects.KindAccount}, 2)
		assert.Len(t, listAcc, 2)
		assert.Equal(t, ConstMiscAccountTestuser, listAcc[0])

		// Reference list with inferred kind from _refs suffix
		listInferred := generateReferenceList("untyped_refs", nil, 2)
		assert.Len(t, listInferred, 2)
		assert.Equal(t, "UNT-999", listInferred[0])
		assert.Equal(t, "UNT-1000", listInferred[1])

		// Reference list with empty kind (no _refs suffix)
		listEmpty := generateReferenceList("plain_field", nil, 2)
		assert.Len(t, listEmpty, 2)
		assert.Equal(t, "REF-999", listEmpty[0])
		assert.Equal(t, "REF-1000", listEmpty[1])
	})
}
