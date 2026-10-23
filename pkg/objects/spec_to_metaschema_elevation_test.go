package objects

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/dna"
)

func TestSpecToMetaSchema_Nil(t *testing.T) {
	t.Parallel()
	var s *Spec
	assert.Nil(t, s.ToMetaSchema())
}

func TestSpecToMetaSchema_Metadata(t *testing.T) {
	t.Parallel()
	kc := true
	spec := &Spec{
		Ontology:       "custom_robot",
		SchemaVersion:  "1.0.0",
		Extends:        "base_object",
		Composes:       []string{"auditable"},
		StorageProfile: "light_file",
		KernelCritical: &kc,
		ResolvedFields: map[string]any{
			"str_field": map[string]any{
				"type": "string",
				"validation": map[string]any{
					"required":   true,
					"min_length": 3,
					"max_length": 50,
					"pattern":    "^[a-z]+$",
				},
				"checklist":   map[string]any{"default": "default_str"},
				"description": "A string field",
			},
		},
		ResolvedTraits: []string{"occupiable", "completable"},
	}

	meta := spec.ToMetaSchema()
	require.NotNil(t, meta)
	assert.Equal(t, "custom_robot", meta.TargetKind)
	assert.True(t, meta.KernelCritical)
	assert.Equal(t, dna.StorageProfile("light_file"), meta.StorageProfile)
	assert.Contains(t, meta.RequiredFields(), "str_field")
	assert.ElementsMatch(t, []string{"occupiable", "completable"}, meta.Traits)

	strRule := meta.Fields["str_field"]
	assert.Equal(t, dna.TypeString, strRule.Type)
	assert.True(t, strRule.Required)
	assert.Equal(t, 3, strRule.MinLength)
	assert.Equal(t, 50, strRule.MaxLength)
	assert.Equal(t, "^[a-z]+$", strRule.Pattern)
	assert.Equal(t, "default_str", strRule.Default)
}

func TestSpecToMetaSchema_FieldTypes(t *testing.T) {
	t.Parallel()
	spec := &Spec{
		Ontology: "custom_robot",
		ResolvedFields: map[string]any{
			"int_field":  map[string]any{"type": "integer"},
			"bool_field": map[string]any{"type": "boolean"},
			"list_field": map[string]any{"type": "array"},
			"map_field":  map[string]any{"type": "map"},
			"urn_field":  map[string]any{"type": "urn"},
			"obj_field":  map[string]any{"type": "object"},
			"enum_field": map[string]any{
				"type": "enum",
				"validation": map[string]any{
					"enum": []any{"opt1", "opt2"},
				},
				"relation": map[string]any{
					"target_kind":   "other_kind",
					"edge_type":     "reference",
					"cascade":       "delete",
					"bidirectional": true,
					"inverse_field": "back_ref",
				},
				"field_profile_code": "PROFILE_A",
				"semantic_type":      "SemType",
				"traits":             []any{"trait1", "trait2"},
			},
		},
	}

	meta := spec.ToMetaSchema()
	require.NotNil(t, meta)
	assert.Equal(t, dna.TypeInt, meta.Fields["int_field"].Type)
	assert.Equal(t, dna.TypeBool, meta.Fields["bool_field"].Type)
	assert.Equal(t, dna.TypeList, meta.Fields["list_field"].Type)
	assert.Equal(t, dna.TypeMap, meta.Fields["map_field"].Type)
	assert.Equal(t, dna.TypeURN, meta.Fields["urn_field"].Type)
	assert.Equal(t, dna.TypeObject, meta.Fields["obj_field"].Type)

	enumRule := meta.Fields["enum_field"]
	assert.Equal(t, dna.TypeEnum, enumRule.Type)
	require.NotNil(t, enumRule.Relation)
	assert.Equal(t, "other_kind", enumRule.Relation.TargetKind)
	assert.True(t, enumRule.Relation.Bidirectional)
	assert.Equal(t, "PROFILE_A", enumRule.FieldProfileCode)
	assert.ElementsMatch(t, []string{"trait1", "trait2"}, enumRule.Traits)
}

func TestSpecToMetaSchema_FallbackToUnresolved(t *testing.T) {
	t.Parallel()
	spec := &Spec{
		Ontology: "unresolved_kind",
		Fields: map[string]any{
			"raw_field": map[string]any{"type": "text"},
		},
		Traits: []string{"raw_trait"},
	}
	meta := spec.ToMetaSchema()
	require.NotNil(t, meta)
	assert.Contains(t, meta.Traits, "raw_trait")
	assert.Contains(t, meta.Fields, "raw_field")
}
