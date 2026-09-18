package registry

import (
	"encoding/json"
	"fmt"

	"sort"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/specbuilder/builders"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// FieldRegistry maps unique profile codes to field metadata, including allocated ID.
type FieldRegistry struct {
	Fields map[string]FieldMetadata `json:"fields"`
}

// FieldMetadata captures field information for registry persistence.
type FieldMetadata struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Type        string `json:"type"`
	ProfileCode string `json:"profile_code"`
	Description string `json:"description"`
}

// GenerateRegistry iterates over all registered builders and generates a FieldRegistry.
func GenerateRegistry() (*FieldRegistry, error) {
	registry := &FieldRegistry{
		Fields: make(map[string]FieldMetadata),
	}

	reg := builders.GetGlobalRegistry()
	ontologies := reg.GetAllOntologies()
	sort.Strings(ontologies)

	currentID := 1
	for _, ontology := range ontologies {
		version, err := reg.GetLatestVersion(ontology)
		if err != nil {
			continue
		}
		builder, err := reg.GetBuilder(ontology, version)
		if err != nil {
			continue
		}

		spec := builder.Build()

		// Sort field keys to ensure deterministic order
		keys := make([]string, 0, len(spec.Fields))
		for k := range spec.Fields {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		for _, k := range keys {
			v := spec.Fields[k]
			fieldMap, ok := v.(map[string]any)
			if !ok {
				continue
			}
			profileCode, _ := fieldMap["field_profile_code"].(string)
			if profileCode == "" {
				continue
			}

			desc, _ := fieldMap[objects.FieldKeyDescription].(string)
			if desc == "" {
				if cl, ok := fieldMap["checklist"].(map[string]any); ok {
					desc, _ = cl[objects.FieldKeyPurpose].(string)
				}
			}

			// Assign ID if not already present
			if _, exists := registry.Fields[profileCode]; !exists {
				registry.Fields[profileCode] = FieldMetadata{
					ID:          currentID,
					Name:        k,
					Type:        fmt.Sprintf("%v", fieldMap[objects.FieldKeyType]),
					ProfileCode: profileCode,
					Description: desc,
				}
				currentID++
			}
		}
	}
	return registry, nil
}

// SaveRegistry writes the registry to the specified file path.
func SaveRegistry(path string, registry *FieldRegistry) error {
	data, err := json.MarshalIndent(registry, "", "  ")
	if err != nil {
		return err
	}
	return fileutil.WriteSecureFile(path, data)
}
