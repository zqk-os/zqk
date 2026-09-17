package paths

import (
	"path/filepath"
	"strings"

	"github.com/lanceman/zqk/pkg/errfmt"
)

const emptyValue = ""

// ErrPathEscapesProject is returned when a path would escape the project root.
var ErrPathEscapesProject = errfmt.Errorf("path escapes project root")

// ProjectPath returns a path under projectRoot by joining projectRoot with elem.
// The result is cleaned and validated to be under projectRoot (no ".." escape).
// Use this for all project-scoped paths (e.g. .zqk, .zqk/process) so nested project
// roots never reference data outside the defined use context.
func ProjectPath(projectRoot string, elem ...string) (string, error) {
	if projectRoot == emptyValue {
		return emptyValue, ErrPathEscapesProject
	}
	base, err := filepath.Abs(projectRoot)
	if err != nil {
		return emptyValue, err
	}
	joined := filepath.Join(append([]string{base}, elem...)...)
	cleaned := filepath.Clean(joined)
	return cleaned, checkUnderRoot(cleaned, base)
}

// UnderProjectRoot returns true if path is under projectRoot (or equal). Both are normalized to absolute.
// Use to validate that a path never escapes the project (e.g. job WorkingDirectory, config paths).
func UnderProjectRoot(projectRoot, path string) bool {
	base, err1 := filepath.Abs(projectRoot)
	p, err2 := filepath.Abs(path)
	if err1 != nil || err2 != nil {
		return false
	}
	base = filepath.Clean(base)
	p = filepath.Clean(p)
	if p == base {
		return true
	}
	sep := string(filepath.Separator)
	return strings.HasPrefix(p, base+sep)
}

func checkUnderRoot(cleaned, base string) error {
	if !UnderProjectRoot(base, cleaned) {
		return ErrPathEscapesProject
	}
	return nil
}
