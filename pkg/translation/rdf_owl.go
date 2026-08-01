package translation

import (
	"fmt"
	"strings"

	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/zqktime"
)

// NewRDFOWLTranslator returns a translator for RDF/OWL (Turtle, RDF/XML, JSON-LD) input.
// CRIT-7642: at least one format translator implemented and testable.
// Parses Turtle or JSON-LD for owl:Class and rdfs terms; produces domain_registry with domains list.
type rdfOwlTranslator struct {
	formatKey string
}

// NewRDFOWLTranslator creates a translator for rdf_owl (and turtle/RDF/XML/JSON-LD) formats.
func NewRDFOWLTranslator() Translator {
	return &rdfOwlTranslator{formatKey: FormatRDFOWL}
}

// SourceFormat implements Translator.
func (t *rdfOwlTranslator) SourceFormat() string {
	return t.formatKey
}

// Translate implements Translator.
// Parses Turtle or JSON-LD for owl:Class and rdfs:label/comment/subClassOf; produces one domain_registry.
// ID is left as placeholder DOMAIN-REG-000 so the import pipeline can assign next DOMAIN-REG-NNN.
func (t *rdfOwlTranslator) Translate(raw []byte, opts *TranslateOptions) (*TranslateResult, error) {
	result := &TranslateResult{
		Objects:  nil,
		Warnings: nil,
	}
	if len(raw) == 0 {
		result.Objects = []TranslatedObject{}
		return result, nil
	}

	text := strings.TrimSpace(string(raw))
	var classes []TurtleClass

	// Try JSON-LD first when content looks like JSON (CRIT-7642: JSON-LD parsing for .jsonld files).
	if strings.HasPrefix(text, JSONPrefixObject) || strings.HasPrefix(text, JSONPrefixArray) {
		if jsonClasses, err := parseJSONLDClasses(raw); err == nil && len(jsonClasses) > 0 {
			classes = jsonClasses
		}
	}
	// Fall back to Turtle when not JSON-LD or no classes found in JSON-LD.
	if len(classes) == 0 && (strings.Contains(text, TurtleMarkerClass) || strings.Contains(text, TurtleMarkerLabel)) {
		classes = parseTurtleClasses(raw)
	}
	// ITEM-712: try RDF/XML when content looks like XML and Turtle/JSON-LD did not produce classes.
	if len(classes) == 0 && (strings.Contains(text, RDFXMLMarkerClass) || strings.Contains(text, RDFXMLMarkerRoot)) {
		classes = parseRDFXMLClasses(raw)
	}
	if len(classes) == 0 {
		result.Objects = []TranslatedObject{}
		return result, nil
	}

	// Build domains list for domain_registry: id, namespace, optional title/description/sub_class_of.
	domains := make([]map[string]any, 0, len(classes))
	for _, c := range classes {
		slug := c.LocalName
		if slug == emptyValue {
			continue
		}
		entry := map[string]any{
			objects.FieldKeyID: slug,
			KeyNamespace:       fmt.Sprintf(NamespacePatternDomain, slug),
		}
		if c.Label != emptyValue {
			entry[KeyTitle] = c.Label
		}
		if c.Comment != emptyValue {
			entry[KeyDescription] = c.Comment
		}
		if c.SubClassOf != emptyValue {
			entry[KeySubClassOf] = c.SubClassOf
		}
		domains = append(domains, entry)
	}
	if len(domains) == 0 {
		result.Objects = []TranslatedObject{}
		return result, nil
	}

	now := zqktime.NowRFC3339UTC()
	// Single domain_registry object; id placeholder for pipeline to replace with next DOMAIN-REG-NNN.
	reg := map[string]any{
		objects.FieldKeyID:            DomainRegistryPlaceholderID,
		objects.FieldKeyKind:          DomainRegistryKind,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyNamespaceID:   DefaultNamespaceID,
		KeyTitle:                      DomainRegistryTitleFromOntology,
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

	// Object-spec-like output: one spec-like map per class for downstream spec generation.
	result.Specs = make([]map[string]any, 0, len(classes))
	for _, c := range classes {
		if c.LocalName == emptyValue {
			continue
		}
		extends := DefaultExtends
		if c.SubClassOf != emptyValue {
			extends = c.SubClassOf
		}
		spec := map[string]any{
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			KeyOntology:                   c.LocalName,
			KeyExtends:                    extends,
			KeyVisibility:                 DefaultVisibility,
			KeyDescription:                c.Comment,
			KeyTraits:                     []string{DefaultTrait},
			KeyFields:                     map[string]any{},
		}
		if c.Label != emptyValue {
			spec[KeyTitle] = c.Label
		}
		result.Specs = append(result.Specs, spec)
	}

	return result, nil
}
