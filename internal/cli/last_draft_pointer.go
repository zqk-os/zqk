package cli

import (
	"fmt"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// Last-draft pointer scopes (written by `zqk new` when materializing to a file).
const (
	LastDraftScopeObject   = "object"
	LastDraftScopeInternal = "internal"
	LastDraftScopeBundle   = "bundle"
)

// LastDraftPointer is persisted to .zqk/drafts/last-draft.yaml by `zqk new` after writing a draft file.
type LastDraftPointer struct {
	APIVersion int    `yaml:"api_version"`
	Scope      string `yaml:"scope"`
	Kind       string `yaml:"kind"`
	Path       string `yaml:"path"`
}

// LastDraftHint selects which pointer entry applies to the current create command.
type LastDraftHint struct {
	Scope string // LastDraftScopeObject, LastDraftScopeInternal, or empty to skip pointer resolution
	Kind  string // canonical kind (must match pointer and YAML)
}

// LastDraftPointerPath returns the absolute path to the pointer file for a project root.
func LastDraftPointerPath(projectRoot string) string {
	return filepath.Join(projectRoot, paths.ProjectDataDir, paths.DraftsDir, paths.LastDraftPointerFile)
}

// WriteLastDraftPointer records the last materialized draft so `zqk object create <kind>` can omit --file.
func WriteLastDraftPointer(projectRoot, scope, canonicalKind, draftAbsPath string) error {
	if projectRoot == emptyValue || scope == emptyValue || canonicalKind == emptyValue || draftAbsPath == emptyValue {
		return nil
	}
	abs, err := filepath.Abs(draftAbsPath)
	if err != nil {
		return errfmt.Newf("last-draft pointer: abs path").Wrap(err)
	}
	ptr := LastDraftPointer{
		APIVersion: 1,
		Scope:      scope,
		Kind:       objects.GetCanonicalKind(canonicalKind),
		Path:       abs,
	}
	out, err := yaml.Marshal(&ptr)
	if err != nil {
		return errfmt.Newf("last-draft pointer: marshal").Wrap(err)
	}
	dir := filepath.Dir(LastDraftPointerPath(projectRoot))
	if err := fileutil.EnsureDir(dir); err != nil {
		return errfmt.Newf("last-draft pointer: mkdir").Wrap(err)
	}
	tmp := LastDraftPointerPath(projectRoot) + paths.TmpFileSuffix
	if err := fileutil.WriteSecureFile(tmp, out); err != nil {
		return errfmt.Newf("last-draft pointer: write tmp").Wrap(err)
	}
	if err := fileutil.Rename(tmp, LastDraftPointerPath(projectRoot)); err != nil {
		_ = fileutil.Remove(tmp)
		return errfmt.Newf("last-draft pointer: rename").Wrap(err)
	}
	return nil
}

// ReadLastDraftPointer reads the pointer file without validation.
func ReadLastDraftPointer(projectRoot string) (*LastDraftPointer, error) {
	p := LastDraftPointerPath(projectRoot)
	b, err := fileutil.ReadFile(p)
	if err != nil {
		if fileutil.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var ptr LastDraftPointer
	if err := yaml.Unmarshal(b, &ptr); err != nil {
		return nil, errfmt.Newf("parse last-draft pointer").Wrap(err)
	}
	return &ptr, nil
}

// ResolveLastDraftFile returns the draft file path if the pointer matches scope and kind and the file exists.
func ResolveLastDraftFile(projectRoot, scope, expectedKind string) (string, error) {
	if projectRoot == emptyValue || scope == emptyValue || expectedKind == emptyValue {
		return "", nil
	}
	ptr, err := ReadLastDraftPointer(projectRoot)
	if err != nil {
		return "", err
	}
	if ptr == nil || ptr.Path == emptyValue {
		return "", nil
	}
	if ptr.APIVersion != 1 {
		return "", errfmt.Errorf("unsupported last-draft pointer api_version %d", ptr.APIVersion)
	}
	if ptr.Scope != scope {
		return "", nil
	}
	want := objects.GetCanonicalKind(expectedKind)
	got := objects.GetCanonicalKind(ptr.Kind)
	if got != want {
		return "", errfmt.Errorf("last draft points to kind %q; expected %q (use --file or run `%s`)", ptr.Kind, want, lastDraftRerunHint(scope, want))
	}
	abs, err := filepath.Abs(ptr.Path)
	if err != nil {
		return "", err
	}
	st, err := fileutil.Stat(abs)
	if err != nil {
		if fileutil.IsNotExist(err) {
			return "", errfmt.Errorf("%s", paths.RewriteCanonicalCLIInvocations(fmt.Sprintf("last draft file missing: %s (run `zqk new` again or use --file)", abs)))
		}
		return "", err
	}
	if st.IsDir() {
		return "", errfmt.Errorf("last draft path is a directory")
	}
	return abs, nil
}

func lastDraftRerunHint(scope, canonicalKind string) string {
	switch scope {
	case LastDraftScopeObject:
		return paths.RewriteCanonicalCLIInvocations(fmt.Sprintf("zqk new object %s", canonicalKind))
	case LastDraftScopeInternal:
		return paths.RewriteCanonicalCLIInvocations(fmt.Sprintf("zqk new internal %s", canonicalKind))
	default:
		return paths.RewriteCanonicalCLIInvocations(fmt.Sprintf("zqk new (scope %s)", scope))
	}
}

// ClearLastDraftPointerIfPath removes the pointer when it points at the same file as a successful create.
func ClearLastDraftPointerIfPath(projectRoot, usedFilePath string) error {
	if projectRoot == emptyValue || usedFilePath == emptyValue {
		return nil
	}
	ptr, err := ReadLastDraftPointer(projectRoot)
	if err != nil || ptr == nil || ptr.Path == emptyValue {
		return err
	}
	usedAbs, err := filepath.Abs(usedFilePath)
	if err != nil {
		return err
	}
	ptrAbs, err := filepath.Abs(ptr.Path)
	if err != nil {
		return err
	}
	if usedAbs != ptrAbs {
		return nil
	}
	_ = fileutil.Remove(LastDraftPointerPath(projectRoot))
	return nil
}
