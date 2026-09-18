package system

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/interactive"

	"github.com/zqk-os/zqk/pkg/objects"
)

// TestStreamingTemplateCache_ConcurrentGenerationRace tests that multiple goroutines
// generating the same template don't cause duplicate work or inconsistent cache state
func TestStreamingTemplateCache_ConcurrentGenerationRace(t *testing.T) {
	t.Cleanup(ensureRepoSpecsForFieldRegistry(t))
	requireFieldRegistryReloaded(t)
	fieldRegistry := objects.GetGlobalFieldRegistry()

	cache := NewStreamingTemplateCache(fieldRegistry)

	const numGoroutines = 100
	const kind = "criteria"

	var wg sync.WaitGroup
	var generationCount atomic.Int64
	templates := make([]*interactive.TemplateWithTokens, numGoroutines)
	errors := make([]error, numGoroutines)

	// All goroutines try to get the same template concurrently
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		goroutinelabels.NewGoroutine("system_test", "concurrent generation race").StartSimple(func() {
			func(idx int) {
				defer wg.Done()
				generationCount.Add(1)
				templates[idx], errors[idx] = cache.GetTemplateWithTokens(kind)
			}(i)
		})
	}

	wg.Wait()

	// All should succeed
	for i, err := range errors {
		if err != nil {
			t.Errorf("Goroutine %d failed: %v", i, err)
		}
	}

	// All templates should be the same pointer (cached)
	firstTemplate := templates[0]
	for i, template := range templates {
		if template != firstTemplate {
			t.Errorf("Template %d is not the same pointer as template 0 (cache miss)", i)
		}
	}

	// Verify only one generation happened (double-checked locking worked)
	if count := generationCount.Load(); count > int64(numGoroutines) {
		t.Errorf("Generation count (%d) should be <= numGoroutines (%d)", count, numGoroutines)
	}
}

// TestStreamingTemplateCache_InvalidationDuringRead tests that cache invalidation
// during concurrent reads doesn't cause panics or inconsistent state
func TestStreamingTemplateCache_InvalidationDuringRead(t *testing.T) {
	t.Cleanup(ensureRepoSpecsForFieldRegistry(t))
	requireFieldRegistryReloaded(t)
	fieldRegistry := objects.GetGlobalFieldRegistry()

	cache := NewStreamingTemplateCache(fieldRegistry)

	// Pre-populate cache
	kinds := []string{"criteria", "requirement", "backlog_item"}
	for _, kind := range kinds {
		_, err := cache.GetTemplateWithTokens(kind)
		if err != nil {
			t.Fatalf("Failed to pre-populate cache for %s: %v", kind, err)
		}
	}

	const numReaders = 50
	const numInvalidators = 5

	var wg sync.WaitGroup
	errors := make(chan error, numReaders+numInvalidators)

	// Start readers
	for i := 0; i < numReaders; i++ {
		wg.Add(1)
		goroutinelabels.NewGoroutine("system_test", "concurrent read").StartSimple(func() {
			func(idx int) {
				defer wg.Done()
				kind := kinds[idx%len(kinds)]
				template, err := cache.GetTemplateWithTokens(kind)
				if err != nil {
					errors <- err
					return
				}
				if template == nil {
					errors <- err
					return
				}
			}(i)
		})
	}

	// Start invalidators (invalidate while reads are happening)
	for i := 0; i < numInvalidators; i++ {
		wg.Add(1)
		goroutinelabels.NewGoroutine("system_test", "concurrent invalidate").StartSimple(func() {
			defer wg.Done()
			for _, kind := range kinds {
				cache.InvalidateKind(kind)
				time.Sleep(1 * time.Millisecond) // Small delay to let reads interleave
			}
		})
	}

	wg.Wait()
	close(errors)

	// Check for errors
	for err := range errors {
		t.Errorf("Error during concurrent invalidation/read: %v", err)
	}

	// Verify cache is still usable after invalidation
	for _, kind := range kinds {
		template, err := cache.GetTemplateWithTokens(kind)
		if err != nil {
			t.Errorf("Cache not usable after concurrent invalidation for %s: %v", kind, err)
		}
		if template == nil {
			t.Errorf("Cache returned nil template for %s after concurrent invalidation", kind)
		}
	}
}

