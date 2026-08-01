package storage

import "testing"

// MustEnsureProcessSpecsLayoutForTest exposes mustEnsureProcessSpecsLayout for storage_test package tests.
func MustEnsureProcessSpecsLayoutForTest(t *testing.T, root string) {
	mustEnsureProcessSpecsLayout(t, root)
}
