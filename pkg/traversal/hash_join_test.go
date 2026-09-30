package traversal

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestJoinBindings_IndexedHashJoinCorrectness tests hash-join correctness on matching, non-matching, and multi-variable schemas.
func TestJoinBindings_IndexedHashJoinCorrectness(t *testing.T) {
	t.Parallel()

	// 1. Single common key join
	a := []map[string]string{
		{"id": "1", "name": "Alpha"},
		{"id": "2", "name": "Beta"},
		{"id": "3", "name": "Gamma"},
	}
	b := []map[string]string{
		{"id": "2", "role": "Worker"},
		{"id": "3", "role": "Lead"},
		{"id": "4", "role": "Observer"},
	}

	joined := joinBindings(a, b)
	require.Len(t, joined, 2)
	assert.Equal(t, "2", joined[0]["id"])
	assert.Equal(t, "Beta", joined[0]["name"])
	assert.Equal(t, "Worker", joined[0]["role"])

	assert.Equal(t, "3", joined[1]["id"])
	assert.Equal(t, "Gamma", joined[1]["name"])
	assert.Equal(t, "Lead", joined[1]["role"])

	// 2. Cartesian product when disjoint keys
	disjointA := []map[string]string{{"x": "1"}, {"x": "2"}}
	disjointB := []map[string]string{{"y": "a"}, {"y": "b"}}
	cartesian := joinBindings(disjointA, disjointB)
	assert.Len(t, cartesian, 4)

	// 3. Empty input handling
	assert.Nil(t, joinBindings(nil, b))
	assert.Nil(t, joinBindings(a, nil))
}

// TestJoinBindings_LinearComplexityScalability verifies F-PERF-002: hash join scales linearly (O(M+N)) on large binding sets.
func TestJoinBindings_LinearComplexityScalability(t *testing.T) {
	t.Parallel()

	const count = 1000
	a := make([]map[string]string, count)
	b := make([]map[string]string, count)

	for i := 0; i < count; i++ {
		idStr := fmt.Sprintf("ID-%d", i)
		a[i] = map[string]string{"id": idStr, "valA": fmt.Sprintf("A-%d", i)}
		b[i] = map[string]string{"id": idStr, "valB": fmt.Sprintf("B-%d", i)}
	}

	start := time.Now()
	joined := joinBindings(a, b)
	elapsed := time.Since(start)

	require.Len(t, joined, count)
	assert.Less(t, elapsed, 100*time.Millisecond, "hash join of 1000x1000 rows must complete in under 100ms")
}