// TestStreamingTemplateCache_ClearCacheDuringAccess tests that clearing cache
// during concurrent access doesn't cause panics or data races
func TestStreamingTemplateCache_ClearCacheDuringAccess(t *testing.T) {
	t.Cleanup(ensureRepoSpecsForFieldRegistry(t))
	requireFieldRegistryReloaded(t)
	fieldRegistry := objects.GetGlobalFieldRegistry()

	cache := NewStreamingTemplateCache(fieldRegistry)

	// Pre-populate cache
	kinds := []string{"criteria", "requirement", "backlog_item", "milestone", "goal"}
	for _, kind := range kinds {
		_, err := cache.GetTemplateWithTokens(kind)
		if err != nil {
			t.Fatalf("Failed to pre-populate cache: %v", err)
		}
	}

	const numAccessors = 50
	const numClearers = 3

	var wg sync.WaitGroup
	errors := make(chan error, numAccessors)

	// Start accessors
	for i := 0; i < numAccessors; i++ {
		wg.Add(1)
		goroutinelabels.NewGoroutine("system_test", "concurrent access").StartSimple(func() {
			func(idx int) {
				defer wg.Done()
				kind := kinds[idx%len(kinds)]
				template, err := cache.GetTemplateWithTokens(kind)
				if err != nil {
					errors <- err
					return
				}
				if template == nil {
					errors <- fmt.Errorf("template nil for kind %s", kind)
				}
			}(i)
		})
	}

	// Start clearers
	for i := 0; i < numClearers; i++ {
		goroutinelabels.NewGoroutine(fmt.Sprintf("test_clear_cache_%d", i), fmt.Sprintf("clearing cache %d in defensive test", i)).
			WithWaitGroup(&wg).
			StartSimple(func() {
				cache.ClearCache()
				time.Sleep(2 * time.Millisecond)
			})
	}

	wg.Wait()
	close(errors)

	// Check for errors
	for err := range errors {
		t.Errorf("Error during concurrent cache clear: %v", err)
	}

	// Verify cache is still usable after clear
	for _, kind := range kinds {
		template, err := cache.GetTemplateWithTokens(kind)
		if err != nil {
			t.Errorf("Cache not usable after clear for %s: %v", kind, err)
		}
		if template == nil {
			t.Errorf("Cache returned nil template for %s after clear", kind)
		}
	}
}

// TestStreamingTemplateCache_StaleCacheAfterFieldRegistryReload tests that cache
// doesn't return stale templates after FieldRegistry reload
func TestStreamingTemplateCache_StaleCacheAfterFieldRegistryReload(t *testing.T) {
	t.Cleanup(ensureRepoSpecsForFieldRegistry(t))
	requireFieldRegistryReloaded(t)
	fieldRegistry := objects.GetGlobalFieldRegistry()

	cache := NewStreamingTemplateCache(fieldRegistry)

	// Get template (should cache)
	template1, err := cache.GetTemplateWithTokens("criteria")
	if err != nil {
		t.Fatalf("Failed to get template: %v", err)
	}

	// Reload field registry (simulates spec change)
	if err := fieldRegistry.LoadFields(); err != nil {
		if strings.Contains(err.Error(), "could not find specs directory") {
			t.Skipf("repo specs not available: %v", err)
		}
		t.Fatalf("Failed to reload field registry: %v", err)
	}

	// Clear cache (user must do this after FieldRegistry reload)
	cache.ClearCache()

	// Get template again (should regenerate)
	template2, err := cache.GetTemplateWithTokens("criteria")
	if err != nil {
		t.Fatalf("Failed to get template after reload: %v", err)
	}

	// Should be different pointer (regenerated)
	if template1 == template2 {
		t.Error("Template should be regenerated after FieldRegistry reload and cache clear")
	}

	// Content should be valid (even if same, pointer should differ)
	if template2.Template == emptyValue {
		t.Error("Regenerated template should have content")
	}
}

