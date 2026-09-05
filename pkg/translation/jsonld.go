package translation

import (
	"encoding/json"
	"strings"
)

const (
	jsonLDKeyGraph       = "@graph"
	jsonLDKeyContext     = "@context"
	jsonLDKeyID          = "@id"
	jsonLDKeyType        = "@type"
	jsonLDTypeOWLClass   = "owl:Class"
	jsonLDTypeClass      = "Class"
	jsonLDKeyRDFSLabel   = "rdfs:label"
	jsonLDKeyLabel       = "label"
	jsonLDKeyRDFSComment = "rdfs:comment"
	jsonLDKeyComment     = "comment"
	jsonLDKeySubClassOf  = "rdfs:subClassOf"
	jsonLDKeyValue       = "@value"
)

// parseJSONLDClasses extracts owl:Class-like nodes from JSON-LD and returns TurtleClass
// for domain_registry translation. Handles @graph and single-object forms.
// CRIT-7642: JSON-LD parsing so ontology import works for .jsonld files.
func parseJSONLDClasses(raw []byte) ([]TurtleClass, error) {
	var top map[string]any
	if err := json.Unmarshal(raw, &top); err != nil {
		return nil, err
	}
	var nodes []map[string]any
	if g, ok := top[jsonLDKeyGraph]; ok {
		if arr, ok := g.([]any); ok {
			for _, item := range arr {
				if m, ok := item.(map[string]any); ok {
					nodes = append(nodes, m)
				}
			}
		}
	} else if _, hasContext := top[jsonLDKeyContext]; hasContext || len(top) > 0 {
		nodes = []map[string]any{top}
	}
	if len(nodes) == 0 {
		return nil, nil
	}
	var classes []TurtleClass
	seen := make(map[string]bool)
	for _, n := range nodes {
		if !isOWLClassNode(n) {
			continue
		}
		id, _ := n[jsonLDKeyID].(string)
		if id == emptyValue {
			continue
		}
		localName := localNameFromJSONLDID(id)
		if localName == emptyValue || seen[localName] {
			continue
		}
		seen[localName] = true
		tc := TurtleClass{
			LocalName:  localName,
			Label:      getString(n, jsonLDKeyRDFSLabel, jsonLDKeyLabel),
			Comment:    getString(n, jsonLDKeyRDFSComment, jsonLDKeyComment),
			SubClassOf: localNameFromJSONLDIDRef(getSubClassOfRef(n)),
		}
		classes = append(classes, tc)
	}
	return classes, nil
}

func isOWLClassNode(n map[string]any) bool {
	typ := n[jsonLDKeyType]
	if typ == nil {
		return false
	}
	switch t := typ.(type) {
	case string:
		return strings.Contains(t, jsonLDTypeClass) || t == jsonLDTypeOWLClass
	case []any:
		for _, v := range t {
			if s, ok := v.(string); ok && (strings.Contains(s, jsonLDTypeClass) || s == jsonLDTypeOWLClass) {
				return true
			}
		}
	}
	return false
}

func getString(n map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := n[k]; ok {
			if s, ok := v.(string); ok {
				return s
			}
			if arr, ok := v.([]any); ok && len(arr) > 0 {
				switch v := arr[0].(type) {
				case string:
					return v
				case map[string]any:
					if lang, ok := v[jsonLDKeyValue].(string); ok {
						return lang
					}
				}
			}

		}
	}
	return emptyValue
}

func getSubClassOfRef(n map[string]any) string {
	v, ok := n[jsonLDKeySubClassOf]
	if !ok {
		return emptyValue
	}
	switch v := v.(type) {
	case string:
		return v
	case map[string]any:
		if id, ok := v[jsonLDKeyID].(string); ok {
			return id
		}
	}

	if arr, ok := v.([]any); ok && len(arr) > 0 {
		switch v := arr[0].(type) {
		case string:
			return v
		case map[string]any:
			if id, ok := v[jsonLDKeyID].(string); ok {
				return id
			}
		}
	}

	return emptyValue
}

// localNameFromJSONLDID derives a slug from @id (URI, prefix:LocalName, or #LocalName).
func localNameFromJSONLDID(id string) string {
	id = strings.TrimSpace(id)
	if id == emptyValue {
		return emptyValue
	}
	if idx := strings.LastIndex(id, "#"); idx >= 0 && idx < len(id)-1 {
		return strings.ToLower(id[idx+1:])
	}
	if idx := strings.LastIndex(id, "/"); idx >= 0 && idx < len(id)-1 {
		return strings.ToLower(id[idx+1:])
	}
	if idx := strings.LastIndex(id, ":"); idx >= 0 && idx < len(id)-1 {
		return strings.ToLower(id[idx+1:])
	}
	return strings.ToLower(id)
}

func localNameFromJSONLDIDRef(ref string) string {
	if ref == emptyValue {
		return emptyValue
	}
	return localNameFromJSONLDID(ref)
}
