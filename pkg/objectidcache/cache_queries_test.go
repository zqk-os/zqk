package objectidcache

import (
	"testing"
)

func TestCacheQueries_Smoke(t *testing.T) {
	// Smoke test ensuring query methods instantiate cleanly
	cache := &ObjectIDCache{}
	entries := cache.GetAll()
	if entries == nil {
		// Empty map is expected for zero-value cache
	}
}
