package paths

import (
	"io/fs"
	"path/filepath"
	"strings"
)

// IndexYAMLNames maps basename and ontology (basename minus .yaml) to abs path.
// Nested directories are included. Last walk wins on collision. Keys are the
// closed spec/lifecycle/trait tree under dir — not every file in the repo.
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
		if !strings.HasSuffix(name, YAMLExtension) {
			return nil
		}
		idx[name] = path
		idx[strings.TrimSuffix(name, YAMLExtension)] = path
		return nil
	})
	return idx
}
