package objects

import (
	"sort"
	"testing"

	"github.com/zqk-os/zqk/pkg/datacell"
)

// TestKernelCritical_indexAndLoaderAgreeForEveryKind pins the property that matters, rather
// than a list of kinds: whichever route IsKernelCriticalKind takes, it must return the same
// answer.
//
// The two routes resolve storage_profile differently — SpecIndex inherits it along extends,
// SpecLoader.LoadSpec does not — so any profile-derived default is a place where they can
// disagree. They did: criteria, backlog_item, milestone, priority_plan and convergence_session
// read critical from the index and disposable from the loader fallback, which made hard-delete
// protection for the durable planning graph depend on spec_index.json being readable from the
// process working directory.
func TestKernelCritical_indexAndLoaderAgreeForEveryKind(t *testing.T) {
	idx := loadSpecIndexForKernelCritical()
	if idx == nil || len(idx.Kinds) == 0 {
		t.Skip("spec index unavailable from this working directory")
	}
	loader := GetGlobalSpecLoader()
	if loader == nil {
		t.Skip("no global spec loader")
	}

	var diverged []string
	for kind, ks := range idx.Kinds {
		// LoadSpecWithInheritance, not LoadSpec: the latter is documented as skipping
		// inheritance, so comparing against it would compare a resolved profile to an unset one.
		spec, err := loader.LoadSpecWithInheritance(kind + yamlExt)
		if err != nil || spec == nil {
			// Only kinds both routes can resolve are comparable.
			continue
		}
		if ks.EffectiveKernelCritical() != EffectiveKernelCritical(spec) {
			if staleIndexKernelCriticalBaseline[kind] {
				continue
			}
			diverged = append(diverged, kind)
		}
	}
	if len(diverged) > 0 {
		sort.Strings(diverged)
		t.Fatalf("index and loader disagree on kernel_critical for %d kind(s): %v\n"+
			"a kind must not change protection based on which route resolved it",
			len(diverged), diverged)
	}
}

// staleIndexKernelCriticalBaseline lists kinds whose materialized spec_index.json entry predates
// the current inference and therefore still disagrees with a freshly resolved spec.
//
// spec_index.json stores the *inferred* kernel_critical, not only what a spec declares, so the
// cache carries whatever the default was on the day it was generated. These three have no
// storage_profile of their own and were materialized false; tde_envelope also resolves
// cas_entity from base_object that the cached entry never picked up. Regenerating the index is a
// 134-kind spec-plane change and belongs in its own reviewable pass, so this baseline exists to
// keep the set from growing rather than to bless it.
//
// Do not add entries. Regenerate the index and delete rows instead.
// TRACK: BLI-1785784863457357000-dda098ed
var staleIndexKernelCriticalBaseline = map[string]bool{
	"kind_synonym": true,
	"qa_success":   true,
	"tde_envelope": true,
}

// TestDefaultKernelCriticalForProfile_unresolvedFailsClosed pins the direction of the default.
// This value decides whether an irreversible delete needs a reason code, so an unresolved
// profile has to protect.
func TestDefaultKernelCriticalForProfile_unresolvedFailsClosed(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		profile string
		want    bool
		why     string
	}{
		{"", true, "unresolved profile must protect, not expose"},
		{"nonsense_profile", true, "an unrecognized profile is not evidence of being ephemeral"},
		{string(datacell.ProfileStream), false, "stream is explicitly ephemeral"},
		{string(datacell.ProfileLightFile), false, "light_file is explicitly ephemeral"},
		{string(datacell.ProfileCASEntity), true, "cas_entity is the durable graph"},
	} {
		if got := defaultKernelCriticalForProfile(tc.profile); got != tc.want {
			t.Errorf("profile %q: got %v want %v — %s", tc.profile, got, tc.want, tc.why)
		}
	}
}

// TestKernelCritical_durableKindsAreProtectedOnBothRoutes covers the specific regression:
// these five are the planning graph other objects point at, and two criteria were destroyed
// in commit 530ec90795. Explicit opt-outs must still win, or this test would be asserting
// that nothing can ever be ephemeral.
func TestKernelCritical_durableKindsAreProtectedOnBothRoutes(t *testing.T) {
	loader := GetGlobalSpecLoader()
	if loader == nil {
		t.Skip("no global spec loader")
	}
	for _, kind := range []string{
		KindCriteria, KindBacklogItem, KindMilestone, KindPriorityPlan, KindConvergenceSession,
	} {
		if !IsKernelCriticalKind(kind) {
			t.Errorf("%s must be kernel-critical", kind)
		}
		spec, err := loader.LoadSpecWithInheritance(kind + yamlExt)
		if err != nil || spec == nil {
			t.Errorf("%s: spec did not load: %v", kind, err)
			continue
		}
		if !EffectiveKernelCritical(spec) {
			t.Errorf("%s must be kernel-critical on the loader route too", kind)
		}
	}
	// An explicit kernel_critical: false still opts out; the fail-closed default is only for
	// kinds that never stated a position.
	if IsKernelCriticalKind(KindSchedulerJob) {
		t.Error("scheduler_job declares kernel_critical: false; explicit opt-out must win")
	}
	if IsKernelCriticalKind(KindAuditEvent) {
		t.Error("audit_event is stream-profiled; must stay non-critical")
	}
}
