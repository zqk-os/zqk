package datacell

import "github.com/lanceman/zqk/pkg/errfmt"

// CellKindDescriptor is one spec-backed kind as seen by the data-cell identity layer (DATA_CELL_MODEL).
//
// Identity (v1): one logical cell per spec-backed kind name. The spec plane is objects.SpecIndex;
// each entry’s Kind is the ontology/kind key. CellID matches Kind until multi-kind cells exist
// (see DATA_CELL_MODEL.md). StorageProfileWire comes from object_specs storage_profile (including
// inheritance when the index is built from specs via LoadSpecWithInheritance).
type CellKindDescriptor struct {
	Kind               string `json:"kind"`
	CellID             string `json:"cell_id"`
	StorageProfileWire string `json:"storage_profile,omitempty"`
	// ParsedProfile is set when StorageProfileWire is non-empty and valid.
	ParsedProfile StorageProfile `json:"-"`
}

// ValidateV1Identity returns nil if Kind is non-empty and CellID equals Kind (v1 contract).
func (c CellKindDescriptor) ValidateV1Identity() error {
	if c.Kind == "" {
		return errfmt.Errorf("data cell descriptor: kind is required")
	}
	if c.CellID != c.Kind {
		return errfmt.Errorf("data cell descriptor: v1 requires cell_id to equal kind (cell_id=%q kind=%q)", c.CellID, c.Kind)
	}
	return nil
}

// EffectiveStorageProfile returns ParsedProfile after descriptors are loaded from the spec index (see datacellregistry).
func (c CellKindDescriptor) EffectiveStorageProfile() StorageProfile {
	return c.ParsedProfile
}

// HasStorageProfile reports whether the spec plane declares a non-empty storage_profile for this kind.
func (c CellKindDescriptor) HasStorageProfile() bool {
	return c.StorageProfileWire != ""
}

// FilterDescriptorsByStorageProfile returns descriptors whose parsed profile equals want (empty want matches only unset profiles if matchEmpty is true).
// MapDescriptorsByKind returns kind name → descriptor. If multiple entries share a kind, the last wins
// (the spec index does not duplicate kinds).
func MapDescriptorsByKind(desc []CellKindDescriptor) map[string]CellKindDescriptor {
	if len(desc) == 0 {
		return nil
	}
	m := make(map[string]CellKindDescriptor, len(desc))
	for _, d := range desc {
		m[d.Kind] = d
	}
	return m
}

func FilterDescriptorsByStorageProfile(desc []CellKindDescriptor, want StorageProfile, matchEmpty bool) []CellKindDescriptor {
	if len(desc) == 0 {
		return nil
	}
	out := make([]CellKindDescriptor, 0, len(desc))
	for _, d := range desc {
		switch {
		case want == "" && matchEmpty && d.StorageProfileWire == "":
			out = append(out, d)
		case want != "" && d.ParsedProfile == want:
			out = append(out, d)
		}
	}
	return out
}
