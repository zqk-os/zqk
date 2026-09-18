package dna

import (
	"context"
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
)

// TRACK: BLI-CELLULAR-DNA-MEMBRANE-012 / CRIT-CELLULAR-MEMBRANE-002

// IsRequired reports whether this field is marked required in its validation spec or flat attributes.
func (f FieldMetaSpec) IsRequired() bool {
	return f.Validation.Required || f.Required
}

// GetMinLength returns the effective minimum length constraint.
func (f FieldMetaSpec) GetMinLength() int {
	if f.Validation.MinLength > 0 {
		return f.Validation.MinLength
	}
	return f.MinLength
}

// GetMaxLength returns the effective maximum length constraint.
func (f FieldMetaSpec) GetMaxLength() int {
	if f.Validation.MaxLength > 0 {
		return f.Validation.MaxLength
	}
	return f.MaxLength
}

// GetPattern returns the effective regex validation pattern.
func (f FieldMetaSpec) GetPattern() string {
	if f.Validation.Pattern != "" {
		return f.Validation.Pattern
	}
	return f.Pattern
}

// GetEnum returns the effective enumerated values.
func (f FieldMetaSpec) GetEnum() []string {
	if len(f.Validation.Enum) > 0 {
		return f.Validation.Enum
	}
	return f.Enum
}

// IsValid reports whether val matches one of the defined Enum values.
func (e Enum) IsValid(val string) bool {
	for _, v := range e.Values {
		if strings.EqualFold(v, val) {
			return true
		}
	}
	return false
}

// GetField returns the FieldMetaSpec for a given field name.
func (m *ObjectMetaSpec) GetField(name string) (FieldMetaSpec, bool) {
	if m.Fields != nil {
		if f, ok := m.Fields[name]; ok {
			return f, true
		}
	}
	return FieldMetaSpec{}, false
}

// RequiredFields inspects all declared fields and returns those marked required.
func (m *ObjectMetaSpec) RequiredFields() []string {
	var req []string
	seen := make(map[string]bool)
	if m.Fields != nil {
		for name, f := range m.Fields {
			if f.IsRequired() && !seen[name] {
				req = append(req, name)
				seen[name] = true
			}
		}
	}
	return req
}

// Global schema registry.
var (
	registryLock sync.RWMutex
	schemaStore  = make(map[string]*MetaSchema)
)

// NewMetaSchema creates a new MetaSchema instance with specified configuration.
func NewMetaSchema(kind, version string, allowedPlanes []Plane, requiredFields []string) *MetaSchema {
	now := time.Now().UTC()
	fields := make(map[string]FieldMetaSpec)
	for _, f := range requiredFields {
		fields[f] = FieldMetaSpec{
			BaseObject: BaseObject{
				URN:       URN{Cell: "core", Kind: "field_meta_spec", ID: f},
				Kind:      "field_meta_spec",
				Plane:     PlanePromoted,
				Version:   1,
				CreatedAt: now,
				UpdatedAt: now,
			},
			Name: f,
			Type: TypeString,
			Validation: FieldValidation{
				Required: true,
			},
			Required: true,
		}
	}
	return &MetaSchema{
		BaseObject: BaseObject{
			URN:       URN{Cell: "core", Kind: "object_meta_spec", ID: kind},
			Kind:      "object_meta_spec",
			SchemaRef: version,
			Plane:     PlanePromoted,
			Version:   1,
			CreatedAt: now,
			UpdatedAt: now,
		},
		Name:          kind,
		TargetKind:    kind,
		AllowedPlanes: allowedPlanes,
		Fields:        fields,
	}
}

// NewObjectMetaSpec is an alias for NewMetaSchema reflecting pure spec-driven naming.
var NewObjectMetaSpec = NewMetaSchema

// KindBaseEntity is the canonical kind for the foundational base entity.
const KindBaseEntity = "base_entity"

