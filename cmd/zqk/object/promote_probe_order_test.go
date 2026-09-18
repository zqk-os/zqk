package object

import (
	"slices"
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
)

func TestPromoteForwardProbeOrder_DropsCompleteWhenInProgressExists(t *testing.T) {
	got := promoteForwardProbeOrder(objects.ObjectStatusDraft, []string{objects.ObjectStatusInProgress, objects.ObjectStatusComplete})
	want := []string{objects.ObjectStatusInProgress}
	if !slices.Equal(got, want) {
		t.Fatalf("promoteForwardProbeOrder = %#v, want %#v", got, want)
	}
}

func TestPromoteForwardProbeOrder_KeepsCompleteWhenItIsTheOnlyHop(t *testing.T) {
	got := promoteForwardProbeOrder(objects.ObjectStatusDraft, []string{objects.ObjectStatusComplete})
	want := []string{objects.ObjectStatusComplete}
	if !slices.Equal(got, want) {
		t.Fatalf("promoteForwardProbeOrder = %#v, want %#v", got, want)
	}
}

func TestPromoteForwardProbeOrder_KeepsCompleteAfterSealHop(t *testing.T) {
	got := promoteForwardProbeOrder(objects.ObjectStatusDraft, []string{objects.ObjectStatusActive, objects.ObjectStatusComplete})
	want := []string{objects.ObjectStatusActive, objects.ObjectStatusComplete}
	if !slices.Equal(got, want) {
		t.Fatalf("promoteForwardProbeOrder = %#v, want %#v", got, want)
	}
}

func TestPromoteForwardProbeOrder_DropsCompleteWhenExecutionFollowsSeal(t *testing.T) {
	got := promoteForwardProbeOrder(objects.ObjectStatusDraft, []string{
		objects.ObjectStatusActive,
		objects.ObjectStatusInProgress,
		objects.ObjectStatusComplete,
	})
	want := []string{objects.ObjectStatusActive, objects.ObjectStatusInProgress}
	if !slices.Equal(got, want) {
		t.Fatalf("promoteForwardProbeOrder = %#v, want %#v", got, want)
	}
}

func TestPromoteForwardProbeOrder_KeepsCompleteFromTestingOrInProgress(t *testing.T) {
	for _, st := range []string{"testing", objects.ObjectStatusInProgress} {
		input := []string{objects.ObjectStatusInProgress, objects.ObjectStatusComplete}
		got := promoteForwardProbeOrder(st, input)
		if !slices.Equal(got, input) {
			t.Fatalf("currentStatus=%s promoteForwardProbeOrder = %#v, want %#v", st, got, input)
		}
	}
}

