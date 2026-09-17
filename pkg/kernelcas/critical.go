package kernelcas

import "github.com/lanceman/zqk/pkg/objects"

// IsCriticalKind reports kinds that must not be silently hard-deleted and that require
// break_glass for lifecycle Force. Policy is spec-driven (object_specs kernel_critical +
// storage_profile inference via objects.IsKernelCriticalKind).
// storage.IsCoreKernelKind delegates here to avoid import cycles.
// TRACK: BLI-REDACTED
func IsCriticalKind(kind string) bool {
	return objects.IsKernelCriticalKind(kind)
}

// ListCriticalKinds returns sorted critical object kinds from the materialized spec index.
// TRACK: BLI-REDACTED
func ListCriticalKinds() []string {
	return objects.ListKernelCriticalKinds()
}
