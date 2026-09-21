package stampmemo

import (
	"path/filepath"
	"strings"

	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

const yamlExt = ".yaml"

// WalkYAML calls fn for each non-dot *.yaml file in dir. Missing dirs return the ReadDir error.
func WalkYAML(dir string, fn func(fileID, path string, data []byte)) error {
	entries, err := fileutil.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, yamlExt) || strings.HasPrefix(name, ".") {
			continue
		}
		path := filepath.Join(dir, name)
		data, readErr := fileutil.ReadFile(path)
		if readErr != nil {
			continue
		}
		fn(strings.TrimSuffix(name, yamlExt), path, data)
	}
	return nil
}
