package mcp

import (
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
)

func TestHasPermission_UnannotatedFailClosed(t *testing.T) {
	t.Parallel()
	unannotated := &DiscoveredCommand{Use: "mystery", Path: "mystery"}
	annotated := &DiscoveredCommand{
		Use:         "list",
		Path:        "object list",
		Permissions: []string{"read:*"},
	}

	guest := pkgctx.NewSecurityContext("account:guest", []string{"guest"}, []string{})
	viewer := pkgctx.NewSecurityContext("ACC-viewer", []string{"viewer"}, []string{"read:*"})
	admin := pkgctx.NewSecurityContext("account:admin", []string{"admin"}, []string{"read:*"})
	wildcard := pkgctx.NewSecurityContext("account:star", []string{"viewer"}, []string{"*"})

	if hasPermission(unannotated, nil) {
		t.Fatal("nil security context must deny unannotated commands")
	}
	if hasPermission(unannotated, guest) {
		t.Fatal("guest must not see unannotated commands")
	}
	if hasPermission(unannotated, viewer) {
		t.Fatal("viewer must not see unannotated commands (fail-closed)")
	}
	if !hasPermission(unannotated, admin) {
		t.Fatal("admin is explicit allow for unannotated commands")
	}
	if !hasPermission(unannotated, wildcard) {
		t.Fatal("wildcard permission is explicit allow for unannotated commands")
	}
	if !hasPermission(annotated, viewer) {
		t.Fatal("viewer with read:* must see annotated read commands")
	}
	if hasPermission(annotated, guest) {
		t.Fatal("guest without read:* must not see annotated read commands")
	}
}
