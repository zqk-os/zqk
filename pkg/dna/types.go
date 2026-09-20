package dna

import (
	"context"
	"sync"
	"time"
)

// TRACK: BLI-CELLULAR-DNA-PRIMITIVES-011 / CRIT-CELLULAR-PRIMITIVES-001 / CRIT-CELLULAR-URN-CONVENTION-007
// TRACK: BLI-CELLULAR-DNA-MEMBRANE-012 / CRIT-CELLULAR-MEMBRANE-002
// TRACK: BLI-CELLULAR-DNA-LIFECYCLE-SUITE-013 / CRIT-CELLULAR-METABOLIC-LIFECYCLE-006

// ============================================================================
// 1. TRANSACTIONAL ISOLATION PLANES
// ============================================================================

// Plane defines the transactional isolation boundary of an entity.
type Plane uint8

const (
	PlaneDraft Plane = iota
	PlaneStaged
	PlanePromoted
	PlaneApoptotic
)

// ============================================================================
// 2. CANONICAL ADDRESSING (URN)
// ============================================================================

// URN represents a canonical entity address: urn:<brand>:<cell_id>:<kind>:<id>
type URN struct {
	Brand string `json:"brand,omitempty" yaml:"brand,omitempty"`
	Cell  string `json:"cell" yaml:"cell"`
	Kind  string `json:"kind" yaml:"kind"`
	ID    string `json:"id" yaml:"id"`
	raw   string
}

// ============================================================================
// 3. BASE ENTITY NUCLEUS
// ============================================================================

// BaseObject forms the structural foundation for all ZQK entities.
type BaseObject struct {
	URN       URN       `json:"urn" yaml:"urn"`
	Kind      string    `json:"kind" yaml:"kind"`
	SchemaRef string    `json:"schema_ref" yaml:"schema_ref"`
	Plane     Plane     `json:"plane" yaml:"plane"`
	Version   uint64    `json:"version" yaml:"version"`
	CreatedAt time.Time `json:"created_at" yaml:"created_at"`
	UpdatedAt time.Time `json:"updated_at" yaml:"updated_at"`
}

// BaseEntity is the canonical alias for BaseObject in the Cellular Knowledge OS.
type BaseEntity = BaseObject

// Entity represents the universal interface implemented by all Cellular Knowledge OS objects.
type Entity interface {
	GetURN() URN
	GetKind() string
	GetPlane() Plane
	GetVersion() uint64
	GetCreatedAt() time.Time
	GetUpdatedAt() time.Time
	Validate() error
}

// SystemObject is an alias for Entity representing any object managed by the Knowledge Kernel.
type SystemObject = Entity

// ============================================================================
// 4. CRYPTOGRAPHIC PROVENANCE & AUDITABILITY
// ============================================================================

// Provenance captures cryptographic actor accountability and causal lineage.
// The actor is identified by its canonical AgentURN (resolving to an Agent/Account
// object whose schema and capabilities are governed by its meta_spec).
type Provenance struct {
	ParentHash string            `json:"parent_hash" yaml:"parent_hash"`
	AgentURN   URN               `json:"agent_urn" yaml:"agent_urn"`
	AgentID    string            `json:"agent_id,omitempty" yaml:"agent_id,omitempty"`
	ModelHash  string            `json:"model_hash,omitempty" yaml:"model_hash,omitempty"`
	Signature  string            `json:"signature" yaml:"signature"`
	Hash       string            `json:"hash,omitempty" yaml:"hash,omitempty"`
	Metadata   map[string]string `json:"metadata,omitempty" yaml:"metadata,omitempty"`
}

// Auditable struct embedded in entities to satisfy provenance guarantees.
type Auditable struct {
	Provenance Provenance `json:"provenance" yaml:"provenance"`
}

// ============================================================================
// 5. METABOLIC STATE MACHINE & LIFECYCLES
// ============================================================================

// InvariantGateFunc evaluates an invariant gate predicate before state transition.
type InvariantGateFunc func(ctx context.Context, obj any) error

// MacroPhase represents the universal metabolic macro-phases of the Cellular Knowledge OS core state machine.
type MacroPhase string

const (
	MacroPhaseGestation   MacroPhase = "Gestation"   // PlaneDraft: Inception, conceptualization, drafting
	MacroPhaseMetabolism  MacroPhase = "Metabolism"  // PlaneStaged: Active development, task execution
	MacroPhaseAttestation MacroPhase = "Attestation" // PlaneStaged: Test execution, criteria invariant verification
	MacroPhaseEquilibrium MacroPhase = "Equilibrium" // PlanePromoted: Complete, validated, authoritative state
	MacroPhaseRegression  MacroPhase = "Regression"  // PlanePromoted: Graduated sentinel suite defending baseline
	MacroPhaseApoptosis   MacroPhase = "Apoptosis"   // PlaneApoptotic: Archived, quarantined, or error
)

