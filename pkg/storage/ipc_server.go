package storage

import (
	"context"
	"fmt"
	"net"
	"net/rpc"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

// IPCWriterArgs defines the arguments for daemon RPC calls.
type IPCWriterArgs struct {
	ID      string
	NewID   string // Used for Rename
	Kind    string
	Payload []byte
	IsDraft bool
}

// PrivilegedWriterDaemon represents the RPC server side implementation.
type PrivilegedWriterDaemon struct {
	// targetStorage is the actual filesystem storage implementation
	// that runs with elevated privileges.
	targetStorage PrivilegedWriter
}

// NewPrivilegedWriterDaemon creates a new daemon instance.
func NewPrivilegedWriterDaemon(target PrivilegedWriter) *PrivilegedWriterDaemon {
	return &PrivilegedWriterDaemon{targetStorage: target}
}

// WriteObject is the RPC handler.
func (d *PrivilegedWriterDaemon) WriteObject(args *IPCWriterArgs, reply *bool) error {
	err := d.targetStorage.WriteObject(context.Background(), args.ID, args.Kind, args.Payload, args.IsDraft) // Background: request-or-shutdown derived
	*reply = (err == nil)
	return err
}

// DeleteObject is the RPC handler.
func (d *PrivilegedWriterDaemon) DeleteObject(args *IPCWriterArgs, reply *bool) error {
	err := d.targetStorage.DeleteObject(context.Background(), args.ID, args.Kind) // Background: request-or-shutdown derived
	*reply = (err == nil)
	return err
}

// RenameObject is the RPC handler.
func (d *PrivilegedWriterDaemon) RenameObject(args *IPCWriterArgs, reply *bool) error {
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
