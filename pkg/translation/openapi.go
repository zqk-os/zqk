package translation

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/zqktime"
	"gopkg.in/yaml.v3"
)

// OpenAPI translator (ITEM-763): parses OpenAPI 3.x / Swagger 2 with components.schemas or definitions
// and produces domain_registry + object_spec-like output.

type openapiRoot struct {
	OpenAPI    string `yaml:"openapi"`
	Swagger    string `yaml:"swagger"`
	Components struct {
		Schemas map[string]openapiSchemaNode `yaml:"schemas"`
	} `yaml:"components"`
	Definitions map[string]openapiSchemaNode `yaml:"definitions"`
}

type openapiSchemaNode struct {
	Type        string         `yaml:"type"`
	Title       string         `yaml:"title"`
	Description string         `yaml:"description"`
	Properties  map[string]any `yaml:"properties"`
}

// openapiTranslator implements Translator for OpenAPI/Swagger files.
type openapiTranslator struct{}

// NewOpenAPITranslator returns a translator for OpenAPI/Swagger (ITEM-763).
func NewOpenAPITranslator() Translator {
	return &openapiTranslator{}
}

// SourceFormat implements Translator.
func (t *openapiTranslator) SourceFormat() string {
	return FormatOpenAPI
}

// Translate implements Translator.
// Extracts components.schemas or definitions and produces one domain_registry + specs.
func (t *openapiTranslator) Translate(raw []byte, opts *TranslateOptions) (*TranslateResult, error) {
	result := &TranslateResult{Objects: nil, Warnings: nil}
	if len(raw) == 0 {
		result.Objects = []TranslatedObject{}
		return result, nil
	}

	var root openapiRoot
	trimmed := strings.TrimSpace(string(raw))
	if strings.HasPrefix(trimmed, JSONPrefixObject) {
		if err := json.Unmarshal(raw, &root); err != nil {
			result.Objects = []TranslatedObject{}
			return result, nil
		}
	} else {
		if err := yaml.Unmarshal(raw, &root); err != nil {
			result.Objects = []TranslatedObject{}
			return result, nil
		}
	}

	schemas := root.Components.Schemas
	if len(schemas) == 0 {
		schemas = root.Definitions
	}
	if len(schemas) == 0 {
		result.Objects = []TranslatedObject{}
		return result, nil
	}

	names := make([]string, 0, len(schemas))
	meta := make(map[string]struct{ title, desc string })
	for name, node := range schemas {
		name = strings.TrimSpace(name)
		if name == emptyValue {
			continue
		}
		names = append(names, name)
		if node.Title != emptyValue {
			meta[name] = struct{ title, desc string }{title: node.Title, desc: node.Description}
		}
	}
	if len(names) == 0 {
		result.Objects = []TranslatedObject{}
		return result, nil
	}

	domains := make([]map[string]any, 0, len(names))
	for _, name := range names {
		entry := map[string]any{objects.FieldKeyID: name, KeyNamespace: fmt.Sprintf(NamespacePatternDomain, name)}
		if m, ok := meta[name]; ok {
			if m.title != emptyValue {
				entry[KeyTitle] = m.title
			}
			if m.desc != emptyValue {
				entry[KeyDescription] = m.desc
			}
		}
		domains = append(domains, entry)
	}

	now := zqktime.NowRFC3339UTC()
	reg := map[string]any{
		objects.FieldKeyID:            DomainRegistryPlaceholderID,
		objects.FieldKeyKind:          DomainRegistryKind,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyNamespaceID:   DefaultNamespaceID,
		KeyTitle:                      DomainRegistryTitleFromOpenAPI,
		KeyDomains:                    domains,
		objects.FieldKeyStatus:        DefaultStatus,
		objects.FieldKeyCreatedAt:     now,
		objects.FieldKeyUpdatedAt:     now,
		objects.FieldKeyCreatedBy:     CreatedBySystem,
		objects.FieldKeyUpdatedBy:     UpdatedBySystem,
	}
	result.Objects = []TranslatedObject{
		{Kind: DomainRegistryKind, Data: reg},
	}

	result.Specs = make([]map[string]any, 0, len(names))
	for _, name := range names {
		spec := map[string]any{
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			KeyOntology:                   name,
			KeyExtends:                    DefaultExtends,
			KeyVisibility:                 DefaultVisibility,
			KeyTraits:                     []string{DefaultTrait},
			KeyFields:                     map[string]any{},
		}
		if m, ok := meta[name]; ok {
			if m.title != emptyValue {
				spec[KeyTitle] = m.title
			}
			if m.desc != emptyValue {
				spec[KeyDescription] = m.desc
			}
		}
		result.Specs = append(result.Specs, spec)
	}
	return result, nil
}