// BaseEntityMetaSchema returns the self-describing MetaSchema governing BaseEntity.
func BaseEntityMetaSchema() *MetaSchema {
	now := time.Now().UTC()
	return &MetaSchema{
		BaseObject: BaseObject{
			URN:       URN{Cell: "core", Kind: "object_meta_spec", ID: KindBaseEntity},
			Kind:      "object_meta_spec",
			SchemaRef: "v1.0.0",
			Plane:     PlanePromoted,
			Version:   1,
			CreatedAt: now,
			UpdatedAt: now,
		},
		Name:          "BaseEntity",
		TargetKind:    KindBaseEntity,
		Traits:        []string{TraitAuditable},
		AllowedPlanes: []Plane{PlaneDraft, PlaneStaged, PlanePromoted, PlaneApoptotic},
		Fields: map[string]FieldMetaSpec{
			"urn": {
				BaseObject: BaseObject{URN: URN{Cell: "core", Kind: "field_meta_spec", ID: "urn"}, Kind: "field_meta_spec", Plane: PlanePromoted, Version: 1},
				Name:       "urn",
				Type:       TypeURN,
				Required:   true,
				Validation: FieldValidation{Required: true},
			},
			"kind": {
				BaseObject: BaseObject{URN: URN{Cell: "core", Kind: "field_meta_spec", ID: "kind"}, Kind: "field_meta_spec", Plane: PlanePromoted, Version: 1},
				Name:       "kind",
				Type:       TypeString,
				Required:   true,
				Validation: FieldValidation{Required: true, MinLength: 1},
			},
			"schema_ref": {
				BaseObject: BaseObject{URN: URN{Cell: "core", Kind: "field_meta_spec", ID: "schema_ref"}, Kind: "field_meta_spec", Plane: PlanePromoted, Version: 1},
				Name:       "schema_ref",
				Type:       TypeString,
			},
			"plane": {
				BaseObject: BaseObject{URN: URN{Cell: "core", Kind: "field_meta_spec", ID: "plane"}, Kind: "field_meta_spec", Plane: PlanePromoted, Version: 1},
				Name:       "plane",
				Type:       TypeInt,
				Required:   true,
				Validation: FieldValidation{Required: true},
			},
			"version": {
				BaseObject: BaseObject{URN: URN{Cell: "core", Kind: "field_meta_spec", ID: "version"}, Kind: "field_meta_spec", Plane: PlanePromoted, Version: 1},
				Name:       "version",
				Type:       TypeInt,
				Required:   true,
				Validation: FieldValidation{Required: true},
			},
			"created_at": {
				BaseObject: BaseObject{URN: URN{Cell: "core", Kind: "field_meta_spec", ID: "created_at"}, Kind: "field_meta_spec", Plane: PlanePromoted, Version: 1},
				Name:       "created_at",
				Type:       TypeString,
				Required:   true,
				Validation: FieldValidation{Required: true},
			},
			"updated_at": {
				BaseObject: BaseObject{URN: URN{Cell: "core", Kind: "field_meta_spec", ID: "updated_at"}, Kind: "field_meta_spec", Plane: PlanePromoted, Version: 1},
				Name:       "updated_at",
				Type:       TypeString,
				Required:   true,
				Validation: FieldValidation{Required: true},
			},
		},
	}
}

func init() {
	_ = RegisterSchema(BaseEntityMetaSchema())
}

// RegisterSchema adds a schema to the global registry.
func RegisterSchema(schema *MetaSchema) error {
	if schema == nil {
		return errfmt.Errorf("schema cannot be nil")
	}
	key := strings.ToLower(strings.TrimSpace(schema.TargetKind))
	if key == "" {
		return errfmt.Errorf("schema target_kind cannot be empty")
	}
	if schema.StorageProfile != "" && !schema.StorageProfile.IsKnown() {
		return errfmt.Errorf("invalid storage_profile %q for kind %q (expected cas_entity, stream, or light_file)", schema.StorageProfile, schema.TargetKind)
	}
	registryLock.Lock()
	defer registryLock.Unlock()
	schemaStore[key] = schema
	return nil
}

// GetSchema retrieves a schema from the global registry (case-insensitive).
func GetSchema(kind string) (*MetaSchema, bool) {
	key := strings.ToLower(strings.TrimSpace(kind))
	registryLock.RLock()
	defer registryLock.RUnlock()
	s, ok := schemaStore[key]
	return s, ok
}

// GetProvenance implements the Auditable interface for MetaSchema.
func (m *MetaSchema) GetProvenance() Provenance {
	return m.Provenance
}

// ComputeDigest computes the cryptographic digest of the MetaSchema provenance.
func (m *MetaSchema) ComputeDigest() (string, error) {
	return m.Provenance.ComputeDigest(), nil
}

func (m *MetaSchema) RegisterDynamicInvariant(fn func(ctx context.Context, obj any) error) {
	m.dynamicInvariants = append(m.dynamicInvariants, fn)
}

func (m *MetaSchema) RegisterHook(hook InvariantHook) {
	m.evalHooks = append(m.evalHooks, hook)
}

