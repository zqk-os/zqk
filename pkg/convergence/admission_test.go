package convergence

import (
	"slices"
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
)

func TestDraftToActivePreconditions_MinimalContract(t *testing.T) {
	t.Parallel()
	got := DraftToActivePreconditions()
	want := []string{
		objects.FieldKeyHypothesis + " is set",
		objects.FieldKeyDesiredEndState + " is set",
		objects.FieldKeyCurrentPhase + " is set",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
	for _, s := range got {
		if s == objects.FieldKeyPredictions+" is set" {
			t.Fatal("do not require predictions at admission")
		}
	}
}
