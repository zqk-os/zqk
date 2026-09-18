package storage

import (
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// StorageConfig holds configuration for file storage operations
// All file permissions, extensions, and paths should be driven from this config
type StorageConfig struct {
	// File permissions
	DefaultFilePerm  fileutil.FileMode // Default file permission (0600)
	DefaultDirPerm   fileutil.FileMode // Default directory permission (0755)
	KeystoreFilePerm fileutil.FileMode // Keystore file permission (0600)
	KeystoreDirPerm  fileutil.FileMode // Keystore directory permission (0700)

	// File extensions
	YAMLExtension    string // YAML file extension (.yaml)
	YAMLAltExtension string // Alternative YAML extension (.yml)
}

// DefaultStorageConfig returns the default storage configuration
// These defaults match current behavior but can be overridden via config files
func DefaultStorageConfig() *StorageConfig {
	return &StorageConfig{
		DefaultFilePerm:  0600,
		DefaultDirPerm:   0755,
		KeystoreFilePerm: 0600,
		KeystoreDirPerm:  0700,
		YAMLExtension:    ".yaml",
		YAMLAltExtension: ".yml",
	}
}

// Global storage config instance (initialized with defaults)
var globalStorageConfig = DefaultStorageConfig()

// GetStorageConfig returns the global storage configuration
func GetStorageConfig() *StorageConfig {
	return globalStorageConfig
}

// SetStorageConfig sets the global storage configuration
// This allows configuration to be loaded from config files
func SetStorageConfig(config *StorageConfig) {
	if config != nil {
		globalStorageConfig = config
	}
}