// LifecyclePhase is an alias for MacroPhase preserving backward compatibility.
type LifecyclePhase = MacroPhase

const (
	PhaseDraft      LifecyclePhase = MacroPhaseGestation
	PhaseClaimed    LifecyclePhase = MacroPhaseMetabolism
	PhaseEvaluating LifecyclePhase = MacroPhaseAttestation
	PhasePromoted   LifecyclePhase = MacroPhaseEquilibrium
	PhaseApoptotic  LifecyclePhase = MacroPhaseApoptosis
)

// LifecycleFacet governs an entity's metabolic macro-phase, domain sub-status, and plane transitions.
type LifecycleFacet struct {
	MacroPhase         MacroPhase          `json:"macro_phase" yaml:"macro_phase"`
	SubStatus          string              `json:"sub_status" yaml:"sub_status"`
	Plane              Plane               `json:"plane" yaml:"plane"`
	AllowedTransitions map[string][]string `json:"allowed_transitions,omitempty" yaml:"allowed_transitions,omitempty"`
	gates              map[string][]InvariantGateFunc
	mu                 *sync.RWMutex
}

// Lifecycle struct manages state machine transitions, invariant-guarded hops, and apoptosis rules.
type Lifecycle struct {
	Status             string              `json:"status" yaml:"status"`
	AllowedTransitions map[string][]string `json:"allowed_transitions,omitempty" yaml:"allowed_transitions,omitempty"`
	gates              map[string][]InvariantGateFunc
	mu                 *sync.RWMutex
}

// ============================================================================
// 1b. PHYSICAL STORAGE PROFILES
// ============================================================================

// StorageProfile defines the physical persistence profile and membrane medium for an entity kind.
type StorageProfile string

const (
	// StorageProfileCASEntity represents hash-addressed, Git-backed CAS instance storage (e.g. .zqk/process/ specs).
	StorageProfileCASEntity StorageProfile = "cas_entity"
	// StorageProfileStream represents append-heavy sequential segment journals (e.g. audit_events, telemetry).
	StorageProfileStream StorageProfile = "stream"
	// StorageProfileLightFile represents low-ceremony JSON/YAML/JSONL filesystem surfaces (e.g. organism slices, scratch caches).
	StorageProfileLightFile StorageProfile = "light_file"
)

// ============================================================================
// 6. SPEC-DRIVEN CELLULAR MEMBRANE: OBJECT & FIELD META-SPECS
// ============================================================================

// FieldType defines the primitive or relation type of a field in an ObjectMetaSpec.
type FieldType string

const (
	TypeString FieldType = "string"
	TypeInt    FieldType = "int"
	TypeBool   FieldType = "bool"
	TypeList   FieldType = "list"
	TypeMap    FieldType = "map"
	TypeURN    FieldType = "urn"
	TypeObject FieldType = "object"
	TypeEnum   FieldType = "enum"
)

// FieldValidation defines structural validation and constraint rules for a field.
type FieldValidation struct {
	Required  bool     `json:"required,omitempty" yaml:"required,omitempty"`
	MinLength int      `json:"min_length,omitempty" yaml:"min_length,omitempty"`
	MaxLength int      `json:"max_length,omitempty" yaml:"max_length,omitempty"`
	Pattern   string   `json:"pattern,omitempty" yaml:"pattern,omitempty"`
	Enum      []string `json:"enum,omitempty" yaml:"enum,omitempty"`
}

// RelationSpec defines the topological and relational contract for reference and URN fields.
type RelationSpec struct {
	TargetKind    string `json:"target_kind,omitempty" yaml:"target_kind,omitempty"` // Target entity kind
	EdgeType      string `json:"edge_type,omitempty" yaml:"edge_type,omitempty"`     // "causal", "composition", "association", "attestation"
	Cascade       string `json:"cascade,omitempty" yaml:"cascade,omitempty"`         // "none", "quarantine", "apoptosis"
	Bidirectional bool   `json:"bidirectional,omitempty" yaml:"bidirectional,omitempty"`
	InverseField  string `json:"inverse_field,omitempty" yaml:"inverse_field,omitempty"`
}

