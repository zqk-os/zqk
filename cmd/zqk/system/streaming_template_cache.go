package system

import (
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/interactive"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/stampmemo"
)

// StreamingTemplateCache memos generated templates by kind (closed spec set).
// Stamp is FieldRegistry.CacheStamp(); InvalidateKind/ClearCache remain for
// out-of-band drops when SpecCacheRevision has not moved.
type StreamingTemplateCache struct {
	generator     *interactive.TemplateGenerator
	fieldRegistry *objects.FieldRegistry
	cache         stampmemo.Table[*interactive.TemplateWithTokens]
}

// NewStreamingTemplateCache creates a new cached template generator
// Backend-agnostic: relies on FieldRegistry which abstracts spec loading
func NewStreamingTemplateCache(fieldRegistry *objects.FieldRegistry) *StreamingTemplateCache {
	return &StreamingTemplateCache{
		generator:     interactive.NewTemplateGenerator(fieldRegistry),
		fieldRegistry: fieldRegistry,
	}
}

func (stc *StreamingTemplateCache) kindStamp() stampmemo.Stamp {
	if stc.fieldRegistry == nil {
		return 0
	}
	return stc.fieldRegistry.CacheStamp()
}

// GetTemplateWithTokens gets a template for a kind, using cache if available
// Returns cached template if available, otherwise generates and caches new template.
func (stc *StreamingTemplateCache) GetTemplateWithTokens(kind string) (*interactive.TemplateWithTokens, error) {
	return stc.cache.Load(kind, stc.kindStamp(), func() (*interactive.TemplateWithTokens, error) {
		return stc.generator.GenerateTemplateWithTokens(kind)
	})
}

// InvalidateKind invalidates the cache entry for a specific kind
func (stc *StreamingTemplateCache) InvalidateKind(kind string) {
	stc.cache.Delete(kind)
}

// ClearCache clears all cached templates
func (stc *StreamingTemplateCache) ClearCache() {
	stc.cache.Reset()
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
