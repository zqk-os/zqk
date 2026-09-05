package validation

// SynonymProviderAdapter adapts IDPrefixesConfig to the objects.SynonymProvider interface
// This allows KindSynonymResolver to use config-based synonyms without creating an import cycle
type SynonymProviderAdapter struct {
	config *IDPrefixesConfig
}

// NewSynonymProviderAdapter creates a new adapter from an IDPrefixesConfig
func NewSynonymProviderAdapter(config *IDPrefixesConfig) *SynonymProviderAdapter {
	return &SynonymProviderAdapter{config: config}
}

// GetSynonymsForKind returns the synonyms for a given kind from the config
func (a *SynonymProviderAdapter) GetSynonymsForKind(kind string) []string {
	if a.config == nil {
		return []string{}
	}
	return a.config.GetSynonymsForKind(kind)
}
