package mcp

import (
	"fmt"
	"net/url"
	"strings"
	"sync/atomic"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/objects"
)

var (
	jsonldOntologyNamespace atomic.Pointer[string]
)

func init() {
	defaultNS := "https://zqk.dev/ontology/"
	jsonldOntologyNamespace.Store(&defaultNS)
}

// SetJSONLDOntologyNamespace sets the base URI for the ZQK JSON-LD ontology.
// Returns an error if uri is empty, cannot be parsed as a valid URI, or does not have an http/https scheme.
func SetJSONLDOntologyNamespace(uri string) error {
	if uri == "" {
		return errfmt.Errorf("ontology namespace URI cannot be empty")
	}
	parsed, err := url.ParseRequestURI(uri)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return errfmt.Errorf("invalid ontology namespace URI '%s': must be a valid http or https URL", uri)
	}
	jsonldOntologyNamespace.Store(&uri)
	return nil
}

// GetJSONLDOntologyNamespace returns the base URI for the ZQK JSON-LD ontology.
func GetJSONLDOntologyNamespace() string {
	if ptr := jsonldOntologyNamespace.Load(); ptr != nil {
		return *ptr
	}
	return "https://zqk.dev/ontology/"
}

// getJSONLDContext returns the standard JSON-LD context for ZQK schemas
func getJSONLDContext() map[string]any {
	return map[string]any{
		"zqk":    GetJSONLDOntologyNamespace(),
		"schema": "https://schema.org/",
		"rdf":    "http://www.w3.org/1999/02/22-rdf-syntax-ns#",
		"rdfs":   "http://www.w3.org/2000/01/rdf-schema#",
		"xsd":    "http://www.w3.org/2001/XMLSchema#",
	}
}

// JSONLDSchema represents an object schema in JSON-LD format
type JSONLDSchema struct {
	Context       map[string]any   `json:"@context"`
	Type          string           `json:"@type"`
	ID            string           `json:"@id"`
	Kind          string           `json:"zqk:kind"`
	SchemaVersion string           `json:"zqk:schemaVersion"`
	Label         string           `json:"rdfs:label,omitempty"`
	Comment       string           `json:"rdfs:comment,omitempty"`
	SubClassOf    string           `json:"rdfs:subClassOf,omitempty"`
	Fields        []JSONLDField    `json:"zqk:fields"`
	CommonFields  bool             `json:"zqk:commonFieldsIncluded"`
	Lifecycle     *JSONLDLifecycle `json:"zqk:lifecycle,omitempty"`
}

// JSONLDField represents a field definition in JSON-LD format
type JSONLDField struct {
	Type            string         `json:"@type"`
	ID              string         `json:"@id"`
	Name            string         `json:"zqk:name"`
	Label           string         `json:"rdfs:label,omitempty"`
	Comment         string         `json:"rdfs:comment,omitempty"`
	FieldType       string         `json:"zqk:type"` // xsd:string, xsd:integer, etc.
	SemanticType    string         `json:"zqk:semanticType,omitempty"`
	Required        bool           `json:"zqk:required"`
	Inherited       bool           `json:"zqk:inherited,omitempty"`
	Traits          []string       `json:"zqk:traits,omitempty"`
	EnumValues      []string       `json:"zqk:enumValues,omitempty"`
	Validation      map[string]any `json:"zqk:validation,omitempty"`
	ReferenceTarget string         `json:"zqk:referenceTarget,omitempty"` // For reference fields
}

// JSONLDLifecycle represents lifecycle information in JSON-LD format
type JSONLDLifecycle struct {
	Type         string             `json:"@type"`
	InitialState string             `json:"zqk:initialState,omitempty"`
	ValidStates  []string           `json:"zqk:validStates,omitempty"`
	Transitions  []JSONLDTransition `json:"zqk:transitions,omitempty"`
}

// JSONLDTransition represents a state transition in JSON-LD format
type JSONLDTransition struct {
	From string `json:"zqk:from"`
	To   string `json:"zqk:to"`
}

// ConvertKindFieldsToJSONLD converts KindFields to JSON-LD schema format
func ConvertKindFieldsToJSONLD(kindFields *objects.KindFields) (*JSONLDSchema, error) {
	context := getJSONLDContext()

	fields := make([]JSONLDField, 0, len(kindFields.AllFields))
	for i := range kindFields.AllFields {
		jsonldField := convertFieldToJSONLD(&kindFields.AllFields[i], kindFields.Kind)
		fields = append(fields, jsonldField)
	}

	return &JSONLDSchema{
		Context:       context,
		Type:          "zqk:ObjectSchema",
		ID:            fmt.Sprintf("zqk:%s", kindFields.Kind),
		Kind:          kindFields.Kind,
		SchemaVersion: objects.DefaultSchemaVersion, // TODO: Get from spec
		Label:         kindFields.Kind,
		Comment:       fmt.Sprintf("Schema for %s objects", kindFields.Kind),
		Fields:        fields,
		CommonFields:  len(kindFields.CommonFields) > 0,
	}, nil
}

// convertFieldToJSONLD converts a FieldInfo to JSON-LD field format
func convertFieldToJSONLD(field *objects.FieldInfo, kind string) JSONLDField {
	fieldID := fmt.Sprintf("zqk:%s/%s", kind, field.Name)

	// Map field type to XSD type
	xsdType := mapFieldTypeToXSD(field.Type)

	jsonldField := JSONLDField{
		Type:         "zqk:FieldDefinition",
		ID:           fieldID,
		Name:         field.Name,
		Label:        field.Name,
		Comment:      field.Description,
		FieldType:    xsdType,
		SemanticType: field.SemanticType,
		Required:     field.Required,
		Inherited:    field.Inherited,
		Traits:       field.Traits,
	}

	// Add enum values if present
	if len(field.EnumValues) > 0 {
		jsonldField.EnumValues = field.EnumValues
	}

	// Add reference target for reference semantic types
	if field.SemanticType == "reference" {
		// Try to infer target from field name (e.g., "priority_plan_ref" -> "priority_plan")
		// This is a heuristic - could be enhanced with metadata
		if refTarget := inferReferenceTarget(field.Name); refTarget != emptyValue {
			jsonldField.ReferenceTarget = fmt.Sprintf("zqk:%s", refTarget)
		}
	}

	return jsonldField
}

// mapFieldTypeToXSD is a convenience wrapper around MapFieldTypeToXSD.
// Kept for backward compatibility with existing code.
func mapFieldTypeToXSD(fieldType string) string {
	return MapFieldTypeToXSD(fieldType)
}

// inferReferenceTarget infers the reference target from a field name
// e.g., "priority_plan_ref" -> "priority_plan"
func inferReferenceTarget(fieldName string) string {
	if strings.HasSuffix(fieldName, "_ref") {
		return strings.TrimSuffix(fieldName, "_ref")
	}
	if strings.HasSuffix(fieldName, "_refs") {
		return strings.TrimSuffix(fieldName, "_refs")
	}
	return ""
}