func (m *MetaSchema) Validate(ctx context.Context, obj any) error {
	if obj == nil {
		return errfmt.Errorf("entity cannot be nil")
	}

	val := reflect.ValueOf(obj)
	if val.Kind() == reflect.Pointer {
		val = val.Elem()
	}

	// 1. Plane check
	if len(m.AllowedPlanes) > 0 {
		var entityPlane Plane
		planeFound := false

		if val.Kind() == reflect.Struct {
			planeField := val.FieldByName("Plane")
			if planeField.IsValid() && planeField.CanInterface() {
				if p, ok := planeField.Interface().(Plane); ok {
					entityPlane = p
					planeFound = true
				}
			}
		} else if val.Kind() == reflect.Map {
			if pVal := val.MapIndex(reflect.ValueOf("plane")); pVal.IsValid() {
				if p, ok := pVal.Interface().(Plane); ok {
					entityPlane = p
					planeFound = true
				}
			}
		}

		if planeFound {
			allowed := false
			for _, p := range m.AllowedPlanes {
				if entityPlane == p {
					allowed = true
					break
				}
			}
			if !allowed {
				return errfmt.Errorf("plane %v is not allowed for schema %q", entityPlane, m.TargetKind)
			}
		}
	}

	// 2. Required fields check
	reqFields := m.RequiredFields()
	if len(reqFields) > 0 {
		if val.Kind() == reflect.Map {
			for _, rf := range reqFields {
				keyVal := val.MapIndex(reflect.ValueOf(rf))
				if !keyVal.IsValid() || keyVal.IsNil() || isEmptyValue(keyVal) {
					return errfmt.Errorf("required field %q is missing or empty", rf)
				}
			}
		} else if val.Kind() == reflect.Struct {
			for _, rf := range reqFields {
				fieldVal := findStructField(val, rf)
				if !fieldVal.IsValid() || isEmptyValue(fieldVal) {
					return errfmt.Errorf("required field %q is missing or empty", rf)
				}
			}
		}
	}

	// 3. Dynamic invariants
	for _, inv := range m.dynamicInvariants {
		if err := inv(ctx, obj); err != nil {
			return err
		}
	}

	return nil
}

func isEmptyValue(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.String:
		return strings.TrimSpace(v.String()) == ""
	case reflect.Array, reflect.Slice, reflect.Map:
		return v.Len() == 0
	case reflect.Interface, reflect.Pointer:
		return v.IsNil()
	default:
		return false
	}
}

func findStructField(structVal reflect.Value, name string) reflect.Value {
	t := structVal.Type()
	for i := 0; i < structVal.NumField(); i++ {
		field := t.Field(i)
		if field.Anonymous && field.Type.Kind() == reflect.Struct {
			if nested := findStructField(structVal.Field(i), name); nested.IsValid() {
				return nested
			}
		}
		if strings.EqualFold(field.Name, name) ||
			strings.EqualFold(strings.ReplaceAll(field.Name, "_", ""), strings.ReplaceAll(name, "_", "")) {
			return structVal.Field(i)
		}
		for _, tagKey := range []string{"yaml", "json"} {
			if tag := field.Tag.Get(tagKey); tag != "" {
				parts := strings.Split(tag, ",")
				if strings.EqualFold(parts[0], name) ||
					strings.EqualFold(strings.ReplaceAll(parts[0], "_", ""), strings.ReplaceAll(name, "_", "")) {
					return structVal.Field(i)
				}
			}
		}
	}
	return reflect.Value{}
}

// HasTrait checks whether this MetaSchema declares a given behavioral trait.
func (m *MetaSchema) HasTrait(trait string) bool {
	for _, t := range m.Traits {
		if strings.EqualFold(strings.TrimSpace(t), strings.TrimSpace(trait)) {
			return true
		}
	}
	return false
}

