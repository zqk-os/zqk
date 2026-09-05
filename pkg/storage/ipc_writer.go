package storage

import (
	"context"
	"fmt"
	"net/rpc"
)

// IPCWriter implements PrivilegedWriter over a local UNIX domain socket RPC connection.
type IPCWriter struct {
	client *rpc.Client
}

// Close closes the underlying RPC connection.
func (w *IPCWriter) Close() error {
	if w.client != nil {
		return w.client.Close()
	}
	return nil
}

// NewIPCWriter connects to the privileged helper daemon.
func NewIPCWriter(socketPath string) (*IPCWriter, error) {
	client, err := rpc.Dial("unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to privileged writer helper at %s: %w", socketPath, err)
	}
	return &IPCWriter{client: client}, nil
}

// WriteObject calls the daemon to write the object.
func (w *IPCWriter) WriteObject(ctx context.Context, id, kind string, payload []byte, isDraft bool) error {
	args := &IPCWriterArgs{ID: id, Kind: kind, Payload: payload, IsDraft: isDraft}
	var reply bool
	return w.client.Call("PrivilegedWriterDaemon.WriteObject", args, &reply)
}

// DeleteObject calls the daemon to delete the object.
func (w *IPCWriter) DeleteObject(ctx context.Context, id, kind string) error {
	args := &IPCWriterArgs{ID: id, Kind: kind}
	var reply bool
	return w.client.Call("PrivilegedWriterDaemon.DeleteObject", args, &reply)
}

// RenameObject calls the daemon to rename the object.
func (w *IPCWriter) RenameObject(ctx context.Context, oldID, newID, kind string) error {
	args := &IPCWriterArgs{ID: oldID, NewID: newID, Kind: kind}
	var reply bool
	return w.client.Call("PrivilegedWriterDaemon.RenameObject", args, &reply)
}
