package validate

import (
	"context"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

var _ workflowStorage = (*memoryWorkflowStore)(nil)

// memoryWorkflowStore is a minimal in-memory workflowStorage for isolated selection/enrichment tests.
type memoryWorkflowStore struct {
	byID map[string]map[string]any
}

func newMemoryWorkflowStore(objs ...map[string]any) *memoryWorkflowStore {
	m := &memoryWorkflowStore{byID: make(map[string]map[string]any, len(objs))}
	for _, o := range objs {
		id, _ := o[objects.FieldKeyID].(string)
		if id == "" {
			panic("memoryWorkflowStore: object missing id")
		}
		m.byID[id] = o
	}
	return m
}

func shallowCopyMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func stringInSlice(s string, list []string) bool {
	for _, e := range list {
		if s == e {
			return true
		}
	}
	return false
}

func asStringSliceFromAny(v any) []string {
	switch x := v.(type) {
	case []string:
		return x
	case []any:
		out := make([]string, 0, len(x))
		for _, e := range x {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func fieldMatches(actual any, cond any) bool {
	switch c := cond.(type) {
	case map[string]any:
		if nin, ok := c["$nin"]; ok {
			excl := asStringSliceFromAny(nin)
			st, _ := actual.(string)
			return !stringInSlice(st, excl)
		}
		if inVal, ok := c["$in"]; ok {
			incl := asStringSliceFromAny(inVal)
			st, _ := actual.(string)
			return stringInSlice(st, incl)
		}
		if neVal, ok := c["$ne"]; ok {
			return !valuesEqualWorkflowFilter(actual, neVal)
		}
		return false
	default:
		return valuesEqualWorkflowFilter(actual, c)
	}
}

func valuesEqualWorkflowFilter(a, b any) bool {
	switch av := a.(type) {
	case string:
		bv, ok := b.(string)
		return ok && av == bv
	case float64:
		switch bv := b.(type) {
		case float64:
			return av == bv
		case int:
			return av == float64(bv)
		case int64:
			return av == float64(bv)
		}
	case int:
		switch bv := b.(type) {
		case int:
			return av == bv
		case float64:
			return float64(av) == bv
		}
	}
	return a == b
}

func matchesListFilter(obj map[string]any, lf storage.ListFilter) bool {
	k, _ := obj[objects.FieldKeyKind].(string)
	if k != lf.Kind {
		return false
	}
	for field, want := range lf.Filters {
		if !fieldMatches(obj[field], want) {
			return false
		}
	}
	return true
}

func (m *memoryWorkflowStore) Read(_ context.Context, _ *pkgctx.SecurityContext, id string) (map[string]any, error) {
	obj, ok := m.byID[id]
	if !ok {
		return nil, nil
	}
	return shallowCopyMap(obj), nil
}

func (m *memoryWorkflowStore) List(_ context.Context, _ *pkgctx.SecurityContext, _ *pkgctx.StorageContext, filter storage.ListFilter) (*storage.QueryResult, error) {
	var out []map[string]any
	for _, obj := range m.byID {
		if matchesListFilter(obj, filter) {
			out = append(out, shallowCopyMap(obj))
		}
	}
	return &storage.QueryResult{Objects: out}, nil
}

func (m *memoryWorkflowStore) Count(_ context.Context, _ *pkgctx.SecurityContext, filter storage.ListFilter) (int, error) {
	n := 0
	for _, obj := range m.byID {
		if matchesListFilter(obj, filter) {
			n++
		}
	}
	return n, nil
}

func (m *memoryWorkflowStore) Update(_ context.Context, _ *storage.SecurityContext, id string, data map[string]any) error {
	obj, ok := m.byID[id]
	if !ok {
		return nil // Or an error depending on storage semantics
	}
	for k, v := range data {
		obj[k] = v
	}
	m.byID[id] = obj
	return nil
}
