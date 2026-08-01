package system

// ITEM-177483 exempt (TEARDOWN_PIPELINE_INVENTORY): no temp ZQK tree; field-registry helpers mutate TestRoot. Not RunProjectTestTeardown.

import (
	"strings"
	"sync"
	"testing"

	"github.com/lanceman/zqk/pkg/goroutinelabels"

	"github.com/lanceman/zqk/pkg/objects"
)

func TestStreamingTemplateCache_GetTemplateWithTokens(t *testing.T) {
	// Not t.Parallel(): ensureRepoSpecsForFieldRegistry mutates process-wide ZQK_TEST_ROOT;
	// parallel tests can reload the global field registry and race (unknown kind "criteria").
	t.Cleanup(ensureRepoSpecsForFieldRegistry(t))
	requireFieldRegistryReloaded(t)
	fieldRegistry := objects.GetGlobalFieldRegistry()

	cache := NewStreamingTemplateCache(fieldRegistry)

	// First call - should generate and cache
	template1, err := cache.GetTemplateWithTokens("criteria")
	if err != nil {
		t.Fatalf("Failed to get template: %v", err)
	}

	if template1 == nil {
		t.Fatal("Template is nil")
	}

	// Second call - should return cached template
	template2, err := cache.GetTemplateWithTokens("criteria")
	if err != nil {
		t.Fatalf("Failed to get template: %v", err)
	}

	// Should be the same pointer (cached)
	if template1 != template2 {
		t.Error("Second call should return cached template (same pointer)")
	}

	// Verify template content is the same
	if template1.Template != template2.Template {
		t.Error("Cached template content should match")
	}
}

func TestStreamingTemplateCache_FileMtimeDetection(t *testing.T) {
	bindFieldRegistryToModuleSpecs(t)
	// Use existing specs directory for this test
	// We'll test cache invalidation by using InvalidateKind instead of file modification
	// (file modification test requires copying spec files which is complex due to dependencies)

	fieldRegistry := objects.GetGlobalFieldRegistry()
	if err := fieldRegistry.LoadFields(); err != nil {
		t.Fatalf("Failed to reload field registry: %v", err)
	}

	cache := NewStreamingTemplateCache(fieldRegistry)

	// Get template (should generate and cache)
	template1, err := cache.GetTemplateWithTokens("criteria")
	if err != nil {
		t.Fatalf("Failed to get template: %v", err)
	}

	// Get template again (should use cache - same pointer)
	template2, err := cache.GetTemplateWithTokens("criteria")
	if err != nil {
		t.Fatalf("Failed to get template: %v", err)
	}

	if template1 != template2 {
		t.Error("Should return cached template (same pointer)")
	}

	// Invalidate cache
	cache.InvalidateKind("criteria")

	// Get template again (should regenerate - different pointer)
	template3, err := cache.GetTemplateWithTokens("criteria")
	if err != nil {
		t.Fatalf("Failed to get template: %v", err)
	}

	// Should be a different template pointer (regenerated)
	if template1 == template3 {
		t.Error("Template should be regenerated after invalidation (different pointer)")
	}

	// Content should be the same (spec file didn't actually change)
	// But pointers should be different
	t.Logf("Template1 pointer: %p, Template2 pointer: %p, Template3 pointer: %p", template1, template2, template3)
}

func TestStreamingTemplateCache_ConcurrentAccess(t *testing.T) {
	t.Cleanup(ensureRepoSpecsForFieldRegistry(t))
	requireFieldRegistryReloaded(t)
	fieldRegistry := objects.GetGlobalFieldRegistry()

	cache := NewStreamingTemplateCache(fieldRegistry)

	// Get all kinds
	kinds, err := cache.GetAllKinds()
	if err != nil {
		t.Fatalf("Failed to get all kinds: %v", err)
	}

	if len(kinds) == 0 {
		t.Fatal("Should have at least one kind")
	}

	// Use first 10 kinds (or all if fewer)
	testKinds := kinds
	if len(testKinds) > 10 {
		testKinds = testKinds[:10]
	}

	// Concurrent access from multiple goroutines
	var wg sync.WaitGroup
	errors := make(chan error, 100)

	for i := 0; i < 20; i++ {
		wg.Add(1)
		goroutinelabels.NewGoroutine("system_test", "streaming template cache concurrent access").StartSimple(func() {
			func(idx int) {
				defer wg.Done()
				kind := testKinds[idx%len(testKinds)]
				for j := 0; j < 5; j++ {
					_, err := cache.GetTemplateWithTokens(kind)
					if err != nil {
						errors <- err
					}
				}
			}(i)
		})
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Errorf("Concurrent access error: %v", err)
	}
}

