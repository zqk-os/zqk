package overseer

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net"
	"path/filepath"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// IPCRequest represents a command sent over the domain socket.
type IPCRequest struct {
	Action string      `json:"action"` // "status", "start", "stop", "restart", "enable", "disable", "add", "remove", "shutdown"
	Target string      `json:"target,omitempty"`
	Spec   *DaemonSpec `json:"spec,omitempty"`
}

// IPCResponse represents the overseer response envelope.
type IPCResponse struct {
	Success bool           `json:"success"`
	Error   string         `json:"error,omitempty"`
	Daemons []DaemonStatus `json:"daemons,omitempty"`
}

// SocketPath returns the canonical path to the overseer Unix socket.
func SocketPath(projectRoot string) string {
	return filepath.Join(projectRoot, paths.ProjectDataDir, paths.StateDir, "daemon", "overseer.sock")
}

// IPCServer handles local client connections over a domain socket.
type IPCServer struct {
	socketPath string
	listener   net.Listener
	supervisor *Supervisor
	mu         sync.Mutex
	stopCh     chan struct{}
	wg         sync.WaitGroup
}

// NewIPCServer creates an IPCServer instance.
func NewIPCServer(socketPath string, supervisor *Supervisor) *IPCServer {
	return &IPCServer{
		socketPath: socketPath,
		supervisor: supervisor,
		stopCh:     make(chan struct{}),
	}
}

// Start begins listening on the Unix domain socket.
func (s *IPCServer) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	dir := filepath.Dir(s.socketPath)
	if err := fileutil.MkdirAll(dir, paths.DirPerm700); err != nil {
		return errfmt.Newf("mkdir socket dir %s", dir).Wrap(err)
	}

	// Remove stale socket if exists
	_ = fileutil.Remove(s.socketPath)

	l, err := net.Listen("unix", s.socketPath)
	if err != nil {
		return errfmt.Newf("listen on unix socket %s", s.socketPath).Wrap(err)
	}
	_ = fileutil.Chmod(s.socketPath, paths.FilePerm600)
	s.listener = l

	goroutinelabels.NewGoroutine("overseer_ipc_serve", "listen and accept overseer IPC connections").
		StartSimple(s.serve)
	return nil
}

// Stop closes the domain socket listener and drains active connections.
func (s *IPCServer) Stop() error {
	s.mu.Lock()
	select {
	case <-s.stopCh:
		s.mu.Unlock()
		return nil
	default:
		close(s.stopCh)
	}

	if s.listener != nil {
		_ = s.listener.Close()
	}
	_ = fileutil.Remove(s.socketPath)
	s.mu.Unlock()

	// Wait for active client connections to finish with a capped deadline
	done := make(chan struct{})
	goroutinelabels.NewGoroutine("overseer_ipc_drain", "drain active IPC connections on shutdown").
		StartSimple(func() {
			s.wg.Wait()
			close(done)
		})

	select {
	case <-done:
	case <-time.After(1 * time.Second):
	}
	return nil
}

func (s *IPCServer) serve() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-s.stopCh:
				return
			default:
			}
			if errors.Is(err, net.ErrClosed) {
				return
			}
			// Transient accept error: back off briefly to avoid tight spin loop
			time.Sleep(50 * time.Millisecond)
			continue
		}

		s.wg.Add(1)
		goroutinelabels.NewGoroutine("overseer_ipc_conn", "handle client request on overseer domain socket").
			WithCleanup(s.wg.Done).
			StartSimple(func() {
				s.handleConnection(conn)
			})
	}
}

func (s *IPCServer) handleConnection(conn net.Conn) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))

	scanner := bufio.NewScanner(conn)
	if !scanner.Scan() {
		return
	}

	var req IPCRequest
	if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
		_ = json.NewEncoder(conn).Encode(IPCResponse{
			Success: false,
			Error:   "invalid request envelope: " + err.Error(),
		})
		return
	}

	resp := s.executeRequest(req)
	_ = json.NewEncoder(conn).Encode(resp)
}

