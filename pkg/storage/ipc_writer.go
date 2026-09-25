package storage

import (
	"context"
	"fmt"
	"net"
	"net/rpc"
	"time"
)

const defaultIPCCallTimeout = 10 * time.Second

// IPCWriter implements PrivilegedWriter over a local UNIX domain socket RPC connection.
type IPCWriter struct {
	client      *rpc.Client
	projectRoot string
}

// Close closes the underlying RPC connection.
func (w *IPCWriter) Close() error {
	if w.client != nil {
		return w.client.Close()
	}
	return nil
}

// NewIPCWriter connects to the privileged helper daemon.
func NewIPCWriter(socketPath string, projectRoots ...string) (*IPCWriter, error) {
	conn, err := net.DialTimeout("unix", socketPath, 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to privileged writer helper at %s: %w", socketPath, err)
	}
	client := rpc.NewClient(conn)
	pr := ""
	if len(projectRoots) > 0 {
		pr = projectRoots[0]
	}
	return &IPCWriter{client: client, projectRoot: pr}, nil
}

func (w *IPCWriter) callWithContext(ctx context.Context, serviceMethod string, args any, reply any) error {
	if ctx == nil {
		ctx = context.Background()
	}
	var cancel context.CancelFunc
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		ctx, cancel = context.WithTimeout(ctx, defaultIPCCallTimeout)
		defer cancel()
	}

	call := w.client.Go(serviceMethod, args, reply, make(chan *rpc.Call, 1))
	select {
	case <-ctx.Done():
		return fmt.Errorf("IPC call %s cancelled or timed out: %w", serviceMethod, ctx.Err())
	case c := <-call.Done:
		return c.Error
	}
}

// WriteObject calls the daemon to write the object.
func (w *IPCWriter) WriteObject(ctx context.Context, id, kind string, payload []byte, isDraft bool) error {
	args := &IPCWriterArgs{ID: id, Kind: kind, Payload: payload, IsDraft: isDraft, ProjectRoot: w.projectRoot}
	var reply bool
	return w.callWithContext(ctx, "PrivilegedWriterDaemon.WriteObject", args, &reply)
}

// DeleteObject calls the daemon to delete the object.
func (w *IPCWriter) DeleteObject(ctx context.Context, id, kind string) error {
	args := &IPCWriterArgs{ID: id, Kind: kind, ProjectRoot: w.projectRoot}
	var reply bool
	return w.callWithContext(ctx, "PrivilegedWriterDaemon.DeleteObject", args, &reply)
}

// RenameObject calls the daemon to rename the object.
func (w *IPCWriter) RenameObject(ctx context.Context, oldID, newID, kind string) error {
	args := &IPCWriterArgs{ID: oldID, NewID: newID, Kind: kind, ProjectRoot: w.projectRoot}
	var reply bool
	return w.callWithContext(ctx, "PrivilegedWriterDaemon.RenameObject", args, &reply)
}

