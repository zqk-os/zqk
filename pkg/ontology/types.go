package ontology

import "errors"

var (
	ErrClassNotFound   = errors.New("class not found in registered ontologies")
	ErrInvalidInstance = errors.New("instance violates class schema")
)

// Ontology represents a versioned semantic schema.
type Ontology struct {
	ID            string
	Version       string
	SchemaVersion string `json:"schema_version" yaml:"schema_version"` // Added for versioning
	Classes       map[string]Class
}

// Class represents a node type.
type Class struct {
	Name       string
	Properties map[string]Property
}

// Property represents an edge or attribute type.
type Property struct {
	Name     string
	Type     string
	Required bool
	// EdgeRole is membership | composition | associate for typed *_ref fields.
	EdgeRole string
}
