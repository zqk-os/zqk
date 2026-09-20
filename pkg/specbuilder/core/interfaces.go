package core

import (
	"io"
)

// Builder is the interface for building objects programmatically
// Domain-specific builders implement this interface
type Builder[T any] interface {
	// Build returns the constructed object
	Build() T
}

// Spec represents a declarative specification
// Domain-specific specs implement this interface
type Spec interface {
	// Validate validates the spec
	Validate() error
	// GetName returns the name/identifier of the spec
	GetName() string
}

// Generator orchestrates the transformation from specs to artifacts
type Generator[S Spec, T any] interface {
	// GenerateFromSpec generates an artifact from a single spec
	GenerateFromSpec(spec S) (T, error)
	// GenerateFromSpecs generates artifacts from multiple specs
	GenerateFromSpecs(specs []S) ([]T, error)
}

// Writer handles writing generated artifacts to outputs
type Writer[T any] interface {
	// Write writes an artifact to a writer
	Write(artifact T, writer io.Writer) error
	// WriteToFile writes an artifact to a file
	WriteToFile(artifact T, filePath string) error
	// WriteToBytes writes an artifact to bytes
	WriteToBytes(artifact T) ([]byte, error)
	// WriteToString writes an artifact to a string
	WriteToString(artifact T) (string, error)
}

// BuilderFactory creates builders from specs
// Domain-specific packages implement this to bridge specs to builders
type BuilderFactory[S Spec, T any] interface {
	// CreateBuilder creates a builder from a spec
	CreateBuilder(spec S) Builder[T]
}

// SpecLoader loads specifications from various sources
type SpecLoader[S Spec] interface {
	// LoadSpec loads a single spec from a source
	LoadSpec(source string) (S, error)
	// LoadSpecs loads multiple specs from a source
	LoadSpecs(source string) ([]S, error)
}

// Constants represents field name constants generated from a spec
// Domain-specific constants implementations provide field name constants
// that can be used instead of hardcoded strings
type Constants interface {
	// GetFieldName returns the constant name for a field (e.g., "FieldId" for "id")
	GetFieldName(fieldName string) string
	// GetAllFieldNames returns all field name constants as a map of field name -> constant name
	GetAllFieldNames() map[string]string
	// GetPackage returns the package name where these constants are defined
	GetPackage() string
	// GetOntology returns the ontology/name of the spec these constants are for
	GetOntology() string
}

// ConstantsGenerator generates constants from specs
type ConstantsGenerator[S Spec] interface {
	// GenerateConstants generates constants from a single spec
	GenerateConstants(spec S) (Constants, error)
	// GenerateConstantsFromSpecs generates constants from multiple specs
	GenerateConstantsFromSpecs(specs []S) ([]Constants, error)
}
