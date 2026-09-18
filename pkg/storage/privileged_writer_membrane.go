package storage

import (
	caspkg "github.com/zqk-os/zqk/pkg/storage/cas"
	"github.com/zqk-os/zqk/pkg/zqkenv"

	"context"
	"path/filepath"
	"strings"

	"github.com/zqk-os/zqk/pkg/brand"
	"github.com/zqk-os/zqk/pkg/errfmt"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// socketFileExtension is the UNIX-domain-socket file suffix used by helper daemons.
const socketFileExtension = ".sock"

// privilegedWriterSocketBasename is the socket file name (without brand prefix or extension).
const privilegedWriterSocketBasename = "privileged-writer"

// DefaultPrivilegedWriterSocketPath returns the default UNIX socket path for the
// PrivilegedWriter helper (LaunchAgent / KeepAlive).
//
// Rendezvous is always under /tmp (not [os.TempDir]): macOS TMPDIR is per-user and
// changes across sessions, which desyncs a KeepAlive LaunchAgent from CLI clients.
// The basename uses [brand.NamespacePrefix] rather than a hardcoded "zqk" token.
// Override with PRIVILEGED_WRITER_SOCKET. TRACK: BLI-CAS-HAND-DUP-CHECK-001
func DefaultPrivilegedWriterSocketPath() string {
	name := brand.NamespacePrefix() + "-" + privilegedWriterSocketBasename + socketFileExtension
	return filepath.Join("/tmp", name)
}

func privilegedWriterMembraneConfigured() bool {
	if strings.TrimSpace(zqkenv.PrivilegedWriterSocket().Get()) != "" {
		return true
	}
	_, err := fileutil.Stat(DefaultPrivilegedWriterSocketPath())
	return err == nil
}

// privilegedWriterLocalWriteAllowed reports whether CAS may write locally when the
// PrivilegedWriter daemon is down. Production stays fail-closed; tests opt in via
// ZQK_TEST_ALLOW_CAS_FALLTHROUGH=1 (see pkg/testing test setup).
// TRACK: BLI-COMMS-CURSOR-TPM-DELIVER-ATTN-001 (test harness) — TestRoot auto-allow is
// belt-and-suspenders with zqkenv.ApplyIsolatedStorageEnv; prefer explicit fallthrough flag.
func privilegedWriterLocalWriteAllowed(projectRoots ...string) bool {
	// We are the membrane endpoint — never Dial our own socket (self-RPC until EMFILE).
	// TRACK: BLI-CEF-R20-SINGLE-WRITER-BLI-001
	if zqkenv.PrivilegedWriterDaemonRole() {
		return true
	}
	// No membrane in this environment: write locally. Fail-closed only when a socket is configured or live.
	if !privilegedWriterMembraneConfigured() {
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
	return zqkenv.IsInTest()
}

func dialPrivilegedWriter() (*IPCWriter, error) {
	path := DefaultPrivilegedWriterSocketPath()
	if v := strings.TrimSpace(zqkenv.PrivilegedWriterSocket().Get()); v != "" {
		path = v
	}
	return NewIPCWriter(path)
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
// TRACK: BLI-1785886134649966000-7732876c — membrane must not leak test creates into live CAS.
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
	w, err := dialPrivilegedWriter()
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
	w, err := dialPrivilegedWriter()
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
	w, err := dialPrivilegedWriter()
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
