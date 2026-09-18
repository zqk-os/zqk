package storage

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
)

func TestListProjectionMask_includesGroupBy(t *testing.T) {
	m := listProjectionMask(objects.KindBacklogItem, ListFilter{
		Kind:    objects.KindBacklogItem,
		Fields:  []string{objects.FieldKeyID},
		GroupBy: objects.FieldKeyStatus,
	})
	// id = global 0, status = global 3
	if m.Global&(1<<0) == 0 || m.Global&(1<<3) == 0 {
		t.Fatalf("expected id+status global bits, got global=%#x", m.Global)
	}
}
