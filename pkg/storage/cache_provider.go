package storage

// ObjectIDCacheProvider defines the interface for the object ID cache used by storage
type ObjectIDCacheProvider interface {
	GetFilePathsForKind(kind string) []string
	LocateObject(id string) (kind string, filePath string, found bool)
}

var globalCacheProvider ObjectIDCacheProvider

// GetGlobalCacheProvider returns the global cache provider
func GetGlobalCacheProvider() ObjectIDCacheProvider {
	return globalCacheProvider
}

// SetGlobalCacheProvider sets the global cache provider
func SetGlobalCacheProvider(provider ObjectIDCacheProvider) {
	globalCacheProvider = provider
}
