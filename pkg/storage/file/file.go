package file

import (
	"path/filepath"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// Store implements basic file persistence for objects.
type Store struct {
	baseDir string
}

// NewStore initializes a new file store rooted at baseDir.
func NewStore(baseDir string) (*Store, error) {
	if err := fileutil.MkdirAll(baseDir, paths.DirPerm755); err != nil {
		return nil, errfmt.Newf("failed to create base storage directory: %v", err).Wrap(err)
	}
	return &Store{baseDir: baseDir}, nil
}

// Write writes an object payload to the given subpath under baseDir.
func (s *Store) Write(relPath string, data []byte) error {
	fullPath := filepath.Join(s.baseDir, relPath)
	dir := filepath.Dir(fullPath)
	if err := fileutil.MkdirAll(dir, paths.DirPerm755); err != nil {
		return errfmt.Newf("failed to create parent directory: %v", err).Wrap(err)
	}
	return fileutil.WriteFile(fullPath, data, paths.FilePerm644)
}

// Read reads an object payload from the given subpath under baseDir.
func (s *Store) Read(relPath string) ([]byte, error) {
	fullPath := filepath.Join(s.baseDir, relPath)
	return fileutil.ReadFile(fullPath)
}
