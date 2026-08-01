package objects

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/lanceman/zqk/pkg/errfmt"
)

// SpecReadMeta is filesystem-oriented metadata returned with spec bytes.
// Other SpecStorageProvider implementations may synthesize ModTime/Size when not applicable.
type SpecReadMeta struct {
	ModTime time.Time
	Size    int64
}

// SpecStorageProvider abstracts reading raw object spec YAML from storage (REQ-035 / ITEM-147 / CRIT-9031).
// Callers pass absolute, cleaned paths under the project spec root; implementations enforce containment.
// Today SpecLoader uses FileSpecStorageProvider; future backends may use CAS or remote stores.
type SpecStorageProvider interface {
	ReadSpecBytes(ctx context.Context, absPath string) ([]byte, SpecReadMeta, error)
}

// FileSpecStorage is a type alias for the file-backed SpecStorageProvider (ITEM-148 / CRIT-9032).
type FileSpecStorage = FileSpecStorageProvider

// FileSpecStorageProvider reads specs from a directory tree on the local filesystem (current default behavior).
type FileSpecStorageProvider struct {
	root string // absolute, filepath.Clean
}

// NewFileSpecStorageProvider returns a provider rooted at root (directory containing object spec YAML files).
func NewFileSpecStorageProvider(root string) (*FileSpecStorageProvider, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, errfmt.Newf("spec storage root").Wrap(err)
	}
	return &FileSpecStorageProvider{root: filepath.Clean(abs)}, nil
}

// ReadSpecBytes reads the file at absPath after verifying it is under the provider root.
func (f *FileSpecStorageProvider) ReadSpecBytes(ctx context.Context, absPath string) ([]byte, SpecReadMeta, error) {
	if f == nil {
		return nil, SpecReadMeta{}, errfmt.Errorf("nil FileSpecStorageProvider")
	}
	if err := ctx.Err(); err != nil {
		return nil, SpecReadMeta{}, err
	}
	target, err := filepath.Abs(absPath)
	if err != nil {
		return nil, SpecReadMeta{}, err
	}
	target = filepath.Clean(target)
	if err := pathUnderRoot(f.root, target); err != nil {
		return nil, SpecReadMeta{}, err
	}
	data, err := os.ReadFile(target)
	if err != nil {
		return nil, SpecReadMeta{}, err
	}
	var meta SpecReadMeta
	if st, err := os.Stat(target); err == nil {
		meta.ModTime = st.ModTime()
		meta.Size = st.Size()
	}
	return data, meta, nil
}

func pathUnderRoot(root, target string) error {
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return errfmt.Errorf("spec path %q outside spec root %q: %w", target, root, err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return errfmt.Errorf("spec path %q escapes spec root %q", target, root)
	}
	return nil
}
