package objectget

import (
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
)

// CVS id used only for prefix inference (need not exist in CAS).

func TestBuildObjectGetArgv(t *testing.T) {
	id := "CVS-STARTER-001"
	got := BuildObjectGetArgv(id, Options{
		Binary:    "zqk",
		Format:    "json",
		View:      ViewMilestoneCompletionReport,
		Hydration: HydrationLazy,
	})
	want := strings.Join([]string{"zqk", "object", "get", id, "--format", "json", "--view", ViewMilestoneCompletionReport, "--link-hydration", "lazy"}, " ")
	if strings.Join(got, " ") != want {
		t.Fatalf("argv mismatch\ngot:  %v\nwant: %s", got, want)
	}

	minimal := BuildObjectGetArgv(id, Options{})
	if len(minimal) != 4 || minimal[0] != paths.CLICommandName || minimal[3] != id {
		t.Fatalf("minimal argv: %#v", minimal)
	}
}

func TestOverlayPlanForMilestoneReportUsesCriteriaOnly(t *testing.T) {
	p := OverlayPlanFor(objects.KindMilestone, ViewMilestoneCompletionReport, HydrationUnspecified)
	if len(p.ReferenceResolver.ResolveFieldKeys) != 1 || p.ReferenceResolver.ResolveFieldKeys[0] != objects.FieldKeyCriteriaRefs {
		t.Fatalf("unexpected resolver keys: %#v", p.ReferenceResolver.ResolveFieldKeys)
	}
	if p.ReferenceResolver.MaxDepth != 1 {
		t.Fatalf("want shallow milestone slice, got depth %d", p.ReferenceResolver.MaxDepth)
	}
	if !p.ApplyMilestoneCriteriaOverlay {
		t.Fatal("expected milestone overlay")
	}
}

func TestOverlayPlanHydrationOverridesResolver(t *testing.T) {
	p := OverlayPlanFor(objects.KindMilestone, ViewMilestoneCompletionReport, HydrationEager)
	if p.ReferenceResolver.MaxDepth != 4 {
		t.Fatalf("eager override: want depth 4, got %d", p.ReferenceResolver.MaxDepth)
	}
	if len(p.ReferenceResolver.ResolveFieldKeys) != 0 {
		t.Fatalf("eager override should clear field filter, got %#v", p.ReferenceResolver.ResolveFieldKeys)
	}
}

func TestParseLinkHydration(t *testing.T) {
	h, err := ParseLinkHydration("LAZY ")
	if err != nil || h != HydrationLazy {
		t.Fatalf("lazy: %v %v", h, err)
	}
	none, err := ParseLinkHydration("none")
	if err != nil || none != HydrationNone {
		t.Fatalf("none: %v %v", none, err)
	}
	if _, err := ParseLinkHydration("nope"); err == nil {
		t.Fatal("expected error")
	}
}

func TestNormalizeReferenceOverlayConfig(t *testing.T) {
	cfg := NormalizeReferenceOverlayConfig(ReferenceResolverOverlayConfig{})
	if cfg.MaxDepth != DefaultReferenceResolverMaxDepth {
		t.Fatalf("depth %d", cfg.MaxDepth)
	}
	disabled := NormalizeReferenceOverlayConfig(ReferenceResolverOverlayConfig{Disabled: true})
	if !disabled.Disabled || disabled.MaxDepth != 0 {
		t.Fatalf("disabled normalize should leave depth 0: %#v", disabled)
	}
}

func TestOverlayPlanForDefaultViewIsRaw(t *testing.T) {
	p := OverlayPlanFor(objects.KindBacklogItem, ViewDefault, HydrationUnspecified)
	if !p.ReferenceResolver.Disabled {
		t.Fatalf("default view want Disabled (raw CAS), got %#v", p.ReferenceResolver)
	}
	lazy := OverlayPlanFor(objects.KindBacklogItem, ViewDefault, HydrationLazy)
	if lazy.ReferenceResolver.Disabled || lazy.ReferenceResolver.MaxDepth != 1 {
		t.Fatalf("--link-hydration lazy want depth 1: %#v", lazy.ReferenceResolver)
	}
	explicit := OverlayPlanFor(objects.KindBacklogItem, ViewDefault, HydrationDefaultExplicit)
	explicit.ReferenceResolver = NormalizeReferenceOverlayConfig(explicit.ReferenceResolver)
	if explicit.ReferenceResolver.MaxDepth != DefaultReferenceResolverMaxDepth {
		t.Fatalf("--link-hydration default want depth %d, got %d", DefaultReferenceResolverMaxDepth, explicit.ReferenceResolver.MaxDepth)
	}
}

func TestInferKindFromObjectID_withProjectRoot(t *testing.T) {
	k := InferKindFromObjectID(".", "CVS-1234567890123456000-abcdef12")
	if k != objects.KindConvergenceSession {
		t.Fatalf("want convergence_session, got %q", k)
	}
	if InferKindFromObjectID(".", "   ") != "" {
		t.Fatal("empty id")
	}
}