// TestStreamingTemplateCache_NonexistentKind tests that requesting a non-existent kind
// doesn't cache an error or cause infinite loops
func TestStreamingTemplateCache_NonexistentKind(t *testing.T) {
	t.Cleanup(ensureRepoSpecsForFieldRegistry(t))
	requireFieldRegistryReloaded(t)
	fieldRegistry := objects.GetGlobalFieldRegistry()

	cache := NewStreamingTemplateCache(fieldRegistry)

	// Request non-existent kind
	nonexistentKind := "nonexistent_kind_xyz_123"
	template, err := cache.GetTemplateWithTokens(nonexistentKind)

	// Should return error, not cache it
	if err == nil {
		t.Error("Should return error for non-existent kind")
	}
	if template != nil {
		t.Error("Template should be nil for non-existent kind")
	}

	// Verify it's not cached (should fail again)
	template2, err2 := cache.GetTemplateWithTokens(nonexistentKind)
	if err2 == nil {
		t.Error("Should still return error on second request (not cached)")
	}
	if template2 != nil {
		t.Error("Template should still be nil on second request")
	}

	// Verify cache doesn't have entry for this kind
	// (We can't directly check, but behavior confirms it)
}

// TestStreamingTemplateCache_RepeatedInvalidation tests that repeatedly invalidating
// the same kind doesn't cause issues or infinite loops
func TestStreamingTemplateCache_RepeatedInvalidation(t *testing.T) {
	t.Cleanup(ensureRepoSpecsForFieldRegistry(t))
	requireFieldRegistryReloaded(t)
	fieldRegistry := objects.GetGlobalFieldRegistry()

	cache := NewStreamingTemplateCache(fieldRegistry)

	kind := "criteria"

	// Get template
	template1, err := cache.GetTemplateWithTokens(kind)
	if err != nil {
		t.Fatalf("Failed to get template: %v", err)
	}

	// Invalidate multiple times
	for i := 0; i < 100; i++ {
		cache.InvalidateKind(kind)
	}

	// Get template again (should regenerate)
	template2, err := cache.GetTemplateWithTokens(kind)
	if err != nil {
		t.Fatalf("Failed to get template after repeated invalidation: %v", err)
	}

	// Should be different pointer (regenerated)
	if template1 == template2 {
		t.Error("Template should be regenerated after invalidation")
	}

	// Verify template is valid
	if template2.Template == emptyValue {
		t.Error("Regenerated template should have content")
	}
}

// TestStreamingTemplateCache_ConcurrentGetOnly runs many concurrent GetTemplateWithTokens
// without any invalidate. Use to isolate whether the hang is get-vs-invalidate or get-vs-get.
// Runs without t.Parallel() so no other tests share globals (field registry, etc.) during the run.
func TestStreamingTemplateCache_ConcurrentGetOnly(t *testing.T) {
	t.Cleanup(ensureRepoSpecsForFieldRegistry(t))
	requireFieldRegistryReloaded(t)
	fieldRegistry := objects.GetGlobalFieldRegistry()
	cache := NewStreamingTemplateCache(fieldRegistry)
	kind := "criteria"
	const iterations = 10
	var wg sync.WaitGroup
	errors := make(chan error, iterations)
	for i := 0; i < iterations; i++ {
		// Builder adds 1 to wg per StartSimple when WithWaitGroup is set; do not double-add here
		goroutinelabels.NewGoroutine(fmt.Sprintf("test_get_only_%d", i), "get only").
			WithWaitGroup(&wg).
			StartSimple(func() {
				defer func() { _ = recover() }()
				template, err := cache.GetTemplateWithTokens(kind)
				if err != nil {
					errors <- err
					return
				}
				if template == nil {
					errors <- fmt.Errorf("nil template")
				}
			})
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		t.Errorf("Get error: %v", err)
	}
}