// FieldMetaSpec is the first-class meta specification governing an individual entity field.
// In the Cellular Knowledge OS ontology, it extends BaseObject as an addressable spec entity.
type FieldMetaSpec struct {
	BaseObject       `yaml:",inline"`
	Name             string          `json:"name" yaml:"name"`
	FieldProfileCode string          `json:"field_profile_code,omitempty" yaml:"field_profile_code,omitempty"`
	Type             FieldType       `json:"type" yaml:"type"`
	SemanticType     string          `json:"semantic_type,omitempty" yaml:"semantic_type,omitempty"`
	Traits           []string        `json:"traits,omitempty" yaml:"traits,omitempty"`
	Validation       FieldValidation `json:"validation,omitempty" yaml:"validation,omitempty"`

	// Definitional structures for non-primitive field types:
	EnumSpec        *Enum         `json:"enum_spec,omitempty" yaml:"enum_spec,omitempty"`
	Relation        *RelationSpec `json:"relation,omitempty" yaml:"relation,omitempty"`
	TargetSchemaRef string        `json:"target_schema_ref,omitempty" yaml:"target_schema_ref,omitempty"` // For TypeObject / composite facets

	Description string `json:"description,omitempty" yaml:"description,omitempty"`

	// Flat attributes:
	Required  bool     `json:"required,omitempty" yaml:"required,omitempty"`
	MinLength int      `json:"min_length,omitempty" yaml:"min_length,omitempty"`
	MaxLength int      `json:"max_length,omitempty" yaml:"max_length,omitempty"`
	Pattern   string   `json:"pattern,omitempty" yaml:"pattern,omitempty"`
	Enum      []string `json:"enum,omitempty" yaml:"enum,omitempty"`
	Default   any      `json:"default,omitempty" yaml:"default,omitempty"`
}

// Enum defines an enumerated set of permitted string values and their semantic definitions.
type Enum struct {
	Name        string   `json:"name" yaml:"name"`
	Values      []string `json:"values" yaml:"values"`
	Description string   `json:"description,omitempty" yaml:"description,omitempty"`
}

// TraitMetaSpec defines the behavioral, structural, or lifecycle contract of a trait in the Knowledge Kernel.
// Extends BaseObject to participate in the cellular spec ontology.
type TraitMetaSpec struct {
	BaseObject  `yaml:",inline"`
	Name        string         `json:"name" yaml:"name"`
	Description string         `json:"description,omitempty" yaml:"description,omitempty"`
	Category    string         `json:"category,omitempty" yaml:"category,omitempty"` // "behavior", "group", "metric", "audit"
	Requires    []string       `json:"requires,omitempty" yaml:"requires,omitempty"`
	Conflicts   []string       `json:"conflicts,omitempty" yaml:"conflicts,omitempty"`
	Includes    []string       `json:"includes,omitempty" yaml:"includes,omitempty"`
	FieldLevel  bool           `json:"field_level,omitempty" yaml:"field_level,omitempty"`
	ObjectLevel bool           `json:"object_level,omitempty" yaml:"object_level,omitempty"`
	Config      map[string]any `json:"config,omitempty" yaml:"config,omitempty"`
}

// InvariantHook represents a dynamic semantic assertion evaluated before plane transitions.
type InvariantHook func(target any, targetPlane Plane) error

// DynamicInvariant represents a contextual assertion evaluated during schema validation.
type DynamicInvariant func(ctx context.Context, obj any) error

// MacroPhaseSpec maps a metabolic macro-phase to its plane and domain sub-statuses.
type MacroPhaseSpec struct {
	Phase       MacroPhase `json:"phase" yaml:"phase"`
	Plane       Plane      `json:"plane" yaml:"plane"`
	SubStatuses []string   `json:"sub_statuses" yaml:"sub_statuses"`
}

// ObjectMetaSpec is the cellular membrane and invariant gatekeeper that defines the
// structural shape, behavioral traits, and execution contracts of an entity organism.
type ObjectMetaSpec struct {
	BaseObject     `yaml:",inline"`
	Provenance     Provenance     `json:"provenance" yaml:"provenance"`
	Name           string         `json:"name" yaml:"name"`
	TargetKind     string         `json:"target_kind" yaml:"target_kind"`
	Extends        string         `json:"extends,omitempty" yaml:"extends,omitempty"`
	Composes       []string       `json:"composes,omitempty" yaml:"composes,omitempty"`
	Traits         []string       `json:"traits,omitempty" yaml:"traits,omitempty"`
	StorageProfile StorageProfile `json:"storage_profile,omitempty" yaml:"storage_profile,omitempty"`
	KernelCritical bool           `json:"kernel_critical,omitempty" yaml:"kernel_critical,omitempty"`
	AllowedPlanes  []Plane        `json:"allowed_planes,omitempty" yaml:"allowed_planes,omitempty"`

	// Spec-driven field definitions
	Fields map[string]FieldMetaSpec `json:"fields,omitempty" yaml:"fields,omitempty"`

	Invariants  []string                      `json:"invariants,omitempty" yaml:"invariants,omitempty"`
	MacroPhases map[MacroPhase]MacroPhaseSpec `json:"macro_phases,omitempty" yaml:"macro_phases,omitempty"`

	dynamicInvariants []DynamicInvariant
	evalHooks         []InvariantHook
}

// MetaSchema is the canonical alias for ObjectMetaSpec.
type MetaSchema = ObjectMetaSpec
