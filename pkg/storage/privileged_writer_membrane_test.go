package storage

import (
	"context"
	"fmt"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/brand"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestPrivilegedWriterLocalWriteAllowed(t *testing.T) {
	t.Setenv(zqkenv.IsDaemon().Name(), "")
	t.Setenv(zqkenv.TestRoot().Name(), "")
	t.Setenv(zqkenv.TestAllowCASFallthrough().Name(), "1")

	if !privilegedWriterLocalWriteAllowed() {
		t.Error("expected local write to be allowed when ZQK_TEST_ALLOW_CAS_FALLTHROUGH=1")
	}

	t.Setenv(zqkenv.TestAllowCASFallthrough().Name(), "0")
	if privilegedWriterLocalWriteAllowed() {
		t.Error("expected local write to not be allowed when ZQK_TEST_ALLOW_CAS_FALLTHROUGH=0")
	}

	t.Setenv(zqkenv.TestAllowCASFallthrough().Name(), "")
	t.Setenv(zqkenv.TestRoot().Name(), t.TempDir())
	if !privilegedWriterLocalWriteAllowed() {
		t.Error("expected local write when ZQK_TEST_ROOT is set (test harness auto-allow)")
	}

	t.Setenv(zqkenv.TestAllowCASFallthrough().Name(), "0")
	if privilegedWriterLocalWriteAllowed() {
		t.Error("expected explicit ZQK_TEST_ALLOW_CAS_FALLTHROUGH=0 to override TestRoot auto-allow")
	}
}

func TestWriteCASThroughMembrane_Fallthrough(t *testing.T) {
	t.Setenv(zqkenv.TestAllowCASFallthrough().Name(), "1")
	// Even if a studio PrivilegedWriter socket is live, test fallthrough must stay local.
	t.Setenv(zqkenv.PrivilegedWriterSocket().Name(), DefaultPrivilegedWriterSocketPath())

	called := false
	localFn := func() error {
		called = true
		return nil
	}

	ctx := context.Background()
	err := writeCASThroughMembrane(ctx, "ID-123", "test_kind", []byte("data"), false, localFn)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !called {
		t.Error("expected localFn to be called")
	}
}

