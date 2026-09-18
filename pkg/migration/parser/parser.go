package parser

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

const emptyValue = ""

// ParsedObject represents a parsed YAML object
type ParsedObject struct {
	ID         string
	Kind       string
	Title      string
	Status     string
	Properties map[string]any
	Content    string // Original YAML content
	FilePath   string
}

// YAMLParser parses YAML files and extracts object properties
type YAMLParser struct {
	referenceFieldSuffixes []string
}

// NewYAMLParser creates a new YAML parser
func NewYAMLParser() *YAMLParser {
	return &YAMLParser{
		referenceFieldSuffixes: []string{
			"_ref",
			"_refs",
		},
	}
}

// ParseFile parses a YAML file and returns a ParsedObject
func (p *YAMLParser) ParseFile(filePath string) (*ParsedObject, error) {
	// Read file content
	content, err := fileutil.ReadFile(filePath)
	if err != nil {
		return nil, errfmt.Newf("failed to read file").Wrap(err)
	}

	// Parse YAML
	var data map[string]any
	if err := yaml.Unmarshal(content, &data); err != nil {
		return nil, errfmt.Newf("failed to parse YAML").Wrap(err)
	}

	// Extract base object fields
	obj := &ParsedObject{
		ID:         getString(data, "id"),
		Kind:       getString(data, "kind"),
		Title:      getString(data, "title"),
		Status:     getString(data, "status"),
		Properties: data,
		Content:    string(content),
		FilePath:   filePath,
	}

	return obj, nil
}

// ParseBytes parses YAML content from bytes and returns a ParsedObject
func (p *YAMLParser) ParseBytes(content []byte) (*ParsedObject, error) {
	// Parse YAML
	var data map[string]any
	if err := yaml.Unmarshal(content, &data); err != nil {
		return nil, errfmt.Newf("failed to parse YAML").Wrap(err)
	}

	// Extract base object fields
	obj := &ParsedObject{
		ID:         getString(data, "id"),
		Kind:       getString(data, "kind"),
		Title:      getString(data, "title"),
		Status:     getString(data, "status"),
		Properties: data,
		Content:    string(content),
		FilePath:   "", // Not available when parsing from bytes
	}

	return obj, nil
}

// ExtractReferenceFields extracts all reference fields from properties (recursively)
func (p *YAMLParser) ExtractReferenceFields(properties map[string]any) map[string]any {
	refs := make(map[string]any)
	p.extractReferenceFieldsRecursive(properties, refs, "")
	return refs
}

func (p *YAMLParser) extractReferenceFieldsRecursive(properties map[string]any, refs map[string]any, prefix string) {
	for key, value := range properties {
		fullKey := key
		if prefix != "" {
			fullKey = prefix + "." + key
		}

		// Check if field name ends with reference suffix
		isRef := false
		for _, suffix := range p.referenceFieldSuffixes {
			if strings.HasSuffix(key, suffix) {
				isRef = true
				break
			}
		}
		if isRef && objects.IsLeftoverNonKernelRefField(key) {
			isRef = false
		}

		if isRef {
			refs[fullKey] = value
			continue
		}

		// Recursively check maps and slices
		switch v := value.(type) {
		case map[string]any:
			p.extractReferenceFieldsRecursive(v, refs, fullKey)
		case map[interface{}]interface{}:
			// Convert to map[string]any if possible
			m := make(map[string]any)
			for mk, mv := range v {
				if mks, ok := mk.(string); ok {
					m[mks] = mv
				}
			}
			p.extractReferenceFieldsRecursive(m, refs, fullKey)
		case []any:
			for i, item := range v {
				if m, ok := item.(map[string]any); ok {
					p.extractReferenceFieldsRecursive(m, refs, fmt.Sprintf("%s[%d]", fullKey, i))
				} else if m, ok := item.(map[interface{}]interface{}); ok {
					ms := make(map[string]any)
					for mk, mv := range m {
						if mks, ok := mk.(string); ok {
							ms[mks] = mv
						}
					}
					p.extractReferenceFieldsRecursive(ms, refs, fmt.Sprintf("%s[%d]", fullKey, i))
				}
			}
		}
	}
}

// ExtractEmbeddingText extracts text for embedding generation
func (p *YAMLParser) ExtractEmbeddingText(obj *ParsedObject) string {
	var parts []string

	// Add title
	if obj.Title != emptyValue {
		parts = append(parts, obj.Title)
	}

	// Add context
	if context, ok := obj.Properties[objects.FieldKeyContext].(string); ok && context != emptyValue {
		parts = append(parts, context)
	}

	// Add description
	if desc, ok := obj.Properties[objects.FieldKeyDescription].(string); ok && desc != emptyValue {
		parts = append(parts, desc)
	}

	return strings.Join(parts, " ")
}

// getString safely extracts a string value from a map
func getString(data map[string]any, key string) string {
	if val, ok := data[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}
