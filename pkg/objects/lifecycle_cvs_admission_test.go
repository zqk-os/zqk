package objects_test

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/lanceman/zqk/pkg/convergence"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
)

func TestLifecycleLoader_CVSDraftToActiveAdmissionContract(t *testing.T) {
	t.Parallel()
	lifecyclesDir := filepath.Join("..", "..", paths.ProcessInternalLifecyclesDir)
	loader := objects.NewLifecycleLoader(lifecyclesDir)

	got, err := loader.GetTransitionPreconditions(objects.KindConvergenceSession, "draft", "active")
	if err != nil {
		t.Fatalf("GetTransitionPreconditions(draft, active): %v", err)
	}
	want := convergence.DraftToActivePreconditions()
	if !slices.Equal(got, want) {
		t.Fatalf("draft→active preconditions = %#v, want %#v", got, want)
	}
}
