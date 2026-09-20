package objects

import (
	"path/filepath"
	"strings"
)

// IsHashedFilename returns true if the base name of the file (without extension)
// is exactly 64 hexadecimal characters.
func IsHashedFilename(name string) bool {
	base := filepath.Base(name)
	base = strings.TrimSuffix(base, filepath.Ext(base))
	if len(base) != 64 {
		return false
	}
	for _, c := range base {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}
