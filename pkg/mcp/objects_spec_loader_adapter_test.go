package mcp

import (
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
)

// TestNewSpecAccessControlFromObjectsLoader verifies ITEM-642 adapter: create SAC from objects.SpecLoader.
func TestNewSpecAccessControlFromObjectsLoader(t *testing.T) {
	t.Parallel()
	if NewSpecAccessControlFromObjectsLoader(nil) != nil {
		t.Error("expected nil when loader is nil")
	}

	loader := objects.NewSpecLoader("")
	sac := NewSpecAccessControlFromObjectsLoader(loader)
	if sac == nil {
		t.Fatal("expected non-nil SpecAccessControl when loader is non-nil")
	}

	// With system context, HasFieldAccess allows all; FilterObjectFields returns id/kind at least
	secCtx := pkgctx.NewSystemSecurityContext()
	obj := map[string]any{objects.FieldKeyID: "ITEM-1", objects.FieldKeyKind: "backlog_item", objects.FieldKeyTitle: "Test"}
	filtered := sac.FilterObjectFields(obj, secCtx, "read")
	if filtered[objects.FieldKeyID] != "ITEM-1" || filtered[objects.FieldKeyKind] != "backlog_item" {
		t.Errorf("FilterObjectFields with system context: got %v", filtered)
	}
}

// TestObjectsSpecLoaderAdapter_nilLoader verifies adapter with nil loader returns nil spec without error.
func TestObjectsSpecLoaderAdapter_nilLoader(t *testing.T) {
	a := &ObjectsSpecLoaderAdapter{Loader: nil}
	spec, err := a.LoadSpecWithInheritance("any.yaml")
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
	if spec != nil {
		t.Error("expected nil spec when loader is nil")
	}
}

// TestSpecAccessControlFromObjectsLoader_hasFieldAccess_write verifies write access with system context (ITEM-642).
func TestSpecAccessControlFromObjectsLoader_hasFieldAccess_write(t *testing.T) {
	loader := objects.NewSpecLoader("")
	sac := NewSpecAccessControlFromObjectsLoader(loader)
	if sac == nil {
		t.Fatal("expected non-nil SpecAccessControl")
	}
	secCtx := pkgctx.NewSystemSecurityContext()
	obj := map[string]any{objects.FieldKeyID: "ITEM-1", objects.FieldKeyKind: "backlog_item", objects.FieldKeyTitle: "Test", objects.FieldKeyStatus: "draft"}

	// System context (admin) has write access to all fields
	if !sac.HasFieldAccess("title", obj, secCtx, "write") {
		t.Error("expected HasFieldAccess(title, write) true for system context")
	}
	if !sac.HasFieldAccess("status", obj, secCtx, "write") {
		t.Error("expected HasFieldAccess(status, write) true for system context")
	}
}
