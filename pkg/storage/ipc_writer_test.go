package storage

import (
	"context"
	"net"
	"net/rpc"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type mockHangingDaemon struct{}

func (d *mockHangingDaemon) WriteObject(args *IPCWriterArgs, reply *bool) error {
	time.Sleep(2 * time.Second)
	*reply = true
	return nil
}

func TestIPCWriter_ContextCancellation(t *testing.T) {
	// Setup a real Unix domain socket with a hanging RPC receiver in /tmp to satisfy macOS path limit
	sockDir, err := os.MkdirTemp("/tmp", "ipctest")
	if err != nil {
		t.Fatalf("mkdir temp: %v", err)
	}
	defer os.RemoveAll(sockDir)
	sockPath := filepath.Join(sockDir, "mock.sock")

	server := rpc.NewServer()
	if err := server.RegisterName("PrivilegedWriterDaemon", &mockHangingDaemon{}); err != nil {
		t.Fatalf("register mock daemon: %v", err)
	}

	listener, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("listen on unix socket: %v", err)
	}
	defer listener.Close()

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go server.ServeConn(conn)
		}
	}()

	writer, err := NewIPCWriter(sockPath)
	if err != nil {
		t.Fatalf("NewIPCWriter: %v", err)
	}
	defer writer.Close()

	// Call WriteObject with a context that times out quickly (50ms)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	err = writer.WriteObject(ctx, "OBJ-1", "backlog_item", []byte("data"), false)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected error due to context timeout, got nil")
	}
	if !strings.Contains(err.Error(), "cancelled or timed out") {
		t.Fatalf("unexpected error message: %v", err)
	}
	if elapsed > 1*time.Second {
		t.Fatalf("call took %v; expected fast timeout cancellation", elapsed)
	}
}

func TestIPCWriter_DialTimeout(t *testing.T) {
	// Attempt to connect to a non-existent socket path
	sockDir := t.TempDir()
	sockPath := filepath.Join(sockDir, "non_existent.sock")

	_, err := NewIPCWriter(sockPath)
	if err == nil {
		t.Fatal("expected dial failure, got nil")
	}
}
