package translator

import (
	"encoding/json"
	"errors"
)

var (
	ErrManifestMismatch = errors.New("target class does not match manifest")
)

// TranslationManifest defines how to map external data to an internal class.
type TranslationManifest struct {
	SourceFormat string            `json:"source_format"`
	TargetClass  string            `json:"target_class"`
	Mappings     map[string]string `json:"mappings"` // map[externalKey]internalProperty
}

// ManifestDrivenTranslator implements Translator using a TranslationManifest.
type ManifestDrivenTranslator struct {
	manifest TranslationManifest
}

// NewManifestTranslator creates a translator powered by a declarative manifest.
func NewManifestTranslator(m TranslationManifest) Translator {
	return &ManifestDrivenTranslator{
		manifest: m,
	}
}

func (m *ManifestDrivenTranslator) SupportedFormat() string {
	return m.manifest.SourceFormat
}

func (m *ManifestDrivenTranslator) TranslateToInternal(className string, externalData []byte) (map[string]any, error) {
	if className != m.manifest.TargetClass {
		return nil, ErrManifestMismatch
	}

	var parsedData map[string]any
	if err := json.Unmarshal(externalData, &parsedData); err != nil {
		return nil, err
	}

	internalData := make(map[string]any)
	for extKey, intKey := range m.manifest.Mappings {
		if val, exists := parsedData[extKey]; exists {
			internalData[intKey] = val
		}
	}

	return internalData, nil
}

func (m *ManifestDrivenTranslator) TranslateToExternal(className string, internalData map[string]any) ([]byte, error) {
	if className != m.manifest.TargetClass {
		return nil, ErrManifestMismatch
	}

	externalData := make(map[string]any)
	for extKey, intKey := range m.manifest.Mappings {
		if val, exists := internalData[intKey]; exists {
			externalData[extKey] = val
		}
	}

	return json.Marshal(externalData)
}
