package storage

import (
	"context"
	"errors"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
)

type mockProviderForOrgAdapter struct {
	ObjectStorageProvider
	readFunc   func(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error)
	createFunc func(ctx context.Context, secCtx *pkgctx.SecurityContext, obj map[string]any) error
	updateFunc func(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, updates map[string]any) error
	listFunc   func(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter ListFilter) (*QueryResult, error)
}

func (m *mockProviderForOrgAdapter) Read(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
	if m.readFunc != nil {
		return m.readFunc(ctx, secCtx, id)
	}
	return nil, nil
}

func (m *mockProviderForOrgAdapter) Create(ctx context.Context, secCtx *pkgctx.SecurityContext, obj map[string]any) error {
	if m.createFunc != nil {
		return m.createFunc(ctx, secCtx, obj)
	}
	return nil
}

func (m *mockProviderForOrgAdapter) Update(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, updates map[string]any) error {
	if m.updateFunc != nil {
		return m.updateFunc(ctx, secCtx, id, updates)
	}
	return nil
}

func (m *mockProviderForOrgAdapter) List(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter ListFilter) (*QueryResult, error) {
	if m.listFunc != nil {
		return m.listFunc(ctx, secCtx, storageCtx, filter)
	}
	return &QueryResult{}, nil
}

func TestOrganizationalStorageAdapter_Operations(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	t.Run("Read success and error", func(t *testing.T) {
		mock := &mockProviderForOrgAdapter{
			readFunc: func(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
				if id == "found" {
					return map[string]any{objects.FieldKeyID: "found"}, nil
				}
				return nil, errors.New("not found")
			},
		}
		adapter := NewOrganizationalStorageAdapter(mock)

		obj, err := adapter.Read(ctx, secCtx, "found")
		if err != nil || obj[objects.FieldKeyID] != "found" {
			t.Fatalf("expected found object, got %v, err %v", obj, err)
		}

		_, err = adapter.Read(ctx, secCtx, "missing")
		if err == nil {
			t.Fatal("expected error for missing object, got nil")
		}
	})

	t.Run("Create success and error", func(t *testing.T) {
		mock := &mockProviderForOrgAdapter{
			createFunc: func(ctx context.Context, secCtx *pkgctx.SecurityContext, obj map[string]any) error {
				if obj["fail"] == true {
					return errors.New("cannot create")
				}
				return nil
			},
		}
		adapter := NewOrganizationalStorageAdapter(mock)

		if err := adapter.Create(ctx, secCtx, map[string]any{"id": "ok"}); err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if err := adapter.Create(ctx, secCtx, map[string]any{"fail": true}); err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("Update success and error", func(t *testing.T) {
		mock := &mockProviderForOrgAdapter{
			updateFunc: func(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, updates map[string]any) error {
				if id == "fail" {
					return errors.New("cannot update")
				}
				return nil
			},
		}
		adapter := NewOrganizationalStorageAdapter(mock)

		if err := adapter.Update(ctx, secCtx, "ok", map[string]any{}); err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if err := adapter.Update(ctx, secCtx, "fail", map[string]any{}); err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("ListByKind success and error and nil handling", func(t *testing.T) {
		mock := &mockProviderForOrgAdapter{
			listFunc: func(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter ListFilter) (*QueryResult, error) {
				if filter.Kind == "error_kind" {
					return nil, errors.New("list failed")
				}
				if filter.Kind == "nil_result" {
					return nil, nil
				}
				return &QueryResult{
					Objects: []map[string]any{
						{objects.FieldKeyID: "obj1", objects.FieldKeyKind: filter.Kind},
					},
				}, nil
			},
		}
		adapter := NewOrganizationalStorageAdapter(mock)

		objs, err := adapter.ListByKind(ctx, secCtx, "my_kind")
		if err != nil || len(objs) != 1 || objs[0][objects.FieldKeyID] != "obj1" {
			t.Fatalf("expected 1 object, got %v, err %v", objs, err)
		}

		_, err = adapter.ListByKind(ctx, secCtx, "error_kind")
		if err == nil {
			t.Fatal("expected error, got nil")
		}

		nilObjs, err := adapter.ListByKind(ctx, secCtx, "nil_result")
		if err != nil || len(nilObjs) != 0 {
			t.Fatalf("expected empty slice on nil result, got %v, err %v", nilObjs, err)
		}
	})
}
