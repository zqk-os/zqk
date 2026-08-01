package cli

import (
	"context"
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/zqkenv"
)

type mockOverrideStorage struct {
	createdObj map[string]any
}

func (m *mockOverrideStorage) Create(ctx context.Context, secCtx *pkgctx.SecurityContext, obj map[string]any) error {
	m.createdObj = obj
	return nil
}

func TestEnforceOverrideFriction(t *testing.T) {
	cmd := NewCommandBuilder("test").Build()
	ctx := context.Background()
	secCtx := &pkgctx.SecurityContext{AccountID: "account:system"}
	store := &mockOverrideStorage{}

	// 1. Short justification fails
	err := EnforceOverrideFriction(cmd, ctx, secCtx, store, "TEST-1", "backlog_item", "short reason")
	if err == nil {
		t.Error("expected error for short justification, got nil")
	}

	// 2. Long justification passes and records technical debt
	reason := "This is a long descriptive reason containing at least five words and thirty characters to bypass the lifecycle rules."
	err = EnforceOverrideFriction(cmd, ctx, secCtx, store, "TEST-1", "backlog_item", reason)
	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}

	if store.createdObj == nil {
		t.Fatal("expected technical debt object to be created, got nil")
	}
	if store.createdObj[objects.FieldKeyDebtType] != "process" {
		t.Errorf("expected debt_type process, got %v", store.createdObj[objects.FieldKeyDebtType])
	}
	if store.createdObj[objects.FieldKeyBacklogRef] != "TEST-1" {
		t.Errorf("expected backlog_ref TEST-1, got %v", store.createdObj[objects.FieldKeyBacklogRef])
	}

	// 3. CI block without allow env var
	t.Setenv("CI", "true")
	t.Setenv(zqkenv.AllowCIOverrides(), "0")
	err = EnforceOverrideFriction(cmd, ctx, secCtx, store, "TEST-1", "backlog_item", reason)
	if err == nil {
		t.Error("expected CI block error, got nil")
	}

	// 4. CI passes with allow env var
	t.Setenv(zqkenv.AllowCIOverrides(), "1")
	err = EnforceOverrideFriction(cmd, ctx, secCtx, store, "TEST-1", "backlog_item", reason)
	if err != nil {
		t.Errorf("expected CI override allowed, got: %v", err)
	}
}