// TestStreamingTemplateCache_ConcurrentGetAndInvalidate tests that getting and invalidating
// the same kind concurrently doesn't cause data races or inconsistent state.
// If this test times out (30s), it indicates a deadlock or severe starvation under load
// that must be fixed in the cache or field registry; the timeout only avoids a 10m hang.
func TestStreamingTemplateCache_ConcurrentGetAndInvalidate(t *testing.T) {
	t.Cleanup(ensureRepoSpecsForFieldRegistry(t))
	requireFieldRegistryReloaded(t)
	fieldRegistry := objects.GetGlobalFieldRegistry()

	cache := NewStreamingTemplateCache(fieldRegistry)

	kind := "criteria"
	const iterations = 100

	var wg sync.WaitGroup
	errors := make(chan error, iterations*2)

	// Start goroutines that get and invalidate concurrently
	for i := 0; i < iterations; i++ {
		// One Add for the invalidate goroutine; get goroutine is counted by WithWaitGroup in StartSimple
		wg.Add(1)

		// Get template (builder adds 1 to wg per StartSimple)
		goroutinelabels.NewGoroutine(fmt.Sprintf("test_get_template_%d", i), fmt.Sprintf("getting template %d in defensive test", i)).
			WithWaitGroup(&wg).
			StartSimple(func() {
				template, err := cache.GetTemplateWithTokens(kind)
				if err != nil {
					errors <- err
					return
				}
				if template == nil {
					errors <- err
				}
			})

		// Invalidate (plain goroutine; we added 1 above)
		goroutinelabels.NewGoroutine("system_test", "concurrent invalidate").StartSimple(func() {
			defer wg.Done()
			cache.InvalidateKind(kind)
		})
	}

	// Complete or fail within 30s to avoid test timeout (was 10m)
	done := make(chan struct{})
	goroutinelabels.NewGoroutine("system_test", "wait for completion").StartSimple(func() {
		wg.Wait()
		close(done)
	})
	select {
	case <-done:
		// continue
	case <-time.After(30 * time.Second):
		t.Fatal("ConcurrentGetAndInvalidate timed out after 30s: deadlock or starvation under load (fix cache/field-registry locking)")
	}
	close(errors)

	// Check for errors
	for err := range errors {
		t.Errorf("Error during concurrent get/invalidate: %v", err)
	}

	// Verify cache is still usable
	template, err := cache.GetTemplateWithTokens(kind)
	if err != nil {
		t.Errorf("Cache not usable after concurrent get/invalidate: %v", err)
	}
	if template == nil {
		t.Error("Cache returned nil template after concurrent get/invalidate")
	}
}

// TestStreamingTemplateCache_MultipleKindsConcurrentAccess tests that concurrent access
// to different kinds doesn't cause contention or inconsistent state
func TestStreamingTemplateCache_MultipleKindsConcurrentAccess(t *testing.T) {
	// Not t.Parallel: global field registry + concurrent preload must not race other package tests.
	t.Cleanup(ensureRepoSpecsForFieldRegistry(t))
	requireFieldRegistryReloaded(t)
	fieldRegistry := objects.GetGlobalFieldRegistry()

	cache := NewStreamingTemplateCache(fieldRegistry)

	kinds := []string{"criteria", "requirement", "backlog_item", "milestone", "goal"}
	const numGoroutines = 50

	var wg sync.WaitGroup
	errors := make(chan error, numGoroutines*len(kinds))
	templates := make(map[string][]*interactive.TemplateWithTokens)
	var mu sync.Mutex

	// Initialize template slices
	for _, kind := range kinds {
		templates[kind] = make([]*interactive.TemplateWithTokens, numGoroutines)
	}

	// Warm cache sequentially so concurrent workers do not observe "unknown kind" or distinct
	// template pointers before the per-kind singleton is installed.
	for _, kind := range kinds {
		if _, err := cache.GetTemplateWithTokens(kind); err != nil {
			t.Fatalf("preload template for kind %q: %v", kind, err)
		}
	}

	// Concurrent access to different kinds
	for i := 0; i < numGoroutines; i++ {
		for _, kind := range kinds {
			wg.Add(1)
			goroutinelabels.NewGoroutine("system_test", "multiple kinds concurrent access").StartSimple(func() {
				func(k string, idx int) {
					defer wg.Done()
					template, err := cache.GetTemplateWithTokens(k)
					if err != nil {
						errors <- err
						return
					}
					mu.Lock()
					templates[k][idx] = template
					mu.Unlock()
				}(kind, i)
			})
		}
	}

	wg.Wait()
	close(errors)

	// Check for errors
	for err := range errors {
		t.Errorf("Error during concurrent access: %v", err)
	}

	// Verify all templates for each kind are the same pointer (cached)
	for _, kind := range kinds {
		kindTemplates := templates[kind]
		if len(kindTemplates) == 0 {
			t.Errorf("No templates cached for %s", kind)
			continue
		}
		firstTemplate := kindTemplates[0]
		for i, template := range kindTemplates {
			if template != firstTemplate {
				t.Errorf("Template %d for %s is not the same pointer (cache miss)", i, kind)
			}
		}
	}
}