func TestWriteCASThroughMembrane_TestRootSkipsLiveDaemon(t *testing.T) {
	t.Setenv(zqkenv.TestAllowCASFallthrough().Name(), "")
	t.Setenv(zqkenv.TestRoot().Name(), t.TempDir())
	t.Setenv(zqkenv.PrivilegedWriterSocket().Name(), DefaultPrivilegedWriterSocketPath())

	called := false
	err := writeCASThroughMembrane(context.Background(), "ID-123", "test_kind", []byte("data"), true, func() error {
		called = true
		return nil
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !called {
		t.Error("expected localFn when ZQK_TEST_ROOT is set even if PrivilegedWriter is dialable")
	}
}

func TestWriteCASThroughMembrane_DaemonArgvWithoutEnv(t *testing.T) {
	t.Setenv(zqkenv.IsDaemon().Name(), "")
	t.Setenv(zqkenv.TestRoot().Name(), "")
	t.Setenv(zqkenv.TestAllowCASFallthrough().Name(), "0")
	t.Setenv(zqkenv.PrivilegedWriterSocket().Name(), zqkenv.UnreachableTestSocketPath())
	setPrivilegedWriterCommandArgs(t, []string{"zqk-stable", "object", "daemon"})

	called := false
	err := writeCASThroughMembrane(context.Background(), "ID-123", "test_kind", []byte("data"), false, func() error {
		called = true
		return nil
	})
	if err != nil {
		t.Fatalf("expected argv role to write locally without IS_DAEMON, got %v", err)
	}
	if !called {
		t.Error("expected localFn from object daemon argv — Setenv order must not be required")
	}
}

func TestWriteCASThroughMembrane_DaemonWritesLocally(t *testing.T) {
	t.Setenv(zqkenv.TestRoot().Name(), "")
	t.Setenv(zqkenv.TestAllowCASFallthrough().Name(), "0")
	t.Setenv(zqkenv.IsDaemon().Name(), "1")
	t.Setenv(zqkenv.PrivilegedWriterSocket().Name(), zqkenv.UnreachableTestSocketPath())

	called := false
	err := writeCASThroughMembrane(context.Background(), "ID-123", "test_kind", []byte("data"), false, func() error {
		called = true
		return nil
	})
	if err != nil {
		t.Fatalf("expected daemon to write locally, got %v", err)
	}
	if !called {
		t.Error("expected localFn when IS_DAEMON=1 — self-dial is the EMFILE footgun")
	}
	if !privilegedWriterLocalWriteAllowed() {
		t.Error("expected local write allowed for the privileged-writer daemon process")
	}
}

func TestWriteCASThroughMembrane_NoFallthrough(t *testing.T) {
	t.Setenv(zqkenv.TestRoot().Name(), "")
	t.Setenv(zqkenv.IsDaemon().Name(), "")
	t.Setenv(zqkenv.TestAllowCASFallthrough().Name(), "0")

	// Dialing will fail because socket doesn't exist, and fallthrough is disabled, so it should return an error
	called := false
	localFn := func() error {
		called = true
		return nil
	}

	ctx := context.Background()
	err := writeCASThroughMembrane(ctx, "ID-123", "test_kind", []byte("data"), false, localFn)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if called {
		t.Error("expected localFn not to be called")
	}
}

func TestPrivilegedWriter_StandaloneNeverDialsGlobalTmp(t *testing.T) {
	// CRIT-1790218415870917000-98c11401: Standalone open-core never dials global /tmp sockets
	t.Setenv(zqkenv.TestRoot().Name(), "")
	t.Setenv(zqkenv.IsDaemon().Name(), "")
	t.Setenv(zqkenv.TestAllowCASFallthrough().Name(), "")
	t.Setenv(zqkenv.PrivilegedWriterSocket().Name(), "")

	// Create a dummy global tmp socket to simulate Studio daemon running on host
	globalTmpSocket := filepath.Join("/tmp", brand.NamespacePrefix()+"-"+privilegedWriterSocketBasename+socketFileExtension)
	l, err := net.Listen("unix", globalTmpSocket)
	if err == nil {
		defer l.Close()
		defer fileutil.Remove(globalTmpSocket)
	}

	// Standalone in a separate project root without a local socket
	projectRoot := t.TempDir()
	fs := &FileObjectStorage{projectRoot: projectRoot}

	called := false
	localFn := func() error {
		called = true
		return nil
	}

	err = fs.writeCASThroughMembrane(context.Background(), "BLI-1", "backlog_item", []byte("name: test\ndescription: valid long description here\n"), false, localFn)
	if err != nil {
		t.Fatalf("expected write to succeed locally, got error: %v", err)
	}
	if !called {
		t.Fatal("expected localFn to be called because standalone open-core must not dial global /tmp socket")
	}
}

func TestPrivilegedWriter_ProjectRootAffinityMismatch(t *testing.T) {
	// CRIT-1790218415870918000-51892f7c: Privileged writer sockets verify project root affinity before IPC write
	daemonRoot := t.TempDir()
	clientRoot := t.TempDir()

	// Keep UNIX domain socket path short (macOS 104 char limit)
	socketPath := filepath.Join("/tmp", fmt.Sprintf("pw_aff_%d.sock", time.Now().UnixNano()))
	_ = fileutil.Remove(socketPath)
	defer fileutil.Remove(socketPath)

	mock := &mockPrivilegedWriter{}
	daemon := NewPrivilegedWriterDaemon(mock, daemonRoot)

	listener, err := StartIPCServer(socketPath, daemon)
	if err != nil {
		t.Fatalf("failed to start IPC server: %v", err)
	}
	defer listener.Close()

	// Wait for listener ready
	for i := 0; i < 20; i++ {
		conn, dialErr := net.Dial("unix", socketPath)
		if dialErr == nil {
			conn.Close()
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Connect with client belonging to clientRoot
	client, err := NewIPCWriter(socketPath, clientRoot)
	if err != nil {
		t.Fatalf("failed to create IPCWriter: %v", err)
	}
	defer client.Close()

	err = client.WriteObject(context.Background(), "OBJ-1", "backlog_item", []byte("test"), false)
	if err == nil {
		t.Fatal("expected error due to project root affinity mismatch, got nil")
	}
	if !strings.Contains(err.Error(), "affinity mismatch") {
		t.Fatalf("expected affinity mismatch error, got: %v", err)
	}

	// Now connect with client belonging to daemonRoot (matching affinity)
	matchingClient, err := NewIPCWriter(socketPath, daemonRoot)
	if err != nil {
		t.Fatalf("failed to create matching IPCWriter: %v", err)
	}
	defer matchingClient.Close()

	err = matchingClient.WriteObject(context.Background(), "OBJ-1", "backlog_item", []byte("test"), false)
	if err != nil {
		t.Fatalf("expected success with matching affinity, got: %v", err)
	}
}
