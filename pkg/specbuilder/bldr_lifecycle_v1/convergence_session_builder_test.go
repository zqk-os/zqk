package bldr_lifecycle_v1

import (
	"slices"
	"testing"

	"github.com/lanceman/zqk/pkg/convergence"
)

func TestConvergenceSessionLifecycleBuilder_DraftToActiveAdmission(t *testing.T) {
	t.Parallel()
	lc := NewConvergenceSessionLifecycleBuilder().Build()
	var got []string
	for _, tr := range lc.Transitions {
		if tr.From == "draft" && tr.To == "active" {
			got = tr.Preconditions
			break
		}
	}
	if got == nil {
		t.Fatal("missing draft→active transition")
	}
	want := convergence.DraftToActivePreconditions()
	if !slices.Equal(got, want) {
		t.Fatalf("builder preconditions = %#v, want %#v", got, want)
	}
}
