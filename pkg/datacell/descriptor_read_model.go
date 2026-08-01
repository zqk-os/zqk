package datacell

import (
	"slices"

	"github.com/lanceman/zqk/pkg/errfmt"
)

// DescriptorReadModel is a spec-index–derived snapshot of cell descriptors correlated with the
// materialized spec_index revision fields (BuilderSpecCacheRevision / GlobalSpecCacheRevision).
// Use [ReadModelIsStale] against objects.SpecLoader.SpecCacheRevision() to decide whether to
// rebuild before serving queries (unified fast path vs durable spec plane).
type DescriptorReadModel struct {
	descriptors []CellKindDescriptor
	byKind      map[string]CellKindDescriptor
	builtAtRev  uint64
}

// NewDescriptorReadModel returns an immutable read model; desc is copied.
func NewDescriptorReadModel(desc []CellKindDescriptor, builtAtSpecCacheRevision uint64) (*DescriptorReadModel, error) {
	if len(desc) == 0 {
		return &DescriptorReadModel{builtAtRev: builtAtSpecCacheRevision}, nil
	}
	cp := slices.Clone(desc)
	for i := range cp {
		if err := cp[i].ValidateV1Identity(); err != nil {
			return nil, errfmt.Newf("descriptor read model").Wrap(err)
		}
	}
	return &DescriptorReadModel{
		descriptors: cp,
		byKind:      MapDescriptorsByKind(cp),
		builtAtRev:  builtAtSpecCacheRevision,
	}, nil
}

// BuiltAtSpecCacheRevision implements [CellReadModel].
func (m *DescriptorReadModel) BuiltAtSpecCacheRevision() uint64 {
	if m == nil {
		return 0
	}
	return m.builtAtRev
}

// Descriptors returns a copy of the descriptor slice (safe for callers to mutate the slice header).
func (m *DescriptorReadModel) Descriptors() []CellKindDescriptor {
	if m == nil || len(m.descriptors) == 0 {
		return nil
	}
	return slices.Clone(m.descriptors)
}

// DescriptorForKind returns one descriptor when present.
func (m *DescriptorReadModel) DescriptorForKind(kind string) (CellKindDescriptor, bool) {
	if m == nil || kind == "" || m.byKind == nil {
		return CellKindDescriptor{}, false
	}
	d, ok := m.byKind[kind]
	return d, ok
}
