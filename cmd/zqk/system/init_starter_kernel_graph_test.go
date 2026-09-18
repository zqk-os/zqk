package system

import (
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
)

func TestSeedStarterKernelGraph_LandsShovelReadyOnCAS(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		Kind:            "system.seed_starter_kernel_graph",
		SeedSchemaPlane: true,
	})
	if _, err := SeedDefaultAgentSeatingPack(proj.Root, nil); err != nil {
		t.Fatalf("SeedDefaultAgentSeatingPack: %v", err)
	}
	created, err := SeedStarterKernelGraph(proj.Root, nil)
	if err != nil {
		t.Fatalf("SeedStarterKernelGraph: %v", err)
	}
	if created < 1 {
		t.Fatalf("created=%d want >=1", created)
	}
	factory, err := storage.NewStorageFactory(pkgctx.NewSystemContext(), proj.Root)
	if err != nil {
		t.Fatal(err)
	}
	sp := factory.GetStorage()
	sec := pkgctx.NewSystemSecurityContext()
	ctx := pkgctx.NewSystemContext()

	want := map[string]string{
		starterOrgID:  objects.ObjectStatusActive,
		starterMisID:  objects.ObjectStatusActive,
		starterVisID:  objects.ObjectStatusActive,
		starterGoalID: objects.ObjectStatusActive,
		starterWsID:   objects.ObjectStatusActive,
		starterPriID:  objects.ObjectStatusActive,
		starterReqID:  objects.ObjectStatusActive,
		starterCritID: objects.ObjectStatusAwaitingVerification,
		starterBliID:  objects.ObjectStatusPlanned,
		starterMilID:  objects.ObjectStatusNotStarted,
	}
	for id, status := range want {
		obj, rerr := sp.Read(ctx, sec, id)
		if rerr != nil {
			t.Fatalf("read %s: %v", id, rerr)
		}
		if got := objects.GetString(obj, objects.FieldKeyStatus); got != status {
			t.Errorf("%s status=%q want %s", id, got, status)
		}
	}
	again, err := SeedStarterKernelGraph(proj.Root, nil)
	if err != nil {
		t.Fatalf("idempotent SeedStarterKernelGraph: %v", err)
	}
	if again != 0 {
		t.Fatalf("second seed created=%d want 0", again)
	}
}
