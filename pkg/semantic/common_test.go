package semantic

import (
	"context"

	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
)

type mockStorage struct {
	storage.ObjectStorageProvider
	objects []map[string]any
}

func (m *mockStorage) Create(_ context.Context, _ *storage.SecurityContext, obj map[string]any) error {
	m.objects = append(m.objects, obj)
	return nil
}

func (m *mockStorage) Update(_ context.Context, _ *storage.SecurityContext, id string, updates map[string]any) error {
	for i, obj := range m.objects {
		if obj[objects.FieldKeyID] == id {
			for k, v := range updates {
				m.objects[i][k] = v
			}
			return nil
		}
	}
	return storage.ErrObjectNotFound
}

func (m *mockStorage) List(_ context.Context, _ *storage.SecurityContext, _ *storage.StorageContext, filter storage.ListFilter) (*storage.QueryResult, error) {
	var result []map[string]any
	for _, obj := range m.objects {
		if obj[objects.FieldKeyKind] == filter.Kind {
			// Basic status filtering
			if status, ok := filter.Filters[objects.FieldKeyStatus]; ok {
				if obj[objects.FieldKeyStatus] == status {
					result = append(result, obj)
				}
			} else {
				result = append(result, obj)
			}
		}
	}
	return &storage.QueryResult{Objects: result}, nil
}

func (m *mockStorage) Shutdown(context.Context) error { return nil }
