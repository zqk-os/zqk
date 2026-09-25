package objects_test

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/objects"
)

var snakeCaseRegex = regexp.MustCompile(`^[a-z0-9]+(_[a-z0-9]+)*$`)

func TestFieldKeys_CanonicalConstants(t *testing.T) {
	// Essential canonical field keys that every object in the system relies on
	essentialKeys := []struct {
		name  string
		value string
	}{
		{"FieldKeyID", objects.FieldKeyID},
		{"FieldKeyKind", objects.FieldKeyKind},
		{"FieldKeyStatus", objects.FieldKeyStatus},
		{"FieldKeyTitle", objects.FieldKeyTitle},
		{"FieldKeyDescription", objects.FieldKeyDescription},
		{"FieldKeyNamespaceID", objects.FieldKeyNamespaceID},
		{"FieldKeyCreatedAt", objects.FieldKeyCreatedAt},
		{"FieldKeyCreatedBy", objects.FieldKeyCreatedBy},
		{"FieldKeyUpdatedAt", objects.FieldKeyUpdatedAt},
		{"FieldKeyUpdatedBy", objects.FieldKeyUpdatedBy},
		{"FieldKeySchemaVersion", objects.FieldKeySchemaVersion},
		{"FieldKeyCriteriaRefs", objects.FieldKeyCriteriaRefs},
		{"FieldKeyRequirementRefs", objects.FieldKeyRequirementRefs},
		{"FieldKeyMilestoneRefs", objects.FieldKeyMilestoneRefs},
		{"FieldKeyPriorityTier", objects.FieldKeyPriorityTier},
		{"FieldKeyTags", objects.FieldKeyTags},
	}

	seen := make(map[string]string)
	for _, k := range essentialKeys {
		require.NotEmpty(t, k.value, "Constant %s must not be empty", k.name)
		assert.True(t, snakeCaseRegex.MatchString(k.value), "Constant %s (%q) must be snake_case", k.name, k.value)

		if existingName, exists := seen[k.value]; exists {
			t.Errorf("Duplicate field key value %q between %s and %s", k.value, existingName, k.name)
		}
		seen[k.value] = k.name
	}

	// Verify exact mappings for foundational keys
	assert.Equal(t, "id", objects.FieldKeyID)
	assert.Equal(t, "kind", objects.FieldKeyKind)
	assert.Equal(t, "status", objects.FieldKeyStatus)
	assert.Equal(t, "title", objects.FieldKeyTitle)
	assert.Equal(t, "description", objects.FieldKeyDescription)
	assert.Equal(t, "namespace_id", objects.FieldKeyNamespaceID)
	assert.Equal(t, "created_at", objects.FieldKeyCreatedAt)
	assert.Equal(t, "updated_at", objects.FieldKeyUpdatedAt)
}
