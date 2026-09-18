package hts

import (
	"context"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

// Codec implements the High-Throughput Semantic (HTS) compression logic.
// It leverages the Knowledge Kernel's ontologies to omit redundant data.
type Codec struct {
	store storage.ObjectStorageProvider
}

// NewCodec creates a new HTS codec.
func NewCodec(store storage.ObjectStorageProvider) *Codec {
	return &Codec{store: store}
}

// Compress removes redundant fields from an object based on its spec.
func (c *Codec) Compress(ctx context.Context, obj map[string]any) (map[string]any, error) {
	kind, _ := obj[objects.FieldKeyKind].(string)
	if kind == "" {
		return obj, nil
	}

	// For the prototype, we simply omit fields that have empty values
	// or match the most common "base" defaults.
	compressed := make(map[string]any)
	for k, v := range obj {
		if v == nil || v == "" {
			continue
		}

		// Omit default schema version
		if k == objects.FieldKeySchemaVersion && v == objects.DefaultSchemaVersion {
			continue
		}

		compressed[k] = v
	}

	return compressed, nil
}

// Decompress restores omitted fields using the local ontology.
func (c *Codec) Decompress(ctx context.Context, obj map[string]any) (map[string]any, error) {
	// In a real implementation, this would look up the object_spec
	// and fill in missing fields with their default values.
	if _, ok := obj[objects.FieldKeySchemaVersion]; !ok {
		obj[objects.FieldKeySchemaVersion] = objects.DefaultSchemaVersion
	}
	return obj, nil
}
