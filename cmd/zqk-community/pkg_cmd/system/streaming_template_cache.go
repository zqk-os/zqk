package system

import (
	"sync"
	"sync/atomic"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/interactive"
	"github.com/lanceman/zqk/pkg/logging"

	"github.com/lanceman/zqk/pkg/objects"
)

// cachedTemplate stores a template (without backend-specific metadata like mtime)
type cachedTemplate struct {
	template *interactive.TemplateWithTokens
}

// StreamingTemplateCache provides cached template generation
// Backend-agnostic: relies on FieldRegistry for spec information
// Cache invalidation is manual or triggered by FieldRegistry.Reload()
type StreamingTemplateCache struct {
	generator     *interactive.TemplateGenerator
	fieldRegistry *objects.FieldRegistry
	cache         atomic.Value // holds map[string]*cachedTemplate
	mu            sync.Mutex   // Protects writes to cache
}

// NewStreamingTemplateCache creates a new cached template generator
// Backend-agnostic: relies on FieldRegistry which abstracts spec loading
func NewStreamingTemplateCache(fieldRegistry *objects.FieldRegistry) *StreamingTemplateCache {
	stc := &StreamingTemplateCache{
		generator:     interactive.NewTemplateGenerator(fieldRegistry),
		fieldRegistry: fieldRegistry,
	}
	stc.cache.Store(make(map[string]*cachedTemplate))
	return stc
}

// GetTemplateWithTokens gets a template for a kind, using cache if available
// Returns cached template if available, otherwise generates and caches new template.
func (stc *StreamingTemplateCache) GetTemplateWithTokens(kind string) (*interactive.TemplateWithTokens, error) {
	c := stc.cache.Load().(map[string]*cachedTemplate)
	if cached, ok := c[kind]; ok {
		return cached.template, nil
	}

	template, err := stc.generator.GenerateTemplateWithTokens(kind)
	if err != nil {
		return nil, err
	}

	stc.mu.Lock()
	defer stc.mu.Unlock()

	c = stc.cache.Load().(map[string]*cachedTemplate)
	if cached, ok := c[kind]; ok {
		return cached.template, nil
	}

	newCache := make(map[string]*cachedTemplate, len(c)+1)
	for k, v := range c {
		newCache[k] = v
	}
	newCache[kind] = &cachedTemplate{template: template}
	stc.cache.Store(newCache)

	return template, nil
}

// InvalidateKind invalidates the cache entry for a specific kind
func (stc *StreamingTemplateCache) InvalidateKind(kind string) {
	stc.mu.Lock()
	defer stc.mu.Unlock()

	c := stc.cache.Load().(map[string]*cachedTemplate)
	if _, ok := c[kind]; !ok {
		return
	}

	newCache := make(map[string]*cachedTemplate, len(c)-1)
	for k, v := range c {
		if k != kind {
			newCache[k] = v
		}
	}
	stc.cache.Store(newCache)
}

// ClearCache clears all cached templates
func (stc *StreamingTemplateCache) ClearCache() {
	stc.mu.Lock()
	defer stc.mu.Unlock()
	stc.cache.Store(make(map[string]*cachedTemplate))
}

// GetAllKinds returns all object kinds that have specs (discovered from FieldRegistry)
// Backend-agnostic: uses FieldRegistry which abstracts spec discovery.
// Does not call FieldRegistry.Reload(): reloading clears the registry and races under
// concurrent callers (see streaming_template_cache_defensive_test). Use
// FieldRegistry.Reload() explicitly when specs on disk change.
func (stc *StreamingTemplateCache) GetAllKinds() ([]string, error) {
	if stc.fieldRegistry == nil {
		return nil, errfmt.Errorf("field registry not set")
	}

	kinds, err := stc.fieldRegistry.GetAllKinds()
	if err != nil {
		return nil, errfmt.Newf("failed to get all kinds").Wrap(err)
	}

	return kinds, nil
}

// PreloadAllKinds preloads templates for all known kinds into the cache
// This is useful for warming the cache before concurrent access
// Backend-agnostic: uses FieldRegistry which abstracts spec loading
func (stc *StreamingTemplateCache) PreloadAllKinds() error {
	kinds, err := stc.GetAllKinds()
	if err != nil {
		return errfmt.Newf("failed to get all kinds").Wrap(err)
	}

	// Preload templates (errors are logged but don't stop preloading)
	for _, kind := range kinds {
		_, err := stc.GetTemplateWithTokens(kind)
		if err != nil {
			// Log error but continue with other kinds
			logging.FluentEvent(logging.GetLogger()).Error("Failed to preload template", err).String("kind", kind).Log()
		}
	}

	return nil
}
