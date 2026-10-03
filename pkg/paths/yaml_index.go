package paths

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// isHexHashStem reports whether s is a 64-character hexadecimal string (e.g. CAS content hash).
func isHexHashStem(s string) bool {
	if len(s) != 64 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

// IndexYAMLNames maps basename and ontology (basename minus .yaml) to abs path.
// Nested directories are included. Last walk wins on collision. Keys are the
// closed spec/lifecycle/trait tree under dir — not every file in the repo.
// Names that start with "." are skipped (includes macOS AppleDouble `._*`).
// Stems that are 64-character hex hashes (CAS instance files) are also skipped.
func IndexYAMLNames(dir string) map[string]string {
	idx := make(map[string]string)
	if dir == "" {
		return idx
	}
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d == nil || d.IsDir() {
			return nil
		}
		name := d.Name()
		if strings.HasPrefix(name, ".") || !strings.HasSuffix(name, YAMLExtension) {
			return nil
		}
		stem := strings.TrimSuffix(name, YAMLExtension)
		if isHexHashStem(stem) {
			return nil
		}
		idx[name] = path
		idx[stem] = path
		return nil
	})
	return idx
}

// ResolveYAMLVariant looks for a YAML file matching name or hyphen/underscore variants in dir.
func ResolveYAMLVariant(dir, name string) string {
	possibleNames := []string{
		name + YAMLExtension,
		strings.ReplaceAll(name, "-", "_") + YAMLExtension,
		strings.ReplaceAll(name, "_", "-") + YAMLExtension,
	}
	for _, fileName := range possibleNames {
		p := filepath.Join(dir, fileName)
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p
		}
	}
	return ""
}
