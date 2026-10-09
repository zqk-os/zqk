package storage

import (
	"context"
	"net/rpc"
	"os"
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestPrivilegedWriterDaemonTypeExists(t *testing.T) {
	t.Parallel()
	_ = PrivilegedWriterDaemon{}
}

func TestPrivilegedWriterAffinity(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	daemon := NewPrivilegedWriterDaemon(nil, tmpDir)

	// 1. Valid matching project root
	args := &IPCWriterArgs{ProjectRoot: tmpDir}
	if err := daemon.checkAffinity(args); err != nil {
		t.Errorf("expected matching project root to pass affinity check, got: %v", err)
	}

	// 2. Relative vs absolute path equivalence via clean
	cleanDir := filepath.Clean(tmpDir)
	argsClean := &IPCWriterArgs{ProjectRoot: cleanDir + "/."}
	if err := daemon.checkAffinity(argsClean); err != nil {
		t.Errorf("expected cleaned project root to match, got: %v", err)
	}

	// 3. Mismatched project root must fail
	otherDir := t.TempDir()
	argsMismatch := &IPCWriterArgs{ProjectRoot: otherDir}
	if err := daemon.checkAffinity(argsMismatch); err == nil {
		t.Errorf("expected affinity mismatch error, got nil")
	}

	// 4. Empty client ProjectRoot must fail when daemon has root configured
	argsEmpty := &IPCWriterArgs{ProjectRoot: ""}
	if err := daemon.checkAffinity(argsEmpty); err == nil {
		t.Errorf("expected error for empty client ProjectRoot, got nil")
	}

	// 5. Nil args must fail
	if err := daemon.checkAffinity(nil); err == nil {
		t.Errorf("expected error for nil args, got nil")
	}

	// 6. Unbound daemon without project root allows any
	unboundDaemon := NewPrivilegedWriterDaemon(nil, "")
	if err := unboundDaemon.checkAffinity(&IPCWriterArgs{ProjectRoot: tmpDir}); err != nil {
		t.Errorf("expected unbound daemon to allow, got: %v", err)
	}
}

type mockPrivilegedWriterTarget struct {
	writes  []string
	deletes []string
	renames []string
}

func (m *mockPrivilegedWriterTarget) WriteObject(ctx context.Context, id, kind string, payload []byte, isDraft bool) error {
	m.writes = append(m.writes, id)
	return nil
}

func (m *mockPrivilegedWriterTarget) DeleteObject(ctx context.Context, id, kind string) error {
	m.deletes = append(m.deletes, id)
	return nil
}

func (m *mockPrivilegedWriterTarget) RenameObject(ctx context.Context, oldID, newID, kind string) error {
	m.renames = append(m.renames, oldID+"->"+newID)
	return nil
}

func TestStartIPCServer_SocketPermissionsAndLifecycle(t *testing.T) {
	t.Parallel()

	tmpDir, err := os.MkdirTemp("/tmp", "pw_ipc")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { fileutil.RemoveAll(tmpDir) })

	socketDir := filepath.Join(tmpDir, "sock")
	socketPath := filepath.Join(socketDir, "pw.sock")

	target := &mockPrivilegedWriterTarget{}
	daemon := NewPrivilegedWriterDaemon(target, tmpDir)

	listener, err := StartIPCServer(socketPath, daemon)
	if err != nil {
		t.Fatalf("StartIPCServer failed: %v", err)
	}
	defer listener.Close()

	// Verify directory permissions 0700
	dirInfo, err := fileutil.Stat(socketDir)
	if err != nil {
		t.Fatalf("failed to stat socket dir: %v", err)
	}
	if dirInfo.Mode().Perm() != paths.DirPerm700 {
		t.Errorf("expected socket dir perm %v, got %v", paths.DirPerm700, dirInfo.Mode().Perm())
	}

	// Verify socket file permissions 0600
	sockInfo, err := fileutil.Stat(socketPath)
	if err != nil {
		t.Fatalf("failed to stat socket file: %v", err)
	}
	if sockInfo.Mode().Perm() != paths.FilePerm600 {
		t.Errorf("expected socket file perm %v, got %v", paths.FilePerm600, sockInfo.Mode().Perm())
	}

	// Connect via RPC client
	client, err := rpc.Dial("unix", socketPath)
	if err != nil {
		t.Fatalf("failed to dial IPC server via unix socket: %v", err)
	}
	defer client.Close()

	// 1. WriteObject RPC
	var reply bool
	writeArgs := &IPCWriterArgs{
		ID:          "OBJ-TEST-001",
		Kind:        "test_kind",
		Payload:     []byte("test data"),
		ProjectRoot: tmpDir,
	}
	if err := client.Call("PrivilegedWriterDaemon.WriteObject", writeArgs, &reply); err != nil || !reply {
		t.Fatalf("WriteObject RPC failed: %v (reply: %v)", err, reply)
	}
	if len(target.writes) != 1 || target.writes[0] != "OBJ-TEST-001" {
		t.Errorf("unexpected target writes: %+v", target.writes)
	}

	// 2. RenameObject RPC
	renameArgs := &IPCWriterArgs{
		ID:          "OBJ-TEST-001",
		NewID:       "OBJ-TEST-002",
		Kind:        "test_kind",
		ProjectRoot: tmpDir,
	}
	if err := client.Call("PrivilegedWriterDaemon.RenameObject", renameArgs, &reply); err != nil || !reply {
		t.Fatalf("RenameObject RPC failed: %v (reply: %v)", err, reply)
	}
	if len(target.renames) != 1 || target.renames[0] != "OBJ-TEST-001->OBJ-TEST-002" {
		t.Errorf("unexpected target renames: %+v", target.renames)
	}

	// 3. DeleteObject RPC
	delArgs := &IPCWriterArgs{
		ID:          "OBJ-TEST-002",
		Kind:        "test_kind",
		ProjectRoot: tmpDir,
	}
	if err := client.Call("PrivilegedWriterDaemon.DeleteObject", delArgs, &reply); err != nil || !reply {
		t.Fatalf("DeleteObject RPC failed: %v (reply: %v)", err, reply)
	}
	if len(target.deletes) != 1 || target.deletes[0] != "OBJ-TEST-002" {
		t.Errorf("unexpected target deletes: %+v", target.deletes)
	}
}

func TestStartIPCServer_BoundaryAndErrorHandling(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	target := &mockPrivilegedWriterTarget{}
	daemon := NewPrivilegedWriterDaemon(target, tmpDir)

	// 1. Affinity mismatch rejects RPC calls
	var reply bool
	badArgs := &IPCWriterArgs{
		ID:          "OBJ-FAIL",
		ProjectRoot: "/unauthorized/other/path",
	}
	if err := daemon.WriteObject(badArgs, &reply); err == nil || reply {
		t.Errorf("expected WriteObject to fail on affinity mismatch, got reply=%v", reply)
	}
	if err := daemon.DeleteObject(badArgs, &reply); err == nil || reply {
		t.Errorf("expected DeleteObject to fail on affinity mismatch, got reply=%v", reply)
	}
	if err := daemon.RenameObject(badArgs, &reply); err == nil || reply {
		t.Errorf("expected RenameObject to fail on affinity mismatch, got reply=%v", reply)
	}

	// 2. StartIPCServer rejects invalid socket path
	invalidSocketPath := "/dev/null/impossible/socket.sock"
	if _, err := StartIPCServer(invalidSocketPath, daemon); err == nil {
		t.Errorf("expected StartIPCServer to fail on invalid socket path, got nil")
	}
}
