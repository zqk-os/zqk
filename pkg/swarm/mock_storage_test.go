package swarm

import (
	"context"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

type mockStorage struct {
	storage.ObjectStorageProvider
	data map[string]map[string]any
}

func newMockStorage() *mockStorage {
	return &mockStorage{
		data: make(map[string]map[string]any),
	}
}

func (m *mockStorage) Exists(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (bool, error) {
	_, ok := m.data[id]
	return ok, nil
}

func (m *mockStorage) Create(ctx context.Context, secCtx *pkgctx.SecurityContext, obj map[string]any) error {
	id, _ := obj[objects.FieldKeyID].(string)
	if id == "" {
		id, _ = obj["id"].(string)
	}
	m.data[id] = obj
	return nil
}

func (m *mockStorage) Read(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
	obj, ok := m.data[id]
	if !ok {
		return nil, storage.ErrObjectNotFound
	}
	return obj, nil
}

func (m *mockStorage) Update(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, updates map[string]any) error {
	obj, ok := m.data[id]
	if !ok {
		return storage.ErrObjectNotFound
	}
	for k, v := range updates {
		obj[k] = v
	}
	m.data[id] = obj
	return nil
}

func (m *mockStorage) List(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter storage.ListFilter) (*storage.QueryResult, error) {
	var results []map[string]any
	for _, obj := range m.data {
		if filter.Kind != "" {
			kind, _ := obj[objects.FieldKeyKind].(string)
			if kind == "" {
				kind, _ = obj["kind"].(string)
			}
			if kind != filter.Kind {
				continue
			}
		}

		match := true
		for k, v := range filter.Filters {
			if filterMap, isMap := v.(map[string]any); isMap {
				if neVal, hasNe := filterMap["$ne"]; hasNe {
					if obj[k] == neVal {
						match = false
						break
					}
				}
				if inList, hasIn := filterMap["$in"].([]any); hasIn {
					inMatch := false
					for _, inVal := range inList {
						if obj[k] == inVal {
							inMatch = true
							break
						}
					}
					if !inMatch {
						match = false
						break
					}
				}
			} else if obj[k] != v {
				match = false
				break
			}
		}
		if match {
			results = append(results, obj)
		}
	}
	return &storage.QueryResult{
		Objects: results,
	}, nil
}
