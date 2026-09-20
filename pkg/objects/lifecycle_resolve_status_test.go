package objects

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
)

func TestResolveStatusForKind_BacklogAliasesAndCanonical(t *testing.T) {
	t.Parallel()
	lifecyclesDir := filepath.Join("..", "..", paths.ProcessInternalLifecyclesDir)
	loader := NewLifecycleLoader(lifecyclesDir)

	cases := []struct {
		name         string
		status       string
		wantValue    string
		wantPrelim   bool
		wantIsPrelim bool
		wantUnknown  bool
	}{
		{name: "canonical conceptual", status: ObjectStatusConceptual, wantValue: ObjectStatusConceptual, wantPrelim: true, wantIsPrelim: true},
		{name: "canonical exploring", status: ObjectStatusExploring, wantValue: ObjectStatusExploring, wantPrelim: false, wantIsPrelim: true},
		{name: "alias proposed", status: ObjectStatusProposed, wantValue: ObjectStatusExploring, wantPrelim: false, wantIsPrelim: true},
		{name: "canonical validated", status: ObjectStatusValidated, wantValue: ObjectStatusValidated, wantPrelim: false, wantIsPrelim: false},
		{name: "canonical roadmap", status: ObjectStatusRoadmap, wantValue: ObjectStatusRoadmap, wantPrelim: false, wantIsPrelim: false},
		{name: "canonical deferred", status: ObjectStatusDeferred, wantValue: ObjectStatusDeferred, wantPrelim: false, wantIsPrelim: false},
		{name: "canonical complete", status: ObjectStatusComplete, wantValue: ObjectStatusComplete, wantPrelim: false, wantIsPrelim: false},
		{name: "alias implemented", status: ObjectStatusImplemented, wantValue: ObjectStatusComplete, wantPrelim: false, wantIsPrelim: false},
		{name: "unknown status", status: "invalid_status", wantUnknown: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := loader.ResolveStatusForKind("backlog_item", tc.status)
			if tc.wantUnknown {
				if err == nil {
					t.Fatalf("ResolveStatusForKind(%q) err=nil, want unknown", tc.status)
				}
				if !errors.Is(err, ErrLifecycleStatusUnknown) {
					t.Fatalf("ResolveStatusForKind(%q) err=%v, want ErrLifecycleStatusUnknown", tc.status, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolveStatusForKind(%q): %v", tc.status, err)
			}
			if got.Value != tc.wantValue {
				t.Errorf("Value=%q, want %q", got.Value, tc.wantValue)
			}
			if got.Preliminary != tc.wantPrelim {
				t.Errorf("Preliminary=%v, want %v", got.Preliminary, tc.wantPrelim)
			}

			prelim, err := loader.IsPreliminaryStatusForKind("backlog_item", tc.status)
			if err != nil {
				t.Fatalf("IsPreliminaryStatusForKind(%q): %v", tc.status, err)
			}
			if prelim != tc.wantIsPrelim {
				t.Errorf("IsPreliminaryStatusForKind(%q)=%v, want %v", tc.status, prelim, tc.wantIsPrelim)
			}
		})
	}
}

func TestNormalizeStatusForKind_UsesStatusMapping(t *testing.T) {
	t.Parallel()
	lifecyclesDir := filepath.Join("..", "..", paths.ProcessInternalLifecyclesDir)
	loader := NewLifecycleLoader(lifecyclesDir)

	got, err := loader.NormalizeStatusForKind("backlog_item", ObjectStatusProposed)
	if err != nil {
		t.Fatalf("NormalizeStatusForKind(proposed): %v", err)
	}
	if got != ObjectStatusExploring {
		t.Fatalf("NormalizeStatusForKind(proposed)=%q, want exploring", got)
	}

	got, err = loader.NormalizeStatusForKind("backlog_item", ObjectStatusImplemented)
	if err != nil {
		t.Fatalf("NormalizeStatusForKind(implemented): %v", err)
	}
	if got != ObjectStatusComplete {
		t.Fatalf("NormalizeStatusForKind(implemented)=%q, want complete", got)
	}

	// Sibling statuses that both exist must not collapse via status_mapping.
	got, err = loader.NormalizeStatusForKind("priority_plan", ObjectStatusInProgress)
	if err != nil {
		t.Fatalf("NormalizeStatusForKind(priority_plan in_progress): %v", err)
	}
	if got != ObjectStatusInProgress {
		t.Fatalf("NormalizeStatusForKind(priority_plan in_progress)=%q, want in_progress", got)
	}
}

func TestIsValidTransition_UnknownFromRepairPark(t *testing.T) {
	t.Parallel()
	lifecyclesDir := filepath.Join("..", "..", paths.ProcessInternalLifecyclesDir)
	loader := NewLifecycleLoader(lifecyclesDir)
	ok, err := loader.IsValidTransition("technical_debt", ObjectStatusAccepted, ObjectStatusDeferred)
	if err != nil {
		t.Fatalf("IsValidTransition(accepted→deferred): %v", err)
	}
	if !ok {
		t.Fatal("IsValidTransition(accepted→deferred) = false, want repair-park true")
	}
}

func TestNormalizeStatusForKind_UnknownFailsClosed(t *testing.T) {
	t.Parallel()
	lifecyclesDir := filepath.Join("..", "..", paths.ProcessInternalLifecyclesDir)
	loader := NewLifecycleLoader(lifecyclesDir)

	_, err := loader.NormalizeStatusForKind("technical_debt", ObjectStatusAccepted)
	if err == nil {
		t.Fatal("NormalizeStatusForKind(technical_debt, accepted) err=nil, want unknown")
	}
	if !errors.Is(err, ErrLifecycleStatusUnknown) {
		t.Fatalf("NormalizeStatusForKind(technical_debt, accepted) err=%v, want ErrLifecycleStatusUnknown", err)
	}
}
