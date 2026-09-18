package objects

import (
	"path/filepath"
	"sort"

	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// applyKernelCriticalDefaults fills KernelCritical when still unset after inheritance.
// Inference: stream/light_file → false (ephemeral); everything else → true (fail-closed).
// Explicit YAML on this kind always wins.
// TRACK: BLI-1785784863457357000-dda098ed
func applyKernelCriticalDefaults(spec *Spec) {
	if spec == nil || spec.KernelCritical != nil {
		return
	}
	v := defaultKernelCriticalForProfile(spec.StorageProfile)
	spec.KernelCritical = &v
}

// defaultKernelCriticalForProfile infers hard-delete protection from the storage profile.
//
// Only an explicitly ephemeral profile opts out. An unresolved profile returns true, because
// this value decides whether an irreversible delete needs a reason code — so "I could not
// determine what this kind is" has to mean protected, not unprotected.
//
// The empty case is not hypothetical. SpecIndex resolves storage_profile along extends, but
// SpecLoader.LoadSpec does not, so criteria, backlog_item, milestone, priority_plan and
// convergence_session all read back empty. Defaulting that to false made the two paths of
// [IsKernelCriticalKind] disagree: the index called them critical while the loader fallback
// called them disposable. Whichever answer you got depended on whether spec_index.json was
// readable from the process working directory.
func defaultKernelCriticalForProfile(storageProfile string) bool {
	switch storageProfile {
	case string(datacell.ProfileStream), string(datacell.ProfileLightFile):
		return false
	default:
		return true
	}
}

func cloneBoolPtr(p *bool) *bool {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}

// EffectiveKernelCritical reports the resolved kernel-critical policy for a loaded spec.
// TRACK: BLI-1785784863457357000-dda098ed
func EffectiveKernelCritical(spec *Spec) bool {
	if spec == nil {
		return false
	}
	if spec.KernelCritical != nil {
		return *spec.KernelCritical
	}
	return defaultKernelCriticalForProfile(spec.StorageProfile)
}

// IsKernelCriticalKind reports whether kind must not be silently hard-deleted / Force-skipped
// without break_glass. Spec-driven via object_specs kernel_critical (+ storage_profile inference).
//
// A kind the ontology does not describe at all still returns false: absent from the index, or
// no loadable spec, means synthetic or test kinds stay deletable. That is deliberate, and it is
// narrower than it looks — a kind whose spec loads but leaves the profile unresolved is now
// protected, per [defaultKernelCriticalForProfile]. Widen this only with a plan for the
// synthetic kinds that unit tests create and delete.
// TRACK: BLI-1785784863457357000-dda098ed
func IsKernelCriticalKind(kind string) bool {
	if kind == "" {
		return false
	}
	if idx := loadSpecIndexForKernelCritical(); idx != nil {
		if ks, ok := idx.GetKindSummary(kind); ok {
			return ks.EffectiveKernelCritical()
		}
		return false
	}
	loader := GetGlobalSpecLoader()
	if loader == nil {
		return false
	}
	// Must resolve inheritance. SpecLoader.LoadSpec is documented "without inheritance" — a raw
	// YAML unmarshal — and only five specs in the tree declare storage_profile on themselves, so
	// asking EffectiveKernelCritical about a raw spec reads an unset profile for nearly every
	// kind. That is how this fallback came to call criteria, backlog_item, milestone,
	// priority_plan and convergence_session non-critical while the index called them critical.
	spec, err := loader.LoadSpecWithInheritance(kind + yamlExt)
	if err != nil || spec == nil {
		return false
	}
	return EffectiveKernelCritical(spec)
}

// ListKernelCriticalKinds returns sorted ontology names with EffectiveKernelCritical true.
// TRACK: BLI-1785784863457357000-dda098ed
func ListKernelCriticalKinds() []string {
	idx := loadSpecIndexForKernelCritical()
	if idx == nil {
		return nil
	}
	out := make([]string, 0, len(idx.Kinds))
	for kind, ks := range idx.Kinds {
		if ks.EffectiveKernelCritical() {
			out = append(out, kind)
		}
	}
	sort.Strings(out)
	return out
}

func loadSpecIndexForKernelCritical() *SpecIndex {
	root := projectRootForKernelCritical()
	if root == "" {
		return nil
	}
	if idx := TryLoadSpecIndexForProjectRoot(root); idx != nil {
		return idx
	}
	// Tests / early init often lack path-cache entries; read the JSON directly.
	p := filepath.Join(root, paths.ProcessInternalDir, "spec_index.json")
	idx, err := LoadSpecIndex(p)
	if err != nil {
		return nil
	}
	return idx
}

func projectRootForKernelCritical() string {
	wd, err := fileutil.Getwd()
	if err != nil {
		return ""
	}
	if mr, err := paths.ModuleRootFromPath(wd); err == nil && mr != "" {
		return mr
	}
	return wd
}
