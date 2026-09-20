package translation

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/zqktime"
)

// Cypher translator (BLI-761): parses Cypher schema (CREATE CONSTRAINT, CREATE (n:Label))
// and produces domain_registry + object_spec-like output for traceability.

var (
	cypherLabelInConstraintRe = regexp.MustCompile(`(?i)FOR\s*\(\s*\w+\s*:\s*([A-Za-z][A-Za-z0-9_]*)`)
	cypherLabelInCreateRe     = regexp.MustCompile(`(?i)CREATE\s*\(\s*\w*\s*:\s*([A-Za-z][A-Za-z0-9_]*)`)
)

type cypherTranslator struct{}

// NewCypherTranslator returns a translator for Cypher schema (BLI-761).
func NewCypherTranslator() Translator {
	return &cypherTranslator{}
}

func (t *cypherTranslator) SourceFormat() string { return FormatCypher }

func (t *cypherTranslator) Translate(raw []byte, opts *TranslateOptions) (*TranslateResult, error) {
	result := &TranslateResult{Objects: nil, Warnings: nil}
	if len(raw) == 0 {
		result.Objects = []TranslatedObject{}
		return result, nil
	}
	text := string(raw)
	seen := make(map[string]bool)
	for _, re := range []*regexp.Regexp{cypherLabelInConstraintRe, cypherLabelInCreateRe} {
		for _, m := range re.FindAllStringSubmatch(text, -1) {
			if len(m) >= 2 {
				label := strings.TrimSpace(m[1])
				if label != emptyValue {
					seen[label] = true
				}
			}
		}
	}
	if len(seen) == 0 {
		result.Objects = []TranslatedObject{}
		return result, nil
	}
	domains := make([]map[string]any, 0, len(seen))
	labels := make([]string, 0, len(seen))
	for k := range seen {
		labels = append(labels, k)
	}
	for _, name := range labels {
		domains = append(domains, map[string]any{
			objects.FieldKeyID: name,
			KeyNamespace:       fmt.Sprintf(NamespacePatternDomain, name),
		})
	}
	now := zqktime.NowRFC3339UTC()
	reg := map[string]any{
		objects.FieldKeyID:            DomainRegistryPlaceholderID,
		objects.FieldKeyKind:          DomainRegistryKind,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyNamespaceID:   DefaultNamespaceID,
		KeyTitle:                      DomainRegistryTitleFromCypher,
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
	result.Specs = make([]map[string]any, 0, len(labels))
	for _, name := range labels {
		result.Specs = append(result.Specs, map[string]any{
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			KeyOntology:                   name,
			KeyExtends:                    DefaultExtends,
			KeyVisibility:                 DefaultVisibility,
			KeyTraits:                     []string{DefaultTrait},
			KeyFields:                     map[string]any{},
		})
	}
	return result, nil
}
