package objects

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/datacell"
)

func TestKernelCritical_Functions(t *testing.T) {
	t.Parallel()
	// cloneBoolPtr
	assert.Nil(t, cloneBoolPtr(nil))
	trueVal := true
	cloned := cloneBoolPtr(&trueVal)
	require.NotNil(t, cloned)
	assert.True(t, *cloned)

	// defaultKernelCriticalForProfile
	assert.False(t, defaultKernelCriticalForProfile(string(datacell.ProfileStream)))
	assert.False(t, defaultKernelCriticalForProfile(string(datacell.ProfileLightFile)))
	assert.True(t, defaultKernelCriticalForProfile(string(datacell.ProfileCASEntity)))
	assert.True(t, defaultKernelCriticalForProfile(""))

	// applyKernelCriticalDefaults
	var nilSpec *Spec
	applyKernelCriticalDefaults(nilSpec) // no panic

	specWithExplicit := &Spec{KernelCritical: &trueVal}
	applyKernelCriticalDefaults(specWithExplicit)
	assert.True(t, *specWithExplicit.KernelCritical)

	specStream := &Spec{StorageProfile: string(datacell.ProfileStream)}
	applyKernelCriticalDefaults(specStream)
	require.NotNil(t, specStream.KernelCritical)
	assert.False(t, *specStream.KernelCritical)

	// EffectiveKernelCritical
	assert.False(t, EffectiveKernelCritical(nil))
	assert.True(t, EffectiveKernelCritical(specWithExplicit))
	specCas := &Spec{StorageProfile: string(datacell.ProfileCASEntity)}
	assert.True(t, EffectiveKernelCritical(specCas))

	// IsKernelCriticalKind
	assert.False(t, IsKernelCriticalKind(""))
	assert.True(t, IsKernelCriticalKind(KindGoal))

	// ListKernelCriticalKinds
	criticalKinds := ListKernelCriticalKinds()
	assert.NotEmpty(t, criticalKinds)
}

func TestMergeResolvedOverlay_Variants(t *testing.T) {
	t.Parallel()
	mergeResolvedOverlay(nil, nil) // no panic
	dst := &Spec{}
	mergeResolvedOverlay(dst, nil) // no panic

	src := &Spec{
		StorageProfile: "cas_entity",
		ResolvedFields: map[string]any{
			"f1": "src_val1",
			"f2": "src_val2",
		},
		ResolvedTraits: []string{"trait_a", "trait_b"},
		ExcludeTraits:  []string{"ex_a", "ex_b"},
	}

	dst = &Spec{
		ResolvedFields: map[string]any{
			"f1": "dst_val1", // existing key wins
		},
		ResolvedTraits: []string{"trait_a"}, // existing trait wins, deduped
		ExcludeTraits:  []string{"ex_a"},
	}

	mergeResolvedOverlay(dst, src)
	assert.Equal(t, "cas_entity", dst.StorageProfile)
	assert.Equal(t, "dst_val1", dst.ResolvedFields["f1"])
	assert.Equal(t, "src_val2", dst.ResolvedFields["f2"])
	assert.ElementsMatch(t, []string{"trait_a", "trait_b"}, dst.ResolvedTraits)
	assert.ElementsMatch(t, []string{"ex_a", "ex_b"}, dst.ExcludeTraits)
}

func TestMergeComposes_Variants(t *testing.T) {
	t.Parallel()
	loader := NewSpecLoader(findSpecsDir())
	assert.NoError(t, loader.mergeComposes(nil, 0))

	var nilLoader *SpecLoader
	assert.NoError(t, nilLoader.mergeComposes(&Spec{}, 0))

	specWithNull := &Spec{
		Ontology: "test_kind",
		Composes: []string{"", "null"},
	}
	assert.NoError(t, loader.mergeComposes(specWithNull, 0))
}