// VetObject validates an entity map against this MetaSchema's structural field rules and vetting checklist.
func (m *MetaSchema) VetObject(data map[string]any) error {
	for name, rule := range m.Fields {
		val, exists := data[name]
		if !exists || val == nil {
			if rule.IsRequired() {
				return fmt.Errorf("membrane violation: required field %q is missing for kind %q", name, m.TargetKind)
			}
			continue
		}

		// List type mismatch check
		if rule.Type != TypeList {
			switch val.(type) {
			case []any, []string, []map[string]any:
				return fmt.Errorf("membrane violation: field %q is not a list but received a slice", name)
			}
		}

		minLen := rule.GetMinLength()
		maxLen := rule.GetMaxLength()
		pattern := rule.GetPattern()
		enumVals := rule.GetEnum()

		switch rule.Type {
		case TypeString, TypeEnum:
			s, ok := val.(string)
			if !ok {
				return fmt.Errorf("membrane violation: field %q must be string, got %T", name, val)
			}
			if rule.IsRequired() && minLen > 0 && len(strings.TrimSpace(s)) < minLen {
				return fmt.Errorf("membrane violation: field %q must have min length %d", name, minLen)
			}
			if maxLen > 0 && len(strings.TrimSpace(s)) > maxLen {
				return fmt.Errorf("membrane violation: field %q exceeds max length %d", name, maxLen)
			}
			if pattern != "" {
				matched, err := regexp.MatchString(pattern, s)
				if err != nil || !matched {
					return fmt.Errorf("membrane violation: field %q does not match required pattern %q", name, pattern)
				}
			}
			if len(enumVals) > 0 {
				found := false
				for _, ev := range enumVals {
					if strings.EqualFold(ev, s) {
						found = true
						break
					}
				}
				if !found {
					return fmt.Errorf("membrane violation: field %q has invalid enum value %q (expected one of %v)", name, s, enumVals)
				}
			}
		case TypeURN:
			s, ok := val.(string)
			if !ok {
				return fmt.Errorf("membrane violation: field %q must be URN string, got %T", name, val)
			}
			if _, err := ParseURN(s); err != nil {
				return fmt.Errorf("membrane violation: field %q has invalid URN: %w", name, err)
			}
		case TypeList:
			switch val.(type) {
			case []any, []string, []map[string]any:
			default:
				return fmt.Errorf("membrane violation: field %q must be list, got %T", name, val)
			}
		case TypeMap:
			switch val.(type) {
			case map[string]any, map[string]string:
			default:
				return fmt.Errorf("membrane violation: field %q must be map, got %T", name, val)
			}
		case TypeObject:
			switch val.(type) {
			case map[string]any, map[string]string:
			default:
				if reflect.ValueOf(val).Kind() != reflect.Struct && reflect.ValueOf(val).Kind() != reflect.Map {
					return fmt.Errorf("membrane violation: field %q must be object, got %T", name, val)
				}
			}
		case TypeInt:
			switch val.(type) {
			case int, int64, float64:
			default:
				return fmt.Errorf("membrane violation: field %q must be integer, got %T", name, val)
			}
		case TypeBool:
			if _, ok := val.(bool); !ok {
				return fmt.Errorf("membrane violation: field %q must be boolean, got %T", name, val)
			}
		}
	}
	return nil
}

// VetTransition evaluates trait invariants and dynamic hooks before allowing an entity to shift planes or phases.
func (m *MetaSchema) VetTransition(target any, targetPlane Plane) error {
	// Determine target macro phase from plane if possible
	var targetPhase MacroPhase
	switch targetPlane {
	case PlaneDraft:
		targetPhase = MacroPhaseGestation
	case PlaneStaged:
		targetPhase = MacroPhaseMetabolism
	case PlanePromoted:
		targetPhase = MacroPhaseEquilibrium
	case PlaneApoptotic:
		targetPhase = MacroPhaseApoptosis
	}

	// 1. Evaluate foundational behavioral trait invariants
	if err := VetTraitInvariants(m, target, targetPhase, targetPlane); err != nil {
		return err
	}

	// 2. Evaluate base epistemic invariants
	if targetPlane == PlanePromoted || targetPlane == PlaneStaged {
		if aud, ok := target.(interface{ GetProvenance() Provenance }); ok {
			prov := aud.GetProvenance()
			if strings.TrimSpace(prov.Actor()) == "" {
				return fmt.Errorf("epistemic invariant violation: cannot transition to %s without ActorAttestation (AgentURN required)", targetPlane)
			}
			if strings.TrimSpace(prov.ParentHash) == "" && targetPlane == PlanePromoted {
				return fmt.Errorf("epistemic invariant violation: cannot transition to PlanePromoted without causal lineage (ParentHash required)")
			}
		}
	}

	// 3. Dynamic evaluation hooks
	for _, hook := range m.evalHooks {
		if err := hook(target, targetPlane); err != nil {
			return err
		}
	}
	return nil
}

// ResolvePhase maps a domain sub-status to its macro-phase and plane based on this schema's declared MacroPhases mapping.
func (m *MetaSchema) ResolvePhase(subStatus string) (MacroPhase, Plane, bool) {
	st := strings.ToLower(strings.TrimSpace(subStatus))
	if m.MacroPhases != nil {
		for phase, spec := range m.MacroPhases {
			for _, s := range spec.SubStatuses {
				if strings.EqualFold(s, st) {
					return phase, spec.Plane, true
				}
			}
		}
	}
	return MacroPhaseGestation, PlaneDraft, false
}
