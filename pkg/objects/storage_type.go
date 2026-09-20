package objects

import (
	"github.com/zqk-os/zqk/pkg/datacell"
)

// StorageType represents the physical storage mechanism for a spec-backed kind.
type StorageType = datacell.StorageProfile

const (
	StorageTypeStream    = datacell.ProfileStream
	StorageTypeCASEntity = datacell.ProfileCASEntity
	StorageTypeLightFile = datacell.ProfileLightFile
)

// IsStreamStorage reports whether a storage profile/type string represents stream-backed storage.
func IsStreamStorage(storageType string) bool {
	return storageType == string(datacell.ProfileStream) || storageType == "stream"
}

// IsBypassStorageType reports whether a storage profile/type bypasses CAS draft-plane routing.
// Objects using stream storage (append-only segments) bypass CAS and the draft plane entirely.
func IsBypassStorageType(storageType string) bool {
	switch {
	case IsStreamStorage(storageType):
		return true
	default:
		return false
	}
}

// ResolveStorageProfile returns the effective storage_profile for kind,
// resolving from the materialized spec index or the spec loader with inheritance.
func ResolveStorageProfile(kind string) string {
	if kind == "" {
		return ""
	}
	if idx := loadSpecIndexForKernelCritical(); idx != nil {
		if ks, ok := idx.GetKindSummary(kind); ok && ks.StorageProfile != "" {
			return ks.StorageProfile
		}
	}
	loader := GetGlobalSpecLoader()
	if loader != nil {
		spec, err := loader.LoadSpecWithInheritance(kind + yamlExt)
		if err == nil && spec != nil && spec.StorageProfile != "" {
			return spec.StorageProfile
		}
	}
	return ""
}

// IsBypassKind reports whether kind bypasses CAS draft-plane routing based on its storage profile.
// It accepts either a kind name (e.g. "audit_event") or a storage profile string directly (e.g. "stream").
func IsBypassKind(kind string) bool {
	if kind == "" {
		return false
	}
	if IsBypassStorageType(kind) {
		return true
	}
	profile := ResolveStorageProfile(kind)
	return IsBypassStorageType(profile)
}
