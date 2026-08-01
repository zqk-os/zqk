package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
)

// ProxyDaemon acts as a middleman between stdio (IDE) and a TCP MCP daemon.
// It detects disconnects via a heartbeat and transparently reconnects and re-initializes.
type ProxyDaemon struct {
	tcpAddr   string
	logger    logging.Logger
	stdin     io.Reader
	stdout    io.Writer
	transport Transport

	mu          sync.Mutex
	conn        net.Conn
	daemonAlive atomic.Bool

	heartbeatsSentTotal    atomic.Int64
	heartbeatFailuresTotal atomic.Int64
	reconnectionsTotal     atomic.Int64

	initMsg    []byte
	initFormat *MessageFormat
}

// GetProxyDaemonStats returns lifetime counters for heartbeats sent, heartbeat failures, and reconnections.
func (p *ProxyDaemon) GetProxyDaemonStats() (heartbeatsSent, heartbeatFailures, reconnections int64) {
	if p == nil {
		return 0, 0, 0
	}
	return p.heartbeatsSentTotal.Load(), p.heartbeatFailuresTotal.Load(), p.reconnectionsTotal.Load()
}

// NewProxyDaemon creates a new ProxyDaemon
func NewProxyDaemon(tcpAddr string, logger logging.Logger) *ProxyDaemon {
	return &ProxyDaemon{
		tcpAddr:   tcpAddr,
		logger:    logger,
		stdin:     os.Stdin,
		stdout:    os.Stdout,
		transport: NewDefaultTransport(),
	}
}

// Start begins the proxy loops
func (p *ProxyDaemon) Start(ctx context.Context) error {
	reader := bufio.NewReader(p.stdin)

	go p.connectionLoop(ctx)
	go p.heartbeatLoop(ctx)

	for {
		msg, format, err := p.transport.ReadMessage(reader)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return errfmt.Errorf("failed to read from stdio: %w", err)
		}

		// Try to parse as JSON-RPC to sniff initialize
		var rpc struct {
			Method string `json:"method"`
		}
		if err := json.Unmarshal(msg, &rpc); err == nil {
			if rpc.Method == "initialize" {
				p.mu.Lock()
				p.initMsg = msg
				p.initFormat = format
				p.mu.Unlock()
				logging.Fluent(p.logger).Debug("Proxy intercepted initialize message").Log()
			}
		}

		p.forwardToDaemon(ctx, msg, format)
	}
}

func (p *ProxyDaemon) connectionLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		p.mu.Lock()
		conn := p.conn
		p.mu.Unlock()

		if conn == nil {
			newConn, err := net.Dial("tcp", p.tcpAddr)
			if err == nil {
				logging.Fluent(p.logger).Info("Proxy connected to daemon").String("addr", p.tcpAddr).Log()

				p.mu.Lock()
				p.conn = newConn
				p.daemonAlive.Store(true)
				p.reconnectionsTotal.Add(1)

				if len(p.initMsg) > 0 {
					logging.Fluent(p.logger).Info("Proxy re-sending initialize message").Log()
					_ = p.transport.WriteMessage(bufio.NewWriter(newConn), p.initMsg, p.initFormat)
				}
				p.mu.Unlock()

				// Start reading from the daemon
				go p.readFromDaemon(newConn)
			} else {
				time.Sleep(1 * time.Second)
			}
		} else {
			time.Sleep(1 * time.Second)
		}
	}
}

func (p *ProxyDaemon) readFromDaemon(conn net.Conn) {
	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(p.stdout)

	for {
		msg, format, err := p.transport.ReadMessage(reader)
		if err != nil {
			logging.Fluent(p.logger).Warn("Proxy lost connection to daemon").WithError(err).Log()
			p.mu.Lock()
			if p.conn == conn {
				p.conn.Close()
				p.conn = nil
				p.daemonAlive.Store(false)
			}
			p.mu.Unlock()
			return
		}

		// Sniff for heartbeat response
		var rpc struct {
			ID json.RawMessage `json:"id"`
		}
		if err := json.Unmarshal(msg, &rpc); err == nil {
			if string(rpc.ID) == `"__proxy_heartbeat__"` {
				p.daemonAlive.Store(true)
				continue // Do not forward heartbeat response to IDE
			}
		}

		p.mu.Lock()
		_ = p.transport.WriteMessage(writer, msg, format)
		p.mu.Unlock()
	}
}

func (p *ProxyDaemon) forwardToDaemon(ctx context.Context, msg []byte, format *MessageFormat) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		p.mu.Lock()
		conn := p.conn
		p.mu.Unlock()

		if conn != nil {
			writer := bufio.NewWriter(conn)
			if err := p.transport.WriteMessage(writer, msg, format); err != nil {
				logging.Fluent(p.logger).Warn("Proxy failed to write to daemon").WithError(err).Log()
				p.mu.Lock()
				if p.conn == conn {
					p.conn.Close()
					p.conn = nil
					p.daemonAlive.Store(false)
				}
				p.mu.Unlock()
				continue // retry!
			}
			return // success
		}

		time.Sleep(100 * time.Millisecond) // wait for connection
	}
}

func (p *ProxyDaemon) heartbeatLoop(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.mu.Lock()
			conn := p.conn
			format := p.initFormat
			if format == nil {
				format = &MessageFormat{IsRawJSON: true}
			}
			p.mu.Unlock()

			if conn != nil {
				pingMsg := `{"jsonrpc":"2.0","id":"__proxy_heartbeat__","method":"ping"}`
				writer := bufio.NewWriter(conn)
				p.heartbeatsSentTotal.Add(1)
				if err := p.transport.WriteMessage(writer, []byte(pingMsg), format); err != nil {
					p.heartbeatFailuresTotal.Add(1)
					logging.Fluent(p.logger).Warn("Proxy heartbeat failed").WithError(err).Log()
					p.mu.Lock()
					if p.conn == conn {
						p.conn.Close()
						p.conn = nil
						p.daemonAlive.Store(false)
					}
					p.mu.Unlock()
				}
			}
		}
	}
}

// IsDaemonAlive returns whether the proxy daemon currently has an active connection to the MCP daemon.
func (p *ProxyDaemon) IsDaemonAlive() bool {
	return p.daemonAlive.Load()
}

// GetTCPAddr returns the target TCP address for the daemon.
func (p *ProxyDaemon) GetTCPAddr() string {
	return p.tcpAddr
}

// ProbeDaemon performs a live active TCP dial check against the daemon address.
// Returns true if a TCP connection can be established within the specified timeout.
func (p *ProxyDaemon) ProbeDaemon(timeout time.Duration) bool {
	conn, err := net.DialTimeout("tcp", p.tcpAddr, timeout)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// ProbeDaemonAndSync performs a live active TCP dial check against the daemon address
// and updates the internal daemonAlive status accordingly.
func (p *ProxyDaemon) ProbeDaemonAndSync(timeout time.Duration) bool {
	alive := p.ProbeDaemon(timeout)
	p.daemonAlive.Store(alive)
	return alive
}
