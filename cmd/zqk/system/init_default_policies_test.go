package system

import (
	"os"
	"path/filepath"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestSeedDefaultPolicyPack_LandsActiveOnCAS(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		Kind:            "system.seed_default_policy_pack",
		SeedSchemaPlane: true,
	})
	dir := filepath.Join(proj.Root, "scripts", "default_policies")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	yaml := []byte(`kind: policy
schema_version: "2.0.0"
title: Fail-Closed Safety Seed Test
status: active
category: workflow
policy_type: requirement
body: |
  Seed must land shovel-ready (active/enforced) on CAS, not draft originated.
effective_date: "2026-09-17"
enforcement:
  automated: true
  reminder_enabled: true
  severity: critical
`)
	if err := fileutil.WriteFile(filepath.Join(dir, "fail_closed.yaml"), yaml, 0o644); err != nil {
		t.Fatal(err)
	}
	created, err := SeedDefaultPolicyPack(proj.Root, nil)
	if err != nil {
		t.Fatalf("SeedDefaultPolicyPack: %v", err)
	}
	if created != 1 {
		t.Fatalf("created=%d want 1", created)
	}
	factory, err := storage.NewStorageFactory(pkgctx.NewSystemContext(), proj.Root)
	if err != nil {
		t.Fatal(err)
	}
	obj, err := factory.GetStorage().Read(pkgctx.NewSystemContext(), pkgctx.NewSystemSecurityContext(), stableDefaultPolicyID("Fail-Closed Safety Seed Test"))
	if err != nil {
		t.Fatalf("read seeded policy: %v", err)
	}
	if got := objects.GetString(obj, objects.FieldKeyStatus); got != objects.ObjectStatusActive {
		t.Fatalf("status=%q want active (CAS shovel-ready / enforced)", got)
	}
	drafts := filepath.Join(proj.Root, ".zqk", "object_drafts", "policy")
	if ents, _ := os.ReadDir(drafts); len(ents) > 0 {
		t.Fatalf("policy parked on draft plane: %v", ents)
	}
}