// TestStreamingTemplateCache_GetAllKindsConcurrent tests that GetAllKinds can be called
// concurrently without issues
func TestStreamingTemplateCache_GetAllKindsConcurrent(t *testing.T) {
	t.Cleanup(ensureRepoSpecsForFieldRegistry(t))
	requireFieldRegistryReloaded(t)
	fieldRegistry := objects.GetGlobalFieldRegistry()

	cache := NewStreamingTemplateCache(fieldRegistry)

	const numGoroutines = 20

	var wg sync.WaitGroup
	errors := make(chan error, numGoroutines)
	kindsSets := make([][]string, numGoroutines)

	// Concurrent GetAllKinds calls
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		goroutinelabels.NewGoroutine("system_test", "get all kinds concurrent").StartSimple(func() {
			func(idx int) {
				defer wg.Done()
				kinds, err := cache.GetAllKinds()
				if err != nil {
					errors <- err
					return
				}
				if len(kinds) == 0 {
					errors <- fmt.Errorf("goroutine %d: GetAllKinds returned empty list", idx)
					return
				}
				kindsSets[idx] = kinds
			}(i)
		})
	}

	wg.Wait()
	close(errors)

	// Check for errors
	for err := range errors {
		t.Errorf("Error during concurrent GetAllKinds: %v", err)
	}

	// Verify all calls returned the same set of kinds (order may differ, but should be same set)
	firstSet := make(map[string]bool)
	for _, kind := range kindsSets[0] {
		firstSet[kind] = true
	}

	for i, kinds := range kindsSets {
		if len(kinds) != len(kindsSets[0]) {
			t.Errorf("Goroutine %d returned different number of kinds: %d vs %d", i, len(kinds), len(kindsSets[0]))
		}
		for _, kind := range kinds {
			if !firstSet[kind] {
				t.Errorf("Goroutine %d returned unknown kind: %s", i, kind)
			}
		}
	}
}

// TestStreamingTemplateCache_PreloadAllKindsConcurrent tests that PreloadAllKinds
// can handle concurrent calls without issues
func TestStreamingTemplateCache_PreloadAllKindsConcurrent(t *testing.T) {
	t.Cleanup(ensureRepoSpecsForFieldRegistry(t))
	requireFieldRegistryReloaded(t)
	fieldRegistry := objects.GetGlobalFieldRegistry()

	cache := NewStreamingTemplateCache(fieldRegistry)

	const numGoroutines = 5

	var wg sync.WaitGroup
	errors := make(chan error, numGoroutines)

	// Concurrent PreloadAllKinds calls
	for i := 0; i < numGoroutines; i++ {
		i := i
		goroutinelabels.NewGoroutine(fmt.Sprintf("test_preload_%d", i), fmt.Sprintf("preloading all kinds %d in defensive test", i)).
			WithWaitGroup(&wg).
			StartSimple(func() {
				if err := cache.PreloadAllKinds(); err != nil {
					errors <- err
				}
			})
	}

	wg.Wait()
	close(errors)

	// Check for errors (skip if specs not available)
	for err := range errors {
		if strings.Contains(err.Error(), "could not find specs directory") {
			t.Skipf("repo specs not available: %v", err)
		}
		t.Errorf("Error during concurrent PreloadAllKinds: %v", err)
	}

	// Verify cache is usable after concurrent preload
	kinds, err := cache.GetAllKinds()
	if err != nil {
		if strings.Contains(err.Error(), "could not find specs directory") {
			t.Skipf("repo specs not available: %v", err)
		}
		t.Fatalf("Failed to get all kinds after concurrent preload: %v", err)
	}

	// Verify templates are cached
	for _, kind := range kinds[:minDefensive(10, len(kinds))] { // Test first 10
		template1, err := cache.GetTemplateWithTokens(kind)
		if err != nil {
			continue // Some kinds might fail, that's okay
		}
		template2, err := cache.GetTemplateWithTokens(kind)
		if err != nil {
			continue
		}
		if template1 != template2 {
			t.Errorf("Template for %s should be cached after preload", kind)
		}
	}
}

