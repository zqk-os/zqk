package objects

import (
	"reflect"
	"testing"
)

func TestHybridMaskGlobalAndExtension(t *testing.T) {
	mask := HybridMaskFromFields(KindBacklogItem, []string{
		FieldKeyID,
		FieldKeyUpdatedAt,
		FieldKeyPriorityTier,
	})
	if mask.Global&(1<<0) == 0 {
		t.Fatal("expected id global bit")
	}
	if mask.Global&(1<<6) == 0 {
		t.Fatal("expected updated_at global bit")
	}
	ext := kindExtensionKeys(KindBacklogItem)
	idx := -1
	for i, k := range ext {
		if k == FieldKeyPriorityTier {
			idx = i
			break
		}
	}
	if idx < 0 {
		t.Fatal("priority_tier not in backlog extension table")
	}
	if mask.Ext&(uint32(1)<<idx) == 0 {
		t.Fatal("expected extension bit for priority_tier")
	}
}

func TestHybridMaskOverflowUnknownField(t *testing.T) {
	mask := HybridMaskFromFields(KindBacklogItem, []string{"custom_extension_field_xyz"})
	if len(mask.Overflow) != 1 || mask.Overflow[0] != "custom_extension_field_xyz" {
		t.Fatalf("overflow %#v", mask.Overflow)
	}
}

func TestProjectMapHybrid(t *testing.T) {
	src := map[string]any{
		FieldKeyID:        "B-1",
		FieldKeyKind:      KindBacklogItem,
		FieldKeyTitle:     "x",
		FieldKeyUpdatedAt: "t1",
		"noise":           1,
	}
	mask := HybridMaskFromFields(KindBacklogItem, []string{FieldKeyID, FieldKeyUpdatedAt})
	got := ProjectMapHybrid(src, mask)
	if len(got) != 2 || got[FieldKeyID] != "B-1" {
		t.Fatalf("%#v", got)
	}
	if _, ok := got["noise"]; ok {
		t.Fatal("expected noise stripped")
	}
}

func TestListProjectionFieldNames_includesSortBy(t *testing.T) {
	got := ListProjectionFieldNames([]string{FieldKeyID}, FieldKeyUpdatedAt)
	if !reflect.DeepEqual(got, []string{FieldKeyID, FieldKeyUpdatedAt}) {
		t.Fatalf("%#v", got)
	}
}

func TestHybridMaskForList_sortAddsUpdatedAtBit(t *testing.T) {
	mask := HybridMaskForList(KindBacklogItem, []string{FieldKeyID}, FieldKeyUpdatedAt)
	if mask.Global&(1<<0) == 0 || mask.Global&(1<<6) == 0 {
		t.Fatalf("global %#x want id+updated_at bits", mask.Global)
	}
}
