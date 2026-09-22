// BLI-STARTER-COMMUNITY-036 / PRI-STARTER-COMMUNITY-036 coverage elevation
package rollup

import (
	"context"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

type listStub struct {
	storage.NoopObjectStorage
	objs []map[string]any
}

func (l listStub) List(context.Context, *pkgctx.SecurityContext, *pkgctx.StorageContext, storage.ListFilter) (*storage.QueryResult, error) {
	return &storage.QueryResult{Objects: l.objs}, nil
}

func (l listStub) BulkGet(context.Context, *pkgctx.SecurityContext, []string) (*storage.BulkResult, error) {
	return &storage.BulkResult{Results: l.objs}, nil
}

func TestFetchChildrenAndToMap(t *testing.T) {
	t.Parallel()
	got, err := FetchChildrenForParent(context.Background(), nil, nil, objects.KindMilestone, "")
	if err != nil || got != nil {
		t.Fatalf("%v %v", got, err)
	}
	msID := "MS-1"
	kids := []map[string]any{
		{objects.FieldKeyID: "BLI-1", objects.FieldKeyKind: objects.KindBacklogItem, objects.FieldKeyMilestoneRef: msID},
		{objects.FieldKeyID: "BLI-2", objects.FieldKeyKind: objects.KindBacklogItem, objects.FieldKeyMilestoneRefs: []string{msID}},
		nil,
		{objects.FieldKeyID: ""},
	}
	out, err := FetchChildrenForParent(context.Background(), pkgctx.NewSystemSecurityContext(), listStub{objs: kids}, objects.KindMilestone, msID)
	if err != nil || len(out) < 1 {
		t.Fatalf("%v %v", out, err)
	}
	planKids := []map[string]any{
		{objects.FieldKeyID: "BLI-p", objects.FieldKeyKind: objects.KindBacklogItem, objects.FieldKeyPriorityPlanRef: "PRI-1"},
		{objects.FieldKeyID: "GOAL-x", objects.FieldKeyKind: objects.KindGoal, objects.FieldKeyPriorityPlanRef: "PRI-1"},
	}
	out, err = FetchChildrenForParent(context.Background(), pkgctx.NewSystemSecurityContext(), listStub{objs: planKids}, objects.KindPriorityPlan, "PRI-1")
	if err != nil || len(out) < 1 {
		t.Fatalf("plan kids %v %v", out, err)
	}
	generic, err := FetchChildrenForParent(context.Background(), pkgctx.NewSystemSecurityContext(), listStub{objs: []map[string]any{
		{objects.FieldKeyID: "C-1", "parent": "P-9"},
		{objects.FieldKeyID: "C-2", "parents": []string{"P-9"}},
	}}, "custom", "P-9")
	if err != nil || len(generic) < 1 {
		t.Fatalf("generic %v %v", generic, err)
	}
	if !isChildOfParent(map[string]any{"ref": "X-1"}, "custom", "X-1") {
		t.Fatal("generic string ref")
	}
	if !isChildOfParent(map[string]any{"refs": []any{"X-1"}}, "custom", "X-1") {
		t.Fatal("generic any slice")
	}
	if extractStringSlice("a")[0] != "a" {
		t.Fatal("string slice")
	}
	sum := Calculate([]map[string]any{nil, {objects.FieldKeyStatus: ""}})
	if sum.ToMap()["total_count"] != 2 {
		t.Fatalf("%v", sum.ToMap())
	}
}
