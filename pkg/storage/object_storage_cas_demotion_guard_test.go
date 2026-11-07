package storage

import (
	"fmt"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"gopkg.in/yaml.v3"
)

// TRACK: BLI-1794075198197653000-dc625b46 — CAS-resident updates must not demote to draft_plane
// when description membrane park would otherwise fire (MIL-008 complete-milestone hazard).
func TestUpdate_CASResidentWithoutDescriptionDoesNotDemote(t *testing.T) {
	tmpDir, fileStorage, _ := SetupTestingFactoryCompleteTestEnvironmentForTest(t)
	_ = tmpDir
	secCtx := pkgctx.NewSystemSecurityContext()
	ctx := pkgctx.NewSystemContext()
	updateCtx := pkgctx.WithLifecycleBreakGlass(
		pkgctx.WithAllowCoreObjectDelete(ctx),
		"cas demotion guard: title update on grandfathered empty-description CAS blob",
	)

	id := fmt.Sprintf("BLI-demote-guard-%d", time.Now().UnixNano())
	obj := map[string]any{
		objects.FieldKeyID:            id,
		objects.FieldKeyKind:          objects.KindBacklogItem,
		objects.FieldKeyTitle:         "CAS demotion guard backlog item",
		objects.FieldKeyDescription:   "Substantive description so CreateCASVisible can materialize into CAS.",
		objects.FieldKeyStatus:        objects.ObjectStatusValidated,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}
	CreateCASVisible(t, fileStorage, ctx, secCtx, obj, objects.ObjectStatusValidated)
	if fileStorage.objectDraftPlaneExists(objects.KindBacklogItem, id) {
		t.Fatal("setup: backlog_item must be CAS-resident, not draft")
	}

	cas, err := fileStorage.getContentAddressableStorage(objects.KindBacklogItem)
	if err != nil || cas == nil {
		t.Fatalf("getContentAddressableStorage: %v", err)
	}
	raw, err := cas.Read(id)
	if err != nil {
		t.Fatalf("cas.Read: %v", err)
	}
	var planted map[string]any
	if err := yaml.Unmarshal(raw, &planted); err != nil {
		t.Fatalf("unmarshal planted: %v", err)
	}
	delete(planted, objects.FieldKeyDescription)
	planted[objects.FieldKeyDescription] = ""
	rewritten, err := yaml.Marshal(planted)
	if err != nil {
		t.Fatalf("marshal stripped: %v", err)
	}
	// Bypass membrane so we can simulate a grandfathered CAS blob with empty description.
	if err := cas.Update(id, rewritten); err != nil {
		t.Fatalf("plant empty-description CAS blob: %v", err)
	}
	fileStorage.InvalidateCachesForKind(objects.KindBacklogItem)
	fileStorage.InvalidateCASCacheForKind(objects.KindBacklogItem)

	err = fileStorage.Update(updateCtx, secCtx, id, map[string]any{
		objects.FieldKeyTitle: "CAS demotion guard backlog item (retitled)",
	})
	if fileStorage.objectDraftPlaneExists(objects.KindBacklogItem, id) {
		t.Fatalf("CAS-resident update must not demote to draft_plane (err=%v)", err)
	}
	if _, getErr := cas.GetHashForID(id); getErr != nil {
		t.Fatalf("CAS id→hash must survive update attempt: %v (update err=%v)", getErr, err)
	}
}
