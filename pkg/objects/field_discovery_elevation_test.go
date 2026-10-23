package objects

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFieldRegistry_MemoStampAndCacheStamp(t *testing.T) {
	t.Parallel()
	var nilFr *FieldRegistry
	assert.NotEmpty(t, nilFr.memoKey())
	assert.Zero(t, nilFr.memoStamp())
	assert.Zero(t, nilFr.CacheStamp())

	emptyFr := NewFieldRegistry(nil)
	assert.NotEmpty(t, emptyFr.memoKey())
	assert.Zero(t, emptyFr.memoStamp())
	assert.Zero(t, emptyFr.CacheStamp())

	loader := NewSpecLoader(findSpecsDir())
	loadedFr := NewFieldRegistry(loader)
	assert.Equal(t, findSpecsDir(), loadedFr.memoKey())
	assert.Equal(t, uint64(loader.SpecCacheRevision()), uint64(loadedFr.CacheStamp()))
}

func TestGetIntFromMap_Variants(t *testing.T) {
	t.Parallel()
	m := map[string]any{
		"int_val":   42,
		"int64_val": int64(99),
		"str_val":   "not_int",
	}
	assert.Equal(t, 42, getIntFromMap(m, "int_val"))
	assert.Equal(t, 99, getIntFromMap(m, "int64_val"))
	assert.Equal(t, 0, getIntFromMap(m, "str_val"))
	assert.Equal(t, 0, getIntFromMap(m, "missing_key"))

	mAny := map[interface{}]interface{}{
		"int_val":   11,
		"int64_val": int64(22),
	}
	assert.Equal(t, 11, getIntFromMapAny(mAny, "int_val"))
	assert.Equal(t, 22, getIntFromMapAny(mAny, "int64_val"))
	assert.Equal(t, 0, getIntFromMapAny(mAny, "missing_key"))
}

func TestFieldRegistry_LoadFieldsError(t *testing.T) {
	tempDir := t.TempDir()
	badLoader := NewSpecLoader(tempDir)
	badFr := NewFieldRegistry(badLoader)
	err := badFr.LoadFields()
	if err != nil {
		assert.Contains(t, err.Error(), "base_object")
	}
	require.NotNil(t, badFr)
}
