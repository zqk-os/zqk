package ontology

import (
	"strings"

	"github.com/zqk-os/zqk/pkg/objects"
)

const systemOntologyID = "zqk-system-ontology"

// primarySystemOntologyKinds is the planning/Gantt slice this package historically
// exposed. Properties are projected from live object specs, not a hand map.
// TRACK: BLI-KERNEL-REF-GRAPH-ACYCLIC-001 — keep in lockstep with GRAPH_EDGE_OWNERSHIP.md
var primarySystemOntologyKinds = []string{
	objects.KindGoal,
	objects.KindMilestone,
	objects.KindWorkstream,
	objects.KindPriorityPlan,
	objects.KindBacklogItem,
	objects.KindRequirement,
	objects.KindCriteria,
	objects.KindTestCase,
	objects.KindRoadmap,
	objects.KindMission,
	objects.KindVision,
	objects.KindDecision,
	objects.KindComponent,
	objects.KindAccount,
}

// untypedRefFields are inherited or stem-unknown refs that are not typed A↔B edges.
// Do not project them as occupancy/composition links. See GRAPH_EDGE_OWNERSHIP.md.
var untypedRefFields = map[string]struct{}{
	objects.FieldKeyRelatedObjectRefs: {},
	"commit_refs":                     {},
	objects.FieldKeyDocumentRefs:      {},
	objects.FieldKeyLifecycleRef:      {},
	objects.FieldKeyLifecycleRefs:    {},
	objects.FieldKeyObjectRef:         {},
}

var identityFieldKeys = []string{
	objects.FieldKeyID,
	objects.FieldKeyKind,
	objects.FieldKeyTitle,
	objects.FieldKeyStatus,
}

// SystemOntology returns the primary planning ontology projected from object specs.
// One-shot spec walk (SPEC_ORIGIN_PLANE): not a second SSOT. Typed *_ref fields
// follow occupancy DNA — parent composition (e.g. requirement.criteria_refs) and
// child membership; reverse occupancy lists are absent because they are gone from the spec.
func SystemOntology() Ontology {
	return SystemOntologyFromLoader(objects.NewSpecLoader(""))
}

// SystemOntologyFromLoader projects primary kinds from an already-constructed spec loader.
func SystemOntologyFromLoader(loader *objects.SpecLoader) Ontology {
	classes := make(map[string]Class, len(primarySystemOntologyKinds))
	if loader == nil {
		return Ontology{ID: systemOntologyID, Version: "1.0", SchemaVersion: "2.0.0", Classes: classes}
	}
	for _, kind := range primarySystemOntologyKinds {
		spec, err := loader.LoadSpecWithInheritance(kind + ".yaml")
		if err != nil || spec == nil {
			continue
		}
		classes[kind] = classFromSpec(kind, spec)
	}
	return Ontology{
		ID:            systemOntologyID,
		Version:       "1.0",
		SchemaVersion: "2.0.0",
		Classes:       classes,
	}
}

func classFromSpec(kind string, spec *objects.Spec) Class {
	props := make(map[string]Property)
	for _, key := range identityFieldKeys {
		props[key] = identityProperty(key, spec)
	}
	if objects.SpecResolvedFieldsMissing(spec) {
		return Class{Name: kind, Properties: props}
	}
	for fieldName, raw := range spec.ResolvedFields {
		if !isTypedGraphRefField(fieldName) {
			continue
		}
		if _, skip := untypedRefFields[fieldName]; skip {
			continue
		}
		fieldMap, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		props[fieldName] = propertyFromFieldDef(kind, fieldName, fieldMap, spec)
	}
	return Class{Name: kind, Properties: props}
}

func identityProperty(key string, spec *objects.Spec) Property {
	p := Property{Name: key, Type: "string", Required: true}
	if objects.SpecResolvedFieldsMissing(spec) {
		return p
	}
	raw, ok := spec.ResolvedFields[key]
	if !ok {
		return p
	}
	fieldMap, ok := raw.(map[string]any)
	if !ok {
		return p
	}
	if t := fieldType(fieldMap); t != "" {
		p.Type = t
	}
	p.Required = fieldRequired(fieldMap)
	return p
}

func propertyFromFieldDef(kind, fieldName string, fieldMap map[string]any, spec *objects.Spec) Property {
	typ := fieldType(fieldMap)
	if typ == "" {
		typ = "list"
	}
	p := Property{
		Name:     fieldName,
		Type:     typ,
		Required: fieldRequired(fieldMap),
		EdgeRole: string(objects.EdgeRoleForKindField(kind, fieldName, spec)),
	}
	return p
}

func isTypedGraphRefField(fieldName string) bool {
	return strings.HasSuffix(fieldName, "_ref") || strings.HasSuffix(fieldName, "_refs")
}

func fieldType(fieldMap map[string]any) string {
	if t, ok := fieldMap[objects.FieldKeyType].(string); ok {
		return t
	}
	return ""
}

func fieldRequired(fieldMap map[string]any) bool {
	if b, ok := fieldMap["required"].(bool); ok && b {
		return true
	}
	v, ok := fieldMap["validation"].(map[string]any)
	if !ok {
		return false
	}
	b, ok := v["required"].(bool)
	return ok && b
}

// SpecClassification satisfies the agent validation script
type SpecClassification string

const (
	ClassificationMixin            SpecClassification = "mixin"
	ClassificationPersistedMissing SpecClassification = "persisted-missing"
	ClassificationThinExam         SpecClassification = "thin-exam"
)
