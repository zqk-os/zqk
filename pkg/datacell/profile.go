package datacell

import (
	"github.com/lanceman/zqk/pkg/errfmt"
)

// StorageProfile names the physical storage mechanism for a spec-backed kind (data cell model).
// Declared as optional top-level storage_profile in object_specs YAML; inherited along extends chains.
type StorageProfile string

const (
	// ProfileCASEntity is hash-addressed / CAS-backed instance storage (typical process objects under .zqk/process/).
	ProfileCASEntity StorageProfile = "cas_entity"
	// ProfileLightFile is low-ceremony JSON/YAML/JSONL surfaces (e.g. runtime organism slice).
	ProfileLightFile StorageProfile = "light_file"
	// ProfileStream is append-heavy stream-backed layouts (segments, summaries).
	ProfileStream StorageProfile = "stream"
)

// KnownStorageProfiles lists every accepted storage_profile value for validation and UX.
var KnownStorageProfiles = []StorageProfile{
	ProfileCASEntity,
	ProfileLightFile,
	ProfileStream,
}

// ParseStorageProfile returns a StorageProfile when s is a known non-empty value.
// Empty string means unspecified (caller may inherit from parent spec or policy).
func ParseStorageProfile(s string) (StorageProfile, error) {
	if s == "" {
		return "", nil
	}
	p := StorageProfile(s)
	if !p.IsKnown() {
		return "", errfmt.Errorf("unknown storage_profile %q (want one of cas_entity, light_file, stream)", s)
	}
	return p, nil
}

// IsKnown reports whether p is one of the declared profile constants.
func (p StorageProfile) IsKnown() bool {
	switch p {
	case ProfileCASEntity, ProfileLightFile, ProfileStream:
		return true
	default:
		return false
	}
}

// String returns the wire value (empty if unset).
func (p StorageProfile) String() string {
	return string(p)
}
