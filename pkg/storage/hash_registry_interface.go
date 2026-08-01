package storage

// HashRegistryProvider defines the interface for hash registry implementations
// This allows consistent behavior across file-based and graph-based backends
type HashRegistryProvider interface {
	// Load loads the hash registry from storage
	Load() error

	// Save saves the hash registry to storage
	Save() error

	// GetHash returns the hash for a given object identifier (filename for files, id for graph)
	GetHash(identifier string) string

	// SetHash sets the hash for a given object identifier
	SetHash(identifier, hash string)

	// DeleteHash removes the hash for a given object identifier
	DeleteHash(identifier string)

	// HasHash returns true if a hash exists for the given identifier
	HasHash(identifier string) bool

	// GetAllHashes returns a copy of all hashes (identifier -> hash)
	GetAllHashes() map[string]string
}
