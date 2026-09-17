package mcp

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lanceman/zqk/pkg/goroutinelabels"

	"github.com/lanceman/zqk/pkg/logging"
)

// BackendDialer defines a function that establishes a connection to the backend server.
type BackendDialer func(ctx context.Context) (net.Conn, error)

// ProxyForwarder implements a resilient proxy between a client (e.g. IDE via stdio)
// and a backend MCP server. It automatically reconnects if the backend goes down.
type ProxyForwarder struct {
	clientIn               *bufio.Reader
	clientOut              *bufio.Writer
	dialer                 BackendDialer
	logger                 *logging.EventLogger
	messagesForwardedTotal atomic.Int64
	reconnectsTotal        atomic.Int64
	forwardErrorsTotal     atomic.Int64

	mu            sync.Mutex
	backendConn   net.Conn
	clientWriteMu sync.Mutex

	reconnectDelay time.Duration
}

// GetProxyForwarderStats returns lifetime counters for messages forwarded, reconnects, and forward errors.
func (p *ProxyForwarder) GetProxyForwarderStats() (forwarded, reconnects, errors int64) {
	if p == nil {
		return 0, 0, 0
	}
	return p.messagesForwardedTotal.Load(), p.reconnectsTotal.Load(), p.forwardErrorsTotal.Load()
}

// NewProxyForwarder creates a new ProxyForwarder.
func NewProxyForwarder(clientIn io.Reader, clientOut io.Writer, dialer BackendDialer, logger *logging.EventLogger) *ProxyForwarder {
	if logger == nil {
		logger = logging.GetLogger()
	}
	return &ProxyForwarder{
		clientIn:       bufio.NewReader(clientIn),
		clientOut:      bufio.NewWriter(clientOut),
		dialer:         dialer,
		logger:         logger,
		reconnectDelay: 100 * time.Millisecond,
	}
}

// SetReconnectDelay overrides the default reconnect delay (useful for testing).
func (p *ProxyForwarder) SetReconnectDelay(d time.Duration) {
	p.reconnectDelay = d
}

// Start begins the proxy loops. It blocks until the context is canceled or clientIn EOFs.
func (p *ProxyForwarder) Start(ctx context.Context) error {
	type clientMsg struct {
		data   []byte
		format *MessageFormat
	}
	clientMsgs := make(chan clientMsg)

	// 1. Read from client continuously
	goroutinelabels.StartNamedGoroutine("mcp-proxy-forwarder", "forward MCP proxy requests", func() {
		func() {
			defer close(clientMsgs)
			transport := NewDefaultTransport()
			for {
				data, format, err := transport.ReadMessage(p.clientIn)
				if err != nil {
					if !errors.Is(err, io.EOF) {
						p.logger.LogError("Failed to read from client", err)
					}
					return
				}
				select {
				case <-ctx.Done():
					return
				case clientMsgs <- clientMsg{data: data, format: format}:
				}
			}
		}()
	})

	// 2. Reconnect loop and writing to backend
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		conn, err := p.dialer(ctx)
		if err != nil {
			p.logger.LogError("Failed to connect to backend, retrying...", err)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(p.reconnectDelay):
			}
			continue
		}

		p.mu.Lock()
		p.backendConn = conn
		p.mu.Unlock()
		p.reconnectsTotal.Add(1)

		// Start reading from backend
		backendDone := make(chan struct{})
		goroutinelabels.NewGoroutine("mcp.read_backend", "reading from mcp backend").StartSimple(func() {
			p.readFromBackend(conn, backendDone)
		})

		// Process client messages and send to backend
		transport := NewDefaultTransport()
		backendWriter := bufio.NewWriter(conn)

		backendActive := true
		for backendActive {
			select {
			case <-ctx.Done():
				_ = conn.Close()
				return ctx.Err()
			case <-backendDone:
				backendActive = false
				// Backend closed/errored, break to reconnect
			case msg, ok := <-clientMsgs:
				if !ok {
					// Client closed
					_ = conn.Close()
					return nil
				}
				err := transport.WriteMessage(backendWriter, msg.data, msg.format)
				if err != nil {
					p.logger.LogError("Failed to write to backend", err)
					p.forwardErrorsTotal.Add(1)
					backendActive = false
				} else {
					p.messagesForwardedTotal.Add(1)
				}
			}
		}

		_ = conn.Close()
		p.mu.Lock()
		p.backendConn = nil
		p.mu.Unlock()
	}
}

func (p *ProxyForwarder) readFromBackend(conn net.Conn, done chan struct{}) {
	defer close(done)
	transport := NewDefaultTransport()
	reader := bufio.NewReader(conn)

	for {
		data, format, err := transport.ReadMessage(reader)
		if err != nil {
			if !errors.Is(err, io.EOF) && !isClosedConnError(err) {
				p.logger.LogError("Failed to read from backend", err)
			}
			return
		}

		p.clientWriteMu.Lock()
		err = transport.WriteMessage(p.clientOut, data, format)
		p.clientWriteMu.Unlock()

		if err != nil {
			p.logger.LogError("Failed to write to client", err)
			p.forwardErrorsTotal.Add(1)
			return
		}
		p.messagesForwardedTotal.Add(1)
	}
}

func isClosedConnError(err error) bool {
	if err == nil {
		return false
	}
	return err.Error() == "use of closed network connection" || netErrIsClosed(err)
}

func netErrIsClosed(err error) bool {
	opErr := &net.OpError{}
	ok := errors.As(err, &opErr)
	if ok && opErr.Err != nil && opErr.Err.Error() == "use of closed network connection" {
		return true
	}
	return false
}
