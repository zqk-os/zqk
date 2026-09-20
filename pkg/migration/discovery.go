package migration

import (
	"path/filepath"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// MigrationsDirName is the directory name under process _internal for migration specs.
const MigrationsDirName = "migrations"

// SpecInfo holds minimal migration spec metadata for discovery and listing.
type SpecInfo struct {
	ID          string
	Name        string
	Description string
	Path        string
}

// ListSpecs discovers migration specs under the project's migrations directory
// and returns metadata (id, name, description, path) for each. Paths are absolute.
// Returns an empty slice if the migrations directory does not exist.
func ListSpecs(projectRoot string) ([]SpecInfo, error) {
	dir := filepath.Join(projectRoot, paths.ProcessInternalDir, MigrationsDirName)
	entries, err := fileutil.ReadDir(dir)
	if err != nil {
		if fileutil.IsNotExist(err) {
			return nil, nil
		}
		return nil, errfmt.Newf("read migrations dir").Wrap(err)
	}

	var out []SpecInfo
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		ext := filepath.Ext(name)
		if ext != ".yaml" && ext != ".yml" {
			continue
		}
		specPath := filepath.Join(dir, name)
		spec, err := LoadSpec(specPath)
		if err != nil {
			// Skip invalid specs but include path so caller can show error if desired
			continue
		}
		out = append(out, SpecInfo{
			ID:          spec.ID,
			Name:        spec.Name,
			Description: spec.Description,
			Path:        specPath,
		})
	}
	return out, nil
}