func (s *IPCServer) executeRequest(req IPCRequest) IPCResponse {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	switch req.Action {
	case "status":
		if req.Target != "" {
			st, err := s.supervisor.GetStatus(req.Target)
			if err != nil {
				return IPCResponse{Success: false, Error: err.Error()}
			}
			return IPCResponse{Success: true, Daemons: []DaemonStatus{*st}}
		}
		return IPCResponse{Success: true, Daemons: s.supervisor.GetAllStatuses()}

	case "start", "enable":
		if req.Target == "" {
			return IPCResponse{Success: false, Error: "missing target daemon name"}
		}
		if _, ok := s.supervisor.registry.Get(req.Target); !ok {
			return IPCResponse{Success: false, Error: "unknown daemon: " + req.Target}
		}
		if err := s.supervisor.Enable(ctx, req.Target); err != nil {
			return IPCResponse{Success: false, Error: err.Error()}
		}
		st, _ := s.supervisor.GetStatus(req.Target)
		var list []DaemonStatus
		if st != nil {
			list = []DaemonStatus{*st}
		}
		return IPCResponse{Success: true, Daemons: list}

	case "stop", "disable":
		if req.Target == "" {
			return IPCResponse{Success: false, Error: "missing target daemon name"}
		}
		if _, ok := s.supervisor.registry.Get(req.Target); !ok {
			return IPCResponse{Success: false, Error: "unknown daemon: " + req.Target}
		}
		if err := s.supervisor.Disable(ctx, req.Target); err != nil {
			return IPCResponse{Success: false, Error: err.Error()}
		}
		st, _ := s.supervisor.GetStatus(req.Target)
		var list []DaemonStatus
		if st != nil {
			list = []DaemonStatus{*st}
		}
		return IPCResponse{Success: true, Daemons: list}

	case "restart":
		if req.Target == "" {
			return IPCResponse{Success: false, Error: "missing target daemon name"}
		}
		if _, ok := s.supervisor.registry.Get(req.Target); !ok {
			return IPCResponse{Success: false, Error: "unknown daemon: " + req.Target}
		}
		if err := s.supervisor.Restart(ctx, req.Target); err != nil {
			return IPCResponse{Success: false, Error: err.Error()}
		}
		st, _ := s.supervisor.GetStatus(req.Target)
		var list []DaemonStatus
		if st != nil {
			list = []DaemonStatus{*st}
		}
		return IPCResponse{Success: true, Daemons: list}

	case "add":
		if req.Spec == nil || req.Spec.Name == "" {
			return IPCResponse{Success: false, Error: "missing daemon specification or name"}
		}
		if err := s.supervisor.AddDaemon(ctx, req.Spec); err != nil {
			return IPCResponse{Success: false, Error: err.Error()}
		}
		st, _ := s.supervisor.GetStatus(req.Spec.Name)
		var list []DaemonStatus
		if st != nil {
			list = []DaemonStatus{*st}
		}
		return IPCResponse{Success: true, Daemons: list}

	case "remove":
		if req.Target == "" {
			return IPCResponse{Success: false, Error: "missing target daemon name"}
		}
		if err := s.supervisor.RemoveDaemon(ctx, req.Target); err != nil {
			return IPCResponse{Success: false, Error: err.Error()}
		}
		return IPCResponse{Success: true}

	default:
		return IPCResponse{Success: false, Error: "unknown action: " + req.Action}
	}
}

// IPCClient facilitates communication with a running Overseer IPCServer.
type IPCClient struct {
	socketPath string
}

// NewIPCClient initializes an IPC client pointing to the domain socket.
func NewIPCClient(socketPath string) *IPCClient {
	return &IPCClient{socketPath: socketPath}
}

// Send dispatches an IPCRequest and returns the IPCResponse.
func (c *IPCClient) Send(ctx context.Context, req IPCRequest) (*IPCResponse, error) {
	var d net.Dialer
	conn, err := d.DialContext(ctx, "unix", c.socketPath)
	if err != nil {
		return nil, errfmt.Newf("dial overseer socket %s", c.socketPath).Wrap(err)
	}
	defer conn.Close()

	b, err := json.Marshal(req)
	if err != nil {
		return nil, errfmt.Newf("marshal request").Wrap(err)
	}
	b = append(b, '\n')

	if _, err := conn.Write(b); err != nil {
		return nil, errfmt.Newf("write to socket").Wrap(err)
	}

	var resp IPCResponse
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		return nil, errfmt.Newf("decode response").Wrap(err)
	}

	return &resp, nil
}

// IsRunning tests if an overseer instance is currently responding on the socket.
func (c *IPCClient) IsRunning() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	resp, err := c.Send(ctx, IPCRequest{Action: "status"})
	return err == nil && resp != nil && resp.Success
}
