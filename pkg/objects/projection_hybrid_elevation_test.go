package objects

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMergeHybridMask(t *testing.T) {
	t.Parallel()
	m1 := HybridProjectionMask{
		Kind:     KindBacklogItem,
		Global:   1 << 0,
		Ext:      1 << 1,
		Overflow: []string{"extra1"},
	}
	m2 := HybridProjectionMask{
		Kind:     KindBacklogItem,
		Global:   1 << 2,
		Ext:      1 << 3,
		Overflow: []string{"extra2", "extra1"},
	}
	merged := MergeHybridMask(m1, m2)
	assert.Equal(t, KindBacklogItem, merged.Kind)
	assert.Equal(t, uint32((1<<0)|(1<<2)), merged.Global)
	assert.Equal(t, uint32((1<<1)|(1<<3)), merged.Ext)
	assert.ElementsMatch(t, []string{"extra1", "extra2"}, merged.Overflow)

	mEmpty := HybridProjectionMask{}
	mergedWithEmpty := MergeHybridMask(mEmpty, m1)
	assert.Equal(t, KindBacklogItem, mergedWithEmpty.Kind)
}

func TestProjectMapHybrid_EdgeCases(t *testing.T) {
	t.Parallel()
	assert.Nil(t, ProjectMapHybrid(nil, HybridProjectionMask{}))

	emptyMask := HybridProjectionMask{}
	src := map[string]any{FieldKeyID: "id-1", "foo": "bar"}
	assert.Empty(t, ProjectMapHybrid(src, emptyMask))

	maskNoMatch := HybridProjectionMask{
		Global:   1 << 31, // FieldKeyCompletedAt
		Overflow: []string{"nonexistent"},
	}
	assert.Nil(t, ProjectMapHybrid(src, maskNoMatch))

	maskWithOverflow := HybridProjectionMask{
		Overflow: []string{"foo"},
	}
	projected := ProjectMapHybrid(src, maskWithOverflow)
	assert.Len(t, projected, 1)
	assert.Equal(t, "bar", projected["foo"])
}

func TestAppendUniqueStringsAndListFieldNames(t *testing.T) {
	t.Parallel()
	a := []string{"  a  ", "b", ""}
	b := []string{"b", " c "}
	res := appendUniqueStrings(a, b)
	assert.Equal(t, []string{"a", "b", "c"}, res)
	assert.Equal(t, a, appendUniqueStrings(a, nil))

	names := ListProjectionFieldNames([]string{" a ", "b", "a", ""}, " b ")
	assert.Equal(t, []string{"a", "b"}, names)
}
