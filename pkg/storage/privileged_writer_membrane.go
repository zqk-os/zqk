package storage

import (
	caspkg "github.com/zqk-os/zqk/pkg/storage/cas"
	"github.com/zqk-os/zqk/pkg/zqkenv"

	"context"
	"path/filepath"
	"strings"

	"github.com/zqk-os/zqk/pkg/brand"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// socketFileExtension is the UNIX-domain-socket file suffix used by helper daemons.
const socketFileExtension = ".sock"

// privilegedWriterSocketBasename is the socket file name (without brand prefix or extension).
const privilegedWriterSocketBasename = "privileged-writer"

// ProjectScopedPrivilegedWriterSocketPath returns the UNIX socket path scoped to a specific projectRoot.
// Privileged writer sockets are located under .zqk/run/ inside the project root.
func ProjectScopedPrivilegedWriterSocketPath(projectRoot string) string {
	name := brand.NamespacePrefix() + "-" + privilegedWriterSocketBasename + socketFileExtension
	if projectRoot == "" {
		projectRoot = paths.ResolveProjectRoot(".")
	}
	return filepath.Join(projectRoot, paths.ProjectDataDir, "run", name)
}

// DefaultPrivilegedWriterSocketPath returns the default UNIX socket path.
// If a project root can be resolved, it returns the project-scoped socket path.
// Otherwise, it falls back to a temporary path under runtime temp dir.
func DefaultPrivilegedWriterSocketPath(projectRoots ...string) string {
	if len(projectRoots) > 0 && projectRoots[0] != "" {
		return ProjectScopedPrivilegedWriterSocketPath(projectRoots[0])
	}
	root := paths.ResolveProjectRoot(".")
	if root != "" {
		return ProjectScopedPrivilegedWriterSocketPath(root)
	}
	name := brand.NamespacePrefix() + "-" + privilegedWriterSocketBasename + socketFileExtension
	return filepath.Join(fileutil.TempDir(), name)
}

func isGlobalTempSocket(path string) bool {
	clean := filepath.Clean(path)
	tempDir := filepath.Clean(fileutil.TempDir())
	return strings.HasPrefix(clean, tempDir) ||
		strings.HasPrefix(clean, "/tmp") ||
		strings.HasPrefix(clean, "/var/tmp") ||
		strings.HasPrefix(clean, "/private/tmp")
}

func resolvePrivilegedWriterSocketPath(projectRoots ...string) string {
	if v := strings.TrimSpace(zqkenv.PrivilegedWriterSocket().Get()); v != "" {
		return v
	}
	return DefaultPrivilegedWriterSocketPath(projectRoots...)
}

func privilegedWriterSocketExists(projectRoots ...string) bool {
	path := resolvePrivilegedWriterSocketPath(projectRoots...)
	// Standalone open-core must NEVER connect to a global tmp socket unless explicitly configured via PRIVILEGED_WRITER_SOCKET.
	if strings.TrimSpace(zqkenv.PrivilegedWriterSocket().Get()) == "" {
		if isGlobalTempSocket(path) {
			return false
		}
	}
	_, err := fileutil.Stat(path)
	return err == nil
}

// privilegedWriterLocalWriteAllowed reports whether CAS may write locally when the
// PrivilegedWriter daemon is down. Production stays fail-closed; tests opt in via
// ZQK_TEST_ALLOW_CAS_FALLTHROUGH=1 (see pkg/testing test setup).
// — TestRoot auto-allow is
// belt-and-suspenders with zqkenv.ApplyIsolatedStorageEnv; prefer explicit fallthrough flag.
func privilegedWriterLocalWriteAllowed(projectRoots ...string) bool {
	if zqkenv.PrivilegedWriterDaemonRole() {
		return true
	}
	if zqkenv.TestAllowCASFallthrough().Get() == "0" {
		return false
	}
	if zqkenv.TestRoot().Get() != "" {
		return true
	}
	if zqkenv.TestAllowCASFallthrough().Get() == "1" {
		return true
	}
	for _, pr := range projectRoots {
		if pr != "" && IsTestOrTempProjectRoot(pr) {
			return true
		}
	}
	if zqkenv.IsInTest() {
		return true
	}
	// In Mode B / enforced cellular membrane, missing daemon socket is a failure, not a fallback (F-SEC-002).
	if isCellularMembraneEnforced(projectRoots...) {
		return false
	}
	// Socket-absent default: if PrivilegedWriter socket does not exist, write locally (open-core / standalone).
	if !privilegedWriterSocketExists(projectRoots...) {
		return true
	}
	return false
}

func isCellularMembraneEnforced(projectRoots ...string) bool {
	if zqkenv.CellularMembraneModeB().Get() == "1" || zqkenv.EnforceCellularMembrane().Get() == "1" {
		return true
	}
	for _, pr := range projectRoots {
		if pr == "" {
			continue
		}
		if fileutil.Exists(filepath.Join(pr, paths.ProjectDataDir, "mode_b")) ||
			fileutil.Exists(filepath.Join(pr, paths.ProjectDataDir, "cellular_membrane_lockdown")) {
			return true
		}
	}
	return false
}

func dialPrivilegedWriter(projectRoots ...string) (*IPCWriter, error) {
	path := resolvePrivilegedWriterSocketPath(projectRoots...)
	// Standalone open-core must NEVER connect to a global tmp socket unless explicitly configured via PRIVILEGED_WRITER_SOCKET.
	if strings.TrimSpace(zqkenv.PrivilegedWriterSocket().Get()) == "" {
		if isGlobalTempSocket(path) {
			return nil, errfmt.Errorf("refusing to connect to global tmp socket: %s", path)
		}
	}
	return NewIPCWriter(path, projectRoots...)
}

func errPrivilegedWriterUnavailable(err error) error {
	return errfmt.Newf("membrane is fail-closed: PrivilegedWriter daemon unavailable").Wrap(err)
}

// writeObjectViaPrivilegedWriter writes through the PW membrane (fail-closed on dial failure).
func writeObjectViaPrivilegedWriter(ctx context.Context, id, kind string, data []byte) error {
	w, err := dialPrivilegedWriter()
	if err != nil {
		return errPrivilegedWriterUnavailable(err)
	}
	defer w.Close()
	return w.WriteObject(ctx, id, kind, data, false)
}

// writeCASThroughMembrane routes CAS/draft writes through PrivilegedWriter in production.
// When test local-write is allowed (ZQK_TEST_ROOT or ZQK_TEST_ALLOW_CAS_FALLTHROUGH=1 or IsTestOrTempProjectRoot),
// always use localFn and skip the daemon — a live LaunchAgent would otherwise write into
// the studio tree while the test asserts paths under an isolated root (ghost draft create).
// TRACK: membrane must not leak test creates into live CAS.
func (f *FileObjectStorage) writeCASThroughMembrane(ctx context.Context, id, kind string, data []byte, isDraft bool, localFn func() error) error {
	if err := caspkg.RefuseCriteriaCASWithoutCategory(kind, isDraft, data); err != nil {
		return err
	}
	if err := caspkg.RefuseCASWithoutDescription(kind, isDraft, data); err != nil {
		return err
	}
	root := ""
	if f != nil {
		root = f.projectRoot
	}
	if privilegedWriterLocalWriteAllowed(root) {
		return localFn()
	}
	w, err := dialPrivilegedWriter(root)
	if err != nil {
		return errPrivilegedWriterUnavailable(err)
	}
	defer w.Close()
	return w.WriteObject(ctx, id, kind, data, isDraft)
}

func writeCASThroughMembrane(ctx context.Context, id, kind string, data []byte, isDraft bool, localFn func() error) error {
	if err := caspkg.RefuseCriteriaCASWithoutCategory(kind, isDraft, data); err != nil {
		return err
	}
	if err := caspkg.RefuseCASWithoutDescription(kind, isDraft, data); err != nil {
		return err
	}
	if privilegedWriterLocalWriteAllowed() {
		return localFn()
	}
	w, err := dialPrivilegedWriter()
	if err != nil {
		return errPrivilegedWriterUnavailable(err)
	}
	defer w.Close()
	return w.WriteObject(ctx, id, kind, data, isDraft)
}

// renameCASThroughMembrane is the ID-change counterpart of writeCASThroughMembrane.
func (f *FileObjectStorage) renameCASThroughMembrane(ctx context.Context, id, newID, kind string, localFn func() error) error {
	root := ""
	if f != nil {
		root = f.projectRoot
	}
	if privilegedWriterLocalWriteAllowed(root) {
		return localFn()
	}
	w, err := dialPrivilegedWriter(root)
	if err != nil {
		return errPrivilegedWriterUnavailable(err)
	}
	defer w.Close()
	return w.RenameObject(ctx, id, newID, kind)
}

func renameCASThroughMembrane(ctx context.Context, id, newID, kind string, localFn func() error) error {
	if privilegedWriterLocalWriteAllowed() {
		return localFn()
	}
	w, err := dialPrivilegedWriter()
	if err != nil {
		return errPrivilegedWriterUnavailable(err)
	}
	defer w.Close()
	return w.RenameObject(ctx, id, newID, kind)
}

// deleteCASThroughMembrane is the delete counterpart of writeCASThroughMembrane.
func (f *FileObjectStorage) deleteCASThroughMembrane(ctx context.Context, id, kind string, localFn func() error) error {
	root := ""
	if f != nil {
		root = f.projectRoot
	}
	if privilegedWriterLocalWriteAllowed(root) {
		return localFn()
	}
	w, err := dialPrivilegedWriter(root)
	if err != nil {
		return errPrivilegedWriterUnavailable(err)
	}
	defer w.Close()
	return w.DeleteObject(ctx, id, kind)
}

func deleteCASThroughMembrane(ctx context.Context, id, kind string, localFn func() error) error {
	if privilegedWriterLocalWriteAllowed() {
		return localFn()
	}
	w, err := dialPrivilegedWriter()
	if err != nil {
		return errPrivilegedWriterUnavailable(err)
	}
	defer w.Close()
	return w.DeleteObject(ctx, id, kind)
}
