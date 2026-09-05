package translation

import "github.com/lanceman/zqk/pkg/objects"

// Constants for translated domain_registry and object_spec-like output.
// Used by all format translators (RDF/OWL, Cypher, JSON Schema, OpenAPI) so
// placeholder IDs, kinds, and defaults are not hardcoded in each file.

const (
	FormatRDFOWL     = "rdf_owl"
	FormatTurtle     = "turtle"
	FormatRDFXML     = "rdf_xml"
	FormatJSONLD     = "jsonld"
	FormatCypher     = "cypher"
	FormatJSONSchema = "json_schema"
	FormatOpenAPI    = "openapi"
	FormatUnknown    = "unknown_format"
)

const (
	KeyTitle       = "title"
	KeyDescription = "description"
	KeyNamespace   = objects.KindNamespace
	KeySubClassOf  = "sub_class_of"
	KeyDomains     = "domains"
	KeyOntology    = "ontology"
	KeyExtends     = "extends"
	KeyVisibility  = "visibility"
	KeyTraits      = "traits"
	KeyFields      = "fields"
)

const (
	NamespacePatternDomain = "domain:%s:*"
)

const (
	JSONPrefixObject = "{"
	JSONPrefixArray  = "["
)

const (
	TurtleMarkerClass = "owl:Class"
	TurtleMarkerLabel = "rdfs:label"
	RDFXMLMarkerClass = "<owl:Class"
	RDFXMLMarkerRoot  = "<rdf:RDF"
)

const (
	DefaultEmptyInputWarning = "empty input"
)

const (
	// DomainRegistryKind is the object kind for translated domain registry output.
	DomainRegistryKind = objects.KindDomainRegistry
	// DomainRegistryPlaceholderID is the placeholder ID replaced by the import pipeline with next DOMAIN-REG-NNN.
	DomainRegistryPlaceholderID = "DOMAIN-REG-000"
	// DefaultNamespaceID is the default namespace for translated domain objects.
	DefaultNamespaceID = "zqk:kernel"
	// DefaultStatus is the default status for translated domain_registry.
	DefaultStatus = "active"
	// CreatedBySystem is the created_by value for system-generated translated objects.
	// TRACK: REDACTED — ACC-* cutover
	CreatedBySystem = "ACC-1785920548450214012-68b850c0"
	// UpdatedBySystem is the updated_by value for system-generated translated objects.
	// TRACK: REDACTED — ACC-* cutover (do not persist bare "system").
	UpdatedBySystem = CreatedBySystem
)

// Spec defaults for object_spec-like output (TranslateResult.Specs).
const (
	// DefaultExtends is the default extends value when no parent type is present.
	DefaultExtends = objects.KindExtensibleObject
	// DefaultVisibility is the default visibility for translated specs.
	DefaultVisibility = "public"
	// DefaultTrait is the default trait name for translated specs.
	DefaultTrait = "base_object_traits"
)

// Domain registry title per source format (for translated domain_registry object).
const (
	DomainRegistryTitleFromCypher     = "Domain registry (translated from Cypher)"
	DomainRegistryTitleFromJSONSchema = "Domain registry (translated from JSON Schema)"
	DomainRegistryTitleFromOpenAPI    = "Domain registry (translated from OpenAPI)"
	DomainRegistryTitleFromOntology   = "Domain registry (translated from ontology)"
)
