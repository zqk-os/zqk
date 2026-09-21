package storage

import (
	"context"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
)

type mockStorageForDependents struct {
	ObjectStorageProvider
	listFn func(ctx context.Context, filter ListFilter) (*QueryResult, error)
}

func (m *mockStorageForDependents) List(ctx context.Context, secCtx *pkgctx.SecurityContext, stCtx *pkgctx.StorageContext, filter ListFilter) (*QueryResult, error) {
	if m.listFn != nil {
		return m.listFn(ctx, filter)
	}
	return &QueryResult{}, nil
}

func TestDependentsForID_CriteriaFallback(t *testing.T) {
	mock := &mockStorageForDependents{
		listFn: func(ctx context.Context, filter ListFilter) (*QueryResult, error) {
			if filter.Kind == objects.KindTestCase {
				return &QueryResult{
					Objects: []map[string]any{
						{
							objects.FieldKeyID:           "TST-001",
							objects.FieldKeyCriteriaRefs: []string{"CRIT-100", "CRIT-200"},
						},
						{
							objects.FieldKeyID:           "TST-002",
							objects.FieldKeyCriteriaRefs: []any{"CRIT-200", "CRIT-300"},
						},
						{
							objects.FieldKeyID: "TST-003",
							"criteria_ref":     "CRIT-100",
						},
					},
				}, nil
			}
			return &QueryResult{}, nil
		},
	}

	deps := DependentsForID(context.Background(), mock, "CRIT-100")
	if len(deps) != 2 {
		t.Fatalf("expected 2 dependents for CRIT-100, got %d: %v", len(deps), deps)
	}
	if deps[0] != "TST-001" || deps[1] != "TST-003" {
		t.Errorf("unexpected dependents: %v", deps)
	}

	deps2 := DependentsForID(context.Background(), mock, "CRIT-200")
	if len(deps2) != 2 {
		t.Fatalf("expected 2 dependents for CRIT-200, got %d: %v", len(deps2), deps2)
	}
}

func TestDependentsForID_WrappedStorageUnwrap(t *testing.T) {
	tempDir := t.TempDir()
	fs, err := NewFileObjectStorageForTest(tempDir)
	if err != nil {
		t.Fatalf("failed to create test file storage: %v", err)
	}
	hybrid := NewHybridObjectStorage(fs, nil)

	// Clear bound root before test
	revRefBoundRoot.Store("")

	_ = DependentsForID(context.Background(), hybrid, "PRI-TEST-999")

	bound := reverseReferenceBoundProjectRoot()
	if bound != tempDir {
		t.Errorf("expected bound project root %q, got %q", tempDir, bound)
	}
}