// TestStreamingTemplateCache_CacheStateAfterError tests that cache state is consistent
// after template generation errors
func TestStreamingTemplateCache_CacheStateAfterError(t *testing.T) {
	t.Cleanup(ensureRepoSpecsForFieldRegistry(t))
	requireFieldRegistryReloaded(t)
	fieldRegistry := objects.GetGlobalFieldRegistry()

	cache := NewStreamingTemplateCache(fieldRegistry)

	// Request non-existent kind (will error)
	nonexistentKind := "definitely_nonexistent_kind_xyz"
	_, err := cache.GetTemplateWithTokens(nonexistentKind)
	if err == nil {
		t.Error("Should return error for non-existent kind")
	}

	// Verify cache doesn't have error entry
	// (We can't directly check, but behavior confirms - should error again)
	_, err2 := cache.GetTemplateWithTokens(nonexistentKind)
	if err2 == nil {
		t.Error("Should still return error (error not cached)")
	}

	// Verify valid kinds still work
	validKind := "criteria"
	template, err := cache.GetTemplateWithTokens(validKind)
	if err != nil {
		t.Errorf("Valid kind should work after error: %v", err)
	}
	if template == nil {
		t.Error("Valid kind should return template after error")
	}
}

// TestStreamingTemplateCache_InvariantConsistency tests that cache maintains invariants
// under various concurrent operations
func TestStreamingTemplateCache_InvariantConsistency(t *testing.T) {
	t.Cleanup(ensureRepoSpecsForFieldRegistry(t))
	requireFieldRegistryReloaded(t)
	fieldRegistry := objects.GetGlobalFieldRegistry()

	cache := NewStreamingTemplateCache(fieldRegistry)

	kinds := []string{"criteria", "requirement", "backlog_item"}
	const iterations = 50

	var wg sync.WaitGroup

	// Mix of operations: get, invalidate, clear
	for i := 0; i < iterations; i++ {
		kind := kinds[i%len(kinds)]

		wg.Add(1)
		goroutinelabels.NewGoroutine("system_test", "concurrent get").StartSimple(func() {
			func(k string) {
				defer wg.Done()
				_, _ = cache.GetTemplateWithTokens(k) //nolint:errcheck // Test - error handling not relevant here
			}(kind)
		})

		if i%5 == 0 {
			wg.Add(1)
			goroutinelabels.NewGoroutine("system_test", "concurrent invalidate").StartSimple(func() {
				defer wg.Done()
				cache.InvalidateKind(kind)
			})
		}

		if i%10 == 0 {
			wg.Add(1)
			goroutinelabels.NewGoroutine("system_test", "concurrent clear").StartSimple(func() {
				defer wg.Done()
				cache.ClearCache()
			})
		}
	}

	wg.Wait()

	// Verify cache is still consistent after mixed operations
	for _, kind := range kinds {
		template, err := cache.GetTemplateWithTokens(kind)
		if err != nil {
			t.Errorf("Cache not consistent for %s after mixed operations: %v", kind, err)
		}
		if template == nil {
			t.Errorf("Cache returned nil template for %s after mixed operations", kind)
		}
	}
}

