package storage

import (
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/validation"
)

func init() {
	// Wire up the synonym provider to use IDPrefixesConfig
	// This happens at package init time to ensure the resolver has access to config-based synonyms
	resolver := objects.GetGlobalSynonymResolver()
	config := validation.GetGlobalIDPrefixesConfig()
	if config != nil {
		provider := validation.NewSynonymProviderAdapter(config)
		resolver.SetSynonymProvider(provider)
	}
}

// StorageSynonymLoader implements objects.SynonymLoader using ObjectStorageProvider
type StorageSynonymLoader struct {
	storageProvider ObjectStorageProvider
}

// NewStorageSynonymLoader creates a new storage-based synonym loader
func NewStorageSynonymLoader(provider ObjectStorageProvider) *StorageSynonymLoader {
	return &StorageSynonymLoader{
		storageProvider: provider,
	}
}

// LoadSynonyms loads all kind_synonym objects from storage
func (ssl *StorageSynonymLoader) LoadSynonyms() ([]objects.SynonymData, error) {
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.GetStorageContext()

	// Use system security context if none provided (for initialization scenarios)
	if secCtx == nil {
		secCtx = pkgctx.NewSystemSecurityContext()
	}

	// Use standard List helper with 5-second timeout
	config := DefaultOperationConfig()
	config.Timeout = 5 * time.Second

	result, err := ListWithConfig(ssl.storageProvider, config, secCtx, storageCtx, ListFilter{
		Kind: objects.KindSynonym,
	})

	if err != nil {
		return nil, err
	}

	if result == nil || len(result.Objects) == 0 {
		return []objects.SynonymData{}, nil
	}

	synonyms := make([]objects.SynonymData, 0, len(result.Objects))

	for _, obj := range result.Objects {
		// Read target_kind (the kind this synonym maps to), not the object's own kind
		targetKind, _ := obj[objects.FieldKeyTargetKind].(string)
		// Fall back to "kind" for backward compatibility with old objects
		if targetKind == emptyValue {
			targetKind, _ = obj[objects.FieldKeyKind].(string)
		}
		synonym, _ := obj[objects.FieldKeySynonym].(string)
		priority := 0

		switch v := obj[objects.FieldKeyPriority].(type) {
		case int:
			priority = v
		case float64:
			priority = int(v)
		}

		if targetKind != emptyValue && synonym != emptyValue {
			synonyms = append(synonyms, objects.SynonymData{
				Kind:     targetKind,
				Synonym:  synonym,
				Priority: priority,
			})
		}
	}

	return synonyms, nil
}
