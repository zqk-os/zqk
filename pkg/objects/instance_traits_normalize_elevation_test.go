package objects

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMaybeStripRedundantTopLevelTraits_Variants(t *testing.T) {
	t.Parallel()
	loader := NewSpecLoader(findSpecsDir())
	reg := NewTraitRegistry()

	// Guard clauses
	MaybeStripRedundantTopLevelTraits(nil, loader, reg)
	MaybeStripRedundantTopLevelTraits(map[string]any{}, nil, reg)
	MaybeStripRedundantTopLevelTraits(map[string]any{}, loader, nil)

	// Missing traits or missing kind
	noTraits := map[string]any{FieldKeyKind: KindBacklogItem}
	MaybeStripRedundantTopLevelTraits(noTraits, loader, reg)
	assert.Nil(t, noTraits["traits"])

	noKind := map[string]any{"traits": []any{"auditable"}}
	MaybeStripRedundantTopLevelTraits(noKind, loader, reg)
	assert.NotNil(t, noKind["traits"])

	// Redundant traits matching spec exactly gets stripped
	spec, err := loader.LoadSpecWithInheritance(KindBacklogItem + ".yaml")
	if err == nil && spec != nil && len(spec.ResolvedTraits) > 0 {
		redundant := map[string]any{
			FieldKeyKind: KindBacklogItem,
			"traits":     spec.ResolvedTraits,
		}
		MaybeStripRedundantTopLevelTraits(redundant, loader, reg)
		assert.Nil(t, redundant["traits"])
	}

	// Non-redundant traits kept
	extraTraits := map[string]any{
		FieldKeyKind: KindBacklogItem,
		"traits":     []any{"completely_custom_unique_trait"},
	}
	MaybeStripRedundantTopLevelTraits(extraTraits, loader, reg)
	assert.NotNil(t, extraTraits["traits"])
}
