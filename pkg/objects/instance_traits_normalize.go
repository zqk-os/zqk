package objects

import (
	"slices"
	"sort"

	"github.com/zqk-os/zqk/pkg/nildecode"
)

// MaybeStripRedundantTopLevelTraits removes top-level obj["traits"] when its expanded trait set
// equals the object spec's ResolvedTraits expanded set (order-insensitive). This reduces YAML
// duplication when callers echo spec trait groups or fully expanded leaves that add no information.
//
// No-op when traits are absent, spec load fails, or trait registry cannot expand consistently.
func MaybeStripRedundantTopLevelTraits(obj map[string]any, specLoader *SpecLoader, tr *TraitRegistry) {
	if obj == nil || specLoader == nil || tr == nil {
		return
	}
	raw, ok := obj["traits"]
	if !ok {
		return
	}
	raw, ok = nildecode.DecodeNonNilPayload[any](raw)
	if !ok {
		return
	}
	kind, _ := obj[FieldKeyKind].(string)
	if kind == emptyValue {
		return
	}
	instTraits := parseInstanceTraitsYAML(raw)
	if len(instTraits) == 0 {
		return
	}
	spec, err := loadSpecForInstancePersistence(specLoader, kind, obj)
	if err != nil || spec == nil || len(spec.ResolvedTraits) == 0 {
		return
	}
	expInst, _ := tr.ExpandTraits(instTraits)
	expSpec, _ := tr.ExpandTraits(spec.ResolvedTraits)
	sort.Strings(expInst)
	sort.Strings(expSpec)
	if slices.Equal(expInst, expSpec) {
		delete(obj, "traits")
	}
}

func loadSpecForInstancePersistence(sl *SpecLoader, kind string, obj map[string]any) (*Spec, error) {
	if sv, ok := obj[FieldKeySchemaVersion].(string); ok && sv != emptyValue {
		return sl.LoadSpecByVersion(kind, sv)
	}
	return sl.LoadSpecWithInheritance(kind + ".yaml")
}

func parseInstanceTraitsYAML(raw any) []string {
	switch v := raw.(type) {
	case []string:
		return append([]string(nil), v...)
	case []any:
		var out []string
		for _, x := range v {
			if s, ok := x.(string); ok && s != emptyValue {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}