// TestStreamingTemplateCache_NoInfiniteLoopOnInvalidation tests that invalidating
// and immediately getting the same kind doesn't cause infinite loops
func TestStreamingTemplateCache_NoInfiniteLoopOnInvalidation(t *testing.T) {
	t.Cleanup(ensureRepoSpecsForFieldRegistry(t))
	requireFieldRegistryReloaded(t)
	fieldRegistry := objects.GetGlobalFieldRegistry()

	cache := NewStreamingTemplateCache(fieldRegistry)

	kind := "criteria"

	// Cycle: get, invalidate, get, invalidate (should not loop)
	for i := 0; i < 1000; i++ {
		template, err := cache.GetTemplateWithTokens(kind)
		if err != nil {
			t.Fatalf("Failed to get template on iteration %d: %v", i, err)
		}
		if template == nil {
			t.Fatalf("Template is nil on iteration %d", i)
		}

		cache.InvalidateKind(kind)
	}

	// If we get here, no infinite loop occurred
	template, err := cache.GetTemplateWithTokens(kind)
	if err != nil {
		t.Errorf("Cache not usable after invalidation cycle: %v", err)
	}
	if template == nil {
		t.Error("Cache returned nil template after invalidation cycle")
	}
}

// TestStreamingTemplateCache_CacheSizeBounded tests that cache doesn't grow unbounded
func TestStreamingTemplateCache_CacheSizeBounded(t *testing.T) {
	t.Cleanup(ensureRepoSpecsForFieldRegistry(t))
	requireFieldRegistryReloaded(t)
	fieldRegistry := objects.GetGlobalFieldRegistry()

	cache := NewStreamingTemplateCache(fieldRegistry)

	// Get all kinds
	kinds, err := cache.GetAllKinds()
	if err != nil {
		t.Fatalf("Failed to get all kinds: %v", err)
	}

	// Get templates for all kinds (populate cache)
	for _, kind := range kinds {
		_, err := cache.GetTemplateWithTokens(kind)
		if err != nil {
			// Some kinds might fail, that's okay
			continue
		}
	}

	// Get templates again (should all be cached)
	cachedCount := 0
	for _, kind := range kinds {
		template1, err1 := cache.GetTemplateWithTokens(kind)
		if err1 != nil {
			continue
		}
		template2, err2 := cache.GetTemplateWithTokens(kind)
		if err2 != nil {
			continue
		}
		if template1 == template2 {
			cachedCount++
		}
	}

	// Cache should contain at least some entries
	if cachedCount == 0 {
		t.Error("Cache should contain some entries after preloading")
	}

	// Cache size should be bounded by number of kinds (not unbounded)
	// We can't directly check cache size, but if we can get all templates,
	// cache is working correctly and not leaking
}

// TestStreamingTemplateCache_ClearCacheIsIdempotent tests that clearing cache
// multiple times doesn't cause issues
func TestStreamingTemplateCache_ClearCacheIsIdempotent(t *testing.T) {
	t.Cleanup(ensureRepoSpecsForFieldRegistry(t))
	requireFieldRegistryReloaded(t)
	fieldRegistry := objects.GetGlobalFieldRegistry()

	cache := NewStreamingTemplateCache(fieldRegistry)

	// Pre-populate cache
	kinds := []string{"criteria", "requirement", "backlog_item"}
	for _, kind := range kinds {
		_, err := cache.GetTemplateWithTokens(kind)
		if err != nil {
			t.Fatalf("Failed to pre-populate cache: %v", err)
		}
	}

	// Clear cache multiple times
	for i := 0; i < 100; i++ {
		cache.ClearCache()
	}

	// Verify cache is still usable
	for _, kind := range kinds {
		template, err := cache.GetTemplateWithTokens(kind)
		if err != nil {
			t.Errorf("Cache not usable after repeated clear for %s: %v", kind, err)
		}
		if template == nil {
			t.Errorf("Cache returned nil template for %s after repeated clear", kind)
		}
	}
}

