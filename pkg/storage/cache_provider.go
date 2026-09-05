package storage

// ObjectIDCacheProvider defines the interface for the object ID cache used by storage
type ObjectIDCacheProvider interface {
	GetFilePathsForKind(kind string) []string
}

var globalCacheProvider ObjectIDCacheProvider

// SetGlobalCacheProvider sets the global cache provider
func SetGlobalCacheProvider(provider ObjectIDCacheProvider) {
	globalCacheProvider = provider
}