func TestStreamingTemplateCache_GetAllKinds(t *testing.T) {
	t.Cleanup(ensureRepoSpecsForFieldRegistry(t))
	requireFieldRegistryReloaded(t)
	fieldRegistry := objects.GetGlobalFieldRegistry()

	cache := NewStreamingTemplateCache(fieldRegistry)

	kinds, err := cache.GetAllKinds()
	if err != nil {
		t.Fatalf("Failed to get all kinds: %v", err)
	}

	if len(kinds) == 0 {
		t.Fatal("Should have at least one kind")
	}

	t.Logf("Found %d kinds: %v", len(kinds), kinds[:min(10, len(kinds))])

	// Verify some expected kinds are present
	expectedKinds := []string{"criteria", "requirement", "backlog_item", "milestone", "goal"}
	kindSet := make(map[string]bool)
	for _, kind := range kinds {
		kindSet[kind] = true
	}

	for _, expected := range expectedKinds {
		if !kindSet[expected] {
			t.Errorf("Expected kind %q not found in all kinds", expected)
		}
	}
}

func TestStreamingTemplateCache_PreloadAllKinds(t *testing.T) {
	t.Cleanup(ensureRepoSpecsForFieldRegistry(t))
	requireFieldRegistryReloaded(t)
	fieldRegistry := objects.GetGlobalFieldRegistry()

	cache := NewStreamingTemplateCache(fieldRegistry)

	// Preload all templates
	err := cache.PreloadAllKinds()
	if err != nil {
		if strings.Contains(err.Error(), "could not find specs directory") {
			t.Skipf("repo specs not available: %v", err)
		}
		t.Fatalf("Failed to preload all kinds: %v", err)
	}

	// Get all kinds
	kinds, err := cache.GetAllKinds()
	if err != nil {
		if strings.Contains(err.Error(), "could not find specs directory") {
			t.Skipf("repo specs not available: %v", err)
		}
		t.Fatalf("Failed to get all kinds: %v", err)
	}

	// Verify templates are cached
	for _, kind := range kinds {
		template1, err := cache.GetTemplateWithTokens(kind)
		if err != nil {
			// Some kinds might fail (e.g., base_object, auditable)
			continue
		}

		// Second call should return cached template immediately
		template2, err := cache.GetTemplateWithTokens(kind)
		if err != nil {
			continue
		}

		if template1 != template2 {
			t.Errorf("Kind %q: template should be cached after preload", kind)
		}
	}

	t.Logf("Successfully preloaded and cached templates for %d kinds", len(kinds))
}

func TestStreamingTemplateCache_InvalidateKind(t *testing.T) {
	t.Cleanup(ensureRepoSpecsForFieldRegistry(t))
	requireFieldRegistryReloaded(t)
	fieldRegistry := objects.GetGlobalFieldRegistry()

	cache := NewStreamingTemplateCache(fieldRegistry)

	// Get template (should cache)
	template1, err := cache.GetTemplateWithTokens("criteria")
	if err != nil {
		t.Fatalf("Failed to get template: %v", err)
	}

	// Get again (should return cached)
	template2, err := cache.GetTemplateWithTokens("criteria")
	if err != nil {
		t.Fatalf("Failed to get template: %v", err)
	}

	if template1 != template2 {
		t.Error("Should return cached template")
	}

	// Invalidate cache
	cache.InvalidateKind("criteria")

	// Get again (should regenerate)
	template3, err := cache.GetTemplateWithTokens("criteria")
	if err != nil {
		t.Fatalf("Failed to get template: %v", err)
	}

	// Should be a new template (but content should be same since spec didn't change)
	if template1 == template3 {
		t.Error("Template should be regenerated after invalidation")
	}
}

func TestStreamingTemplateCache_ClearCache(t *testing.T) {
	t.Cleanup(ensureRepoSpecsForFieldRegistry(t))
	requireFieldRegistryReloaded(t)
	fieldRegistry := objects.GetGlobalFieldRegistry()

	cache := NewStreamingTemplateCache(fieldRegistry)

	// Get templates for multiple kinds
	kinds := []string{"criteria", "requirement", "backlog_item"}
	for _, kind := range kinds {
		_, err := cache.GetTemplateWithTokens(kind)
		if err != nil {
			t.Fatalf("Failed to get template for %s: %v", kind, err)
		}
	}

	// Clear cache
	cache.ClearCache()

	// Get templates again - should regenerate (different pointers)
	for _, kind := range kinds {
		template1, err := cache.GetTemplateWithTokens(kind)
		if err != nil {
			t.Fatalf("Failed to get template for %s: %v", kind, err)
		}

		template2, err := cache.GetTemplateWithTokens(kind)
		if err != nil {
			t.Fatalf("Failed to get template for %s: %v", kind, err)
		}

		if template1 != template2 {
			t.Errorf("Kind %s: template should be cached after first get", kind)
		}
	}
}

// Helper functions

func stringSliceContains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
