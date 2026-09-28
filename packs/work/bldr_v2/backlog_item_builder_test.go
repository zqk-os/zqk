package bldr_v2_test

import (
	"testing"

	"github.com/zqk-os/zqk/packs/work/bldr_v2"
)

func TestBacklogItemBuilder_CodeLocation(t *testing.T) {
	b := bldr_v2.NewBacklogItemBuilder()
	spec := b.Build()

	if _, ok := spec.Fields["code_location"]; !ok {
		t.Errorf("Expected 'code_location' field on backlog_item spec")
	}
}
