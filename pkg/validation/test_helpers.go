package validation

import (
	"path/filepath"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/paths"
)

// setupTestEnvironment creates the test directory structure
// This is inlined here to avoid import cycle:
// validation -> testing -> specbuilder/bldr_v2 -> validation
func setupTestEnvironment(testRoot string) (string, error) {
	absRoot, err := filepath.Abs(testRoot)
	if err != nil {
		return "", errfmt.Newf(ConstMagic8a3a4a62).Wrap(err)
	}

	if err := paths.LayoutUnder(absRoot).
		Dir(paths.ProjectDataDir, paths.DirPerm755).
		Dir(paths.ProcessInternalObjectSpecsDir, paths.DirPerm755).
		Dir(paths.ProcessInternalTraitsDir, paths.DirPerm755).
		Err(); err != nil {
		return "", errfmt.Newf(ConstMagic25824abd).Wrap(err)
	}

	return absRoot, nil
}
