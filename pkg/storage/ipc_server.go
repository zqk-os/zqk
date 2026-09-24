package storage

import (
	"context"
	"fmt"
	"net"
	"net/rpc"
	"path/filepath"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

// IPCWriterArgs defines the arguments for daemon RPC calls.
type IPCWriterArgs struct {
	ID          string
	NewID       string // Used for Rename
	Kind        string
	Payload     []byte
	IsDraft     bool
	ProjectRoot string
}

// PrivilegedWriterDaemon represents the RPC server side implementation.
type PrivilegedWriterDaemon struct {
	// targetStorage is the actual filesystem storage implementation
	// that runs with elevated privileges.
	targetStorage PrivilegedWriter
	projectRoot   string
}

// NewPrivilegedWriterDaemon creates a new daemon instance.
func NewPrivilegedWriterDaemon(target PrivilegedWriter, projectRoots ...string) *PrivilegedWriterDaemon {
	pr := ""
	if len(projectRoots) > 0 {
		pr = projectRoots[0]
	}
	return &PrivilegedWriterDaemon{targetStorage: target, projectRoot: pr}
}

func (d *PrivilegedWriterDaemon) checkAffinity(args *IPCWriterArgs) error {
	if d.projectRoot == "" || args == nil || args.ProjectRoot == "" {
		return nil
	}
	dClean := filepath.Clean(d.projectRoot)
	aClean := filepath.Clean(args.ProjectRoot)
	if dClean != aClean {
		return fmt.Errorf("project root affinity mismatch: client requested %s but daemon serves %s", aClean, dClean)
	}
	return nil
}

// WriteObject is the RPC handler.
func (d *PrivilegedWriterDaemon) WriteObject(args *IPCWriterArgs, reply *bool) error {
	if err := d.checkAffinity(args); err != nil {
		*reply = false
		return err
	}
	err := d.targetStorage.WriteObject(context.Background(), args.ID, args.Kind, args.Payload, args.IsDraft) // Background: request-or-shutdown derived
	*reply = (err == nil)
	return err
}

// DeleteObject is the RPC handler.
func (d *PrivilegedWriterDaemon) DeleteObject(args *IPCWriterArgs, reply *bool) error {
	if err := d.checkAffinity(args); err != nil {
		*reply = false
		return err
	}
	err := d.targetStorage.DeleteObject(context.Background(), args.ID, args.Kind) // Background: request-or-shutdown derived
	*reply = (err == nil)
	return err
}

// RenameObject is the RPC handler.
func (d *PrivilegedWriterDaemon) RenameObject(args *IPCWriterArgs, reply *bool) error {
	if err := d.checkAffinity(args); err != nil {
		*reply = false
		return err
	}
	err := d.targetStorage.RenameObject(context.Background(), args.ID, args.NewID, args.Kind) // Background: request-or-shutdown derived
	*reply = (err == nil)
	return err
}

// StartIPCServer starts listening on the given socket path.
func StartIPCServer(socketPath string, daemon *PrivilegedWriterDaemon) (net.Listener, error) {
	server := rpc.NewServer()
	if err := server.Register(daemon); err != nil {
		return nil, fmt.Errorf("failed to register daemon RPC: %w", err)
	}

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("failed to listen on %s: %w", socketPath, err)
	}

	goroutinelabels.NewGoroutine("storage.ipc_server_accept", "accepting IPC server connections").StartSimple(func() {
		server.Accept(listener)
	})
	return listener, nil
}