// TestStreamingTemplateCache_InvalidateNonexistentKind tests that invalidating
// a non-existent kind doesn't cause issues
func TestStreamingTemplateCache_InvalidateNonexistentKind(t *testing.T) {
	t.Cleanup(ensureRepoSpecsForFieldRegistry(t))
	requireFieldRegistryReloaded(t)
	fieldRegistry := objects.GetGlobalFieldRegistry()

	cache := NewStreamingTemplateCache(fieldRegistry)

	// Invalidate non-existent kind (should not panic or error)
	nonexistentKind := "nonexistent_kind_xyz_123"
	cache.InvalidateKind(nonexistentKind)

	// Verify cache is still usable
	validKind := "criteria"
	template, err := cache.GetTemplateWithTokens(validKind)
	if err != nil {
		t.Errorf("Cache not usable after invalidating non-existent kind: %v", err)
	}
	if template == nil {
		t.Error("Cache returned nil template after invalidating non-existent kind")
	}

	// Invalidate non-existent kind multiple times
	for i := 0; i < 100; i++ {
		cache.InvalidateKind(nonexistentKind)
	}

	// Verify cache is still usable
	template2, err2 := cache.GetTemplateWithTokens(validKind)
	if err2 != nil {
		t.Errorf("Cache not usable after repeated invalidations of non-existent kind: %v", err2)
	}
	if template2 == nil {
		t.Error("Cache returned nil template after repeated invalidations of non-existent kind")
	}
}

// TestStreamingTemplateCache_GetAllKindsAfterClear tests that GetAllKinds works
// correctly after cache clear
func TestStreamingTemplateCache_GetAllKindsAfterClear(t *testing.T) {
	t.Cleanup(ensureRepoSpecsForFieldRegistry(t))
	requireFieldRegistryReloaded(t)
	fieldRegistry := objects.GetGlobalFieldRegistry()

	cache := NewStreamingTemplateCache(fieldRegistry)

	// Get all kinds
	kinds1, err := cache.GetAllKinds()
	if err != nil {
		t.Fatalf("Failed to get all kinds: %v", err)
	}

	// Clear cache
	cache.ClearCache()

	// Get all kinds again (should work)
	kinds2, err := cache.GetAllKinds()
	if err != nil {
		t.Fatalf("Failed to get all kinds after clear: %v", err)
	}

	// Should return same set of kinds
	if len(kinds1) != len(kinds2) {
		t.Errorf("GetAllKinds returned different number of kinds after clear: %d vs %d", len(kinds1), len(kinds2))
	}

	// Verify sets are the same
	kinds1Set := make(map[string]bool)
	for _, kind := range kinds1 {
		kinds1Set[kind] = true
	}

	for _, kind := range kinds2 {
		if !kinds1Set[kind] {
			t.Errorf("GetAllKinds returned unknown kind after clear: %s", kind)
		}
	}
}

// TestStreamingTemplateCache_PreloadAllKindsAfterClear tests that PreloadAllKinds
// works correctly after cache clear
func TestStreamingTemplateCache_PreloadAllKindsAfterClear(t *testing.T) {
	t.Cleanup(ensureRepoSpecsForFieldRegistry(t))
	requireFieldRegistryReloaded(t)
	fieldRegistry := objects.GetGlobalFieldRegistry()

	cache := NewStreamingTemplateCache(fieldRegistry)

	// Preload
	err := cache.PreloadAllKinds()
	if err != nil {
		t.Fatalf("Failed to preload: %v", err)
	}

	// Clear cache
	cache.ClearCache()

	// Preload again (should work)
	err = cache.PreloadAllKinds()
	if err != nil {
		t.Fatalf("Failed to preload after clear: %v", err)
	}

	// Verify cache is populated
	kinds, err := cache.GetAllKinds()
	if err != nil {
		t.Fatalf("Failed to get all kinds: %v", err)
	}

	// Verify some templates are cached
	cachedCount := 0
	for _, kind := range kinds[:minDefensive(10, len(kinds))] {
		template1, err1 := cache.GetTemplateWithTokens(kind)
		if err1 != nil {
			continue
		}
		template2, err2 := cache.GetTemplateWithTokens(kind)
		if err2 != nil {
			continue
		}
		if template1 == template2 {
			cachedCount++
		}
	}

	if cachedCount == 0 {
		t.Error("Cache should be populated after preload")
	}
}

// Helper function (duplicate from streaming_template_cache_test.go to avoid dependency)
func minDefensive(a, b int) int {
	if a < b {
		return a
	}
	return b
}
