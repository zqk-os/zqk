package zqksession

import (
	"context"
	"errors"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

type mockSessionStorage struct {
	storage.ObjectStorageProvider
	created        []map[string]any
	promotedCreate bool
	readable       map[string]map[string]any
}

func (s *mockSessionStorage) Read(_ context.Context, _ *storage.SecurityContext, id string) (map[string]any, error) {
	if s.readable != nil {
		if obj, ok := s.readable[id]; ok {
			return obj, nil
		}
	}
	return nil, errors.New("not found")
}

func (s *mockSessionStorage) Create(
	ctx context.Context,
	_ *storage.SecurityContext,
	obj map[string]any,
) error {
	s.promotedCreate = pkgctx.GetPromoteOnCreate(ctx)
	s.created = append(s.created, obj)
	return nil
}

func TestStartCLISession_SetsPromoteOnCreate(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := &mockSessionStorage{}
	ctx := pkgctx.NewSystemContext()

	sessionID := StartCLISession(ctx, tmpDir, "test-title", pkgctx.SystemAccountID, store)
	if sessionID == EmptyValue {
		t.Fatal("expected non-empty session ID")
	}
	if !store.promotedCreate {
		t.Errorf("expected Create to be invoked with promoteOnCreate context flag")
	}
	if len(store.created) != 1 {
		t.Fatalf("expected 1 created session object, got %d", len(store.created))
	}
	created := store.created[0]
	if status, _ := created[objects.FieldKeyStatus].(string); status != StatusActive {
		t.Errorf("expected status %q, got %q", StatusActive, status)
	}
}

func TestTryReuse_ValidActiveSession(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	sessionID := "ZQK-TEST-999"
	store := &mockSessionStorage{
		readable: map[string]map[string]any{
			sessionID: {
				objects.FieldKeyStatus:    StatusActive,
				objects.FieldKeyAccountID: pkgctx.SystemAccountID,
			},
		},
	}
	ctx := pkgctx.NewSystemContext()

	ok := WritePersistedID(ctx, tmpDir, sessionID, pkgctx.SystemAccountID, store)
	if !ok {
		t.Fatalf("WritePersistedID failed")
	}

	gotID, reused := TryReuse(ctx, tmpDir, "cmd", pkgctx.SystemAccountID, store)
	if !reused || gotID != sessionID {
		t.Fatalf("TryReuse: got (%q, %v), want (%q, true)", gotID, reused, sessionID)
	}
}
