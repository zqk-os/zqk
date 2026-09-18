package kernelcas

import "github.com/zqk-os/zqk/pkg/objects"

// IsCriticalKind reports kinds that must not be silently hard-deleted and that require
// break_glass for lifecycle Force. Policy is spec-driven (object_specs kernel_critical +
// storage_profile inference via objects.IsKernelCriticalKind).
// storage.IsCoreKernelKind delegates here to avoid import cycles.
// TRACK: BLI-1785784863457357000-dda098ed
func IsCriticalKind(kind string) bool {
	return objects.IsKernelCriticalKind(kind)
}

// ListCriticalKinds returns sorted critical object kinds from the materialized spec index.
// TRACK: BLI-1785784863457357000-dda098ed
func ListCriticalKinds() []string {
	return objects.ListKernelCriticalKinds()
}
