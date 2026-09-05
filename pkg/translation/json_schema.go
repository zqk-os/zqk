package translation

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/zqktime"
)

// JSON Schema translator (BLI-762): parses JSON Schema (definitions) and produces domain_registry + specs.

type jsonSchemaRoot struct {
	Schema      string                     `json:"$schema"`
	Definitions map[string]json.RawMessage `json:"definitions"`
	Title       string                     `json:"title"`
}

type jsonSchemaNode struct {
	Type        string         `json:"type"`
	Title       string         `json:"title"`
	Description string         `json:"description"`
	Properties  map[string]any `json:"properties"`
}

type jsonSchemaTranslator struct{}

func NewJSONSchemaTranslator() Translator {
	return &jsonSchemaTranslator{}
}

func (t *jsonSchemaTranslator) SourceFormat() string { return FormatJSONSchema }

func (t *jsonSchemaTranslator) Translate(raw []byte, opts *TranslateOptions) (*TranslateResult, error) {
	result := &TranslateResult{Objects: nil, Warnings: nil}
	if len(raw) == 0 {
		result.Objects = []TranslatedObject{}
		return result, nil
	}
	var root jsonSchemaRoot
	if err := json.Unmarshal(raw, &root); err != nil {
		result.Objects = []TranslatedObject{}
		return result, nil
	}
	if len(root.Definitions) == 0 {
		result.Objects = []TranslatedObject{}
		return result, nil
	}
	names := make([]string, 0, len(root.Definitions))
	meta := make(map[string]struct{ Title, Desc string })
	for name, def := range root.Definitions {
		name = strings.TrimSpace(name)
		if name == emptyValue {
			continue
		}
		names = append(names, name)
		var node jsonSchemaNode
		_ = json.Unmarshal(def, &node)
		meta[name] = struct{ Title, Desc string }{Title: node.Title, Desc: node.Description}
	}
	if len(names) == 0 {
		result.Objects = []TranslatedObject{}
		return result, nil
	}
	domains := make([]map[string]any, 0, len(names))
	for _, name := range names {
		entry := map[string]any{objects.FieldKeyID: name, KeyNamespace: fmt.Sprintf(NamespacePatternDomain, name)}
		if m := meta[name]; m.Title != emptyValue {
			entry[KeyTitle] = m.Title
		}
		if m := meta[name]; m.Desc != emptyValue {
			entry[KeyDescription] = m.Desc
		}
		domains = append(domains, entry)
	}
	now := zqktime.NowRFC3339UTC()
	reg := map[string]any{
		objects.FieldKeyID:            DomainRegistryPlaceholderID,
		objects.FieldKeyKind:          DomainRegistryKind,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyNamespaceID:   DefaultNamespaceID,
		KeyTitle:                      DomainRegistryTitleFromJSONSchema,
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
		if m := meta[name]; m.Title != emptyValue {
			spec[KeyTitle] = m.Title
		}
		if m := meta[name]; m.Desc != emptyValue {
			spec[KeyDescription] = m.Desc
		}
		result.Specs = append(result.Specs, spec)
	}
	return result, nil
}
