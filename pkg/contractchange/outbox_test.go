package contractchange

import (
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestHasDispatchIdentity(t *testing.T) {
	t.Parallel()
	if HasDispatchIdentity(nil) {
		t.Fatal("nil")
	}
	if HasDispatchIdentity(map[string]any{objects.FieldKeyTitle: "x"}) {
		t.Fatal("neither")
	}
	if !HasDispatchIdentity(map[string]any{objects.FieldKeyTeamConfigurationRef: "TC-1"}) {
		t.Fatal("team")
	}
	if !HasDispatchIdentity(map[string]any{objects.FieldKeyPersonaRefs: []string{"PER-1"}}) {
		t.Fatal("persona")
	}
}

func TestEmitAndListPending(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	lcDir := filepath.Join(root, paths.ProcessInternalLifecyclesDir)
	if err := fileutil.MkdirAll(lcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "object_type: priority_plan\nstatuses:\n  - value: active\n    stay_in_status:\n      - at least one team_configuration_ref or persona_refs\n"
	if err := fileutil.WriteFile(filepath.Join(lcDir, "priority_plan_lifecycle.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := EmitForKind(root, "priority_plan", "test"); err != nil {
		t.Fatal(err)
	}
	pending, err := ListPending(root)
	if err != nil || len(pending) != 1 {
		t.Fatalf("pending=%v err=%v", pending, err)
	}
	// idempotent same fingerprint
	if err := EmitForKind(root, "priority_plan", "test"); err != nil {
		t.Fatal(err)
	}
	pending, _ = ListPending(root)
	if len(pending) != 1 {
		t.Fatalf("want 1 after dup emit, got %d", len(pending))
	}
	if err := MarkConsumed(root, []string{pending[0].ID}); err != nil {
		t.Fatal(err)
	}
	pending, _ = ListPending(root)
	if len(pending) != 0 {
		t.Fatalf("want 0 after consume, got %d", len(pending))
	}
}
