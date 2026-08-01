package fileutil

import (
	"os"
)

// WriteSecureFile writes data to a file with secure permissions (0600).
func WriteSecureFile(path string, data []byte) error {
	return os.WriteFile(path, data, 0600)
}

// WriteStandardFile writes data to a file with standard permissions (0644).
func WriteStandardFile(path string, data []byte) error {
	return os.WriteFile(path, data, 0644)
}

// EnsureDir ensures that a directory exists with standard permissions (0755).
func EnsureDir(path string) error {
	return os.MkdirAll(path, 0755)
}
