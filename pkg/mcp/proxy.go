package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
)

// ProxyDaemon acts as a middleman between stdio (IDE) and a TCP MCP daemon.
// It detects disconnects via a heartbeat and transparently reconnects and re-initializes.
type ProxyDaemon struct {
	tcpAddr string
	logger  logging.Logger
	stdin   io.Reader
	stdout  io.Writer
	// Separate transports: DefaultTransport caches a json.Decoder per reader.
	// Sharing one between stdin and TCP caused concurrent Decode panics (IDE MCP hang).
	stdioTransport  Transport
	daemonTransport Transport

	mu          sync.Mutex
	conn        net.Conn
	daemonAlive atomic.Bool
	// ideSubscribed is set after we inject events/subscribe on the daemon conn.
	// Reset when the TCP conn drops so reconnect re-registers (CRIT-COMMS-003 / W3).
	ideSubscribed atomic.Bool

	heartbeatsSentTotal    atomic.Int64
	heartbeatFailuresTotal atomic.Int64
	reconnectionsTotal     atomic.Int64

	initMsg    []byte
	initFormat *MessageFormat
}

// Reserved JSON-RPC ids for proxy→daemon control plane (never forward responses to IDE).
const (
	proxyHeartbeatID       = `"__proxy_heartbeat__"`
	proxyEventsSubscribeID = `"__proxy_events_subscribe__"`
)

// IDEProxySubscriberClientID identifies the IDE/IDE seat in events/list
// (proxy and ide-adapter under-cover daemon sessions).
const IDEProxySubscriberClientID = "ide-ide-proxy"

// feedSteerProbeClientID is used by feed doctor/steer events/list probes.
// Must stay in isHumanClient allowlist so initialize skips credential elicitation.
const feedSteerProbeClientID = "zqk-feed-steer"

// Deprecated: use IDEProxySubscriberClientID.
const ideProxySubscriberClientID = IDEProxySubscriberClientID

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
		tcpAddr:         tcpAddr,
		logger:          logger,
		stdin:           os.Stdin,
		stdout:          os.Stdout,
		stdioTransport:  NewDefaultTransport(),
		daemonTransport: NewDefaultTransport(),
	}
}

func (p *ProxyDaemon) ensureTransports() {
	if p.stdioTransport == nil {
		p.stdioTransport = NewDefaultTransport()
	}
	if p.daemonTransport == nil {
		p.daemonTransport = NewDefaultTransport()
	}
}

// Start begins the proxy loops
func (p *ProxyDaemon) Start(ctx context.Context) error {
	p.ensureTransports()
	reader := bufio.NewReader(p.stdin)

	goroutinelabels.NewGoroutine("mcp_proxy", "daemon connection loop").
		StartSimple(func() { p.connectionLoop(ctx) })
	goroutinelabels.NewGoroutine("mcp_proxy", "daemon heartbeat loop").
		StartSimple(func() { p.heartbeatLoop(ctx) })

	for {
		msg, format, err := p.stdioTransport.ReadMessage(reader)
		if err != nil {
			if errors.Is(err, io.EOF) || strings.Contains(err.Error(), "EOF") {
				logging.Fluent(p.logger).Info("Client disconnected (EOF), exiting proxy cleanly").Log()
				return nil
			}
			return errfmt.Errorf("failed to read from stdio: %w", err)
		}

		// Try to parse as JSON-RPC to sniff initialize / initialized
		var rpc struct {
			Method string `json:"method"`
		}
		if err := json.Unmarshal(msg, &rpc); err == nil {
			switch rpc.Method {
			case "initialize":
				// IDE often sends a clientInfo.name the daemon does not treat as a
				// human IDE → initialize returns auth elicitation → UI green, zero tools.
				// Stamp a known human client name before forward/cache.
				msg = stampIDEInitializeClientInfo(msg)
				p.mu.Lock()
				p.initMsg = msg
				p.initFormat = format
				p.mu.Unlock()
				logging.Fluent(p.logger).Debug("Proxy intercepted initialize message").Log()
				p.forwardToDaemon(ctx, msg, format)
				// Stock IDE never calls events/subscribe; register after initialize
				// lands (auth-error init still accepts subscribe — enable-all path).
				goroutinelabels.NewGoroutine("mcp_proxy", "ensure IDE events/subscribe after initialize").
					StartSimple(func() {
						time.Sleep(50 * time.Millisecond)
						p.ensureIDEEventSubscription()
					})
				continue
			case "notifications/initialized":
				p.forwardToDaemon(ctx, msg, format)
				p.ensureIDEEventSubscription()
				continue
			}
		}

		p.forwardToDaemon(ctx, msg, format)
	}
}

func (p *ProxyDaemon) connectionLoop(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.mu.Lock()
			conn := p.conn
			p.mu.Unlock()

			if conn == nil {
				_ = p.tryConnectOnce(true) // replay cached initialize after daemon restart
			}
		}
	}
}

func (p *ProxyDaemon) readFromDaemon(conn net.Conn) {
	p.ensureTransports()
	// Fresh transport per TCP connection so decoder state never spans reconnects
	// or races with the stdio reader.
	daemonXport := NewDefaultTransport()
	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(p.stdout)

	for {
		msg, format, err := daemonXport.ReadMessage(reader)
		if err != nil {
			logging.Fluent(p.logger).Warn("Proxy lost connection to daemon").WithError(err).Log()
			p.mu.Lock()
			if p.conn == conn {
				_ = p.conn.Close()
				p.conn = nil
				p.daemonAlive.Store(false)
				p.ideSubscribed.Store(false)
			}
			p.mu.Unlock()
			return
		}

		// Sniff for proxy control-plane responses (never forward to IDE)
		var rpc struct {
			ID json.RawMessage `json:"id"`
		}
		if err := json.Unmarshal(msg, &rpc); err == nil {
			switch string(rpc.ID) {
			case proxyHeartbeatID:
				p.daemonAlive.Store(true)
				continue
			case proxyEventsSubscribeID:
				logging.Fluent(p.logger).Debug("Proxy IDE events/subscribe acknowledged").Log()
				continue
			}
		}

		p.mu.Lock()
		_ = p.stdioTransport.WriteMessage(writer, msg, format)
		p.mu.Unlock()
	}
}

// forwardDialTimeout bounds retries when the daemon was up and then dropped
// (brief restart). First-ever connect uses a single dial and fails immediately
// on refusal — IDE otherwise sits on "Connect"/mcp_auth far longer than 3s
// because it retries discovery after each slow failure.
const forwardDialTimeout = 1500 * time.Millisecond

func (p *ProxyDaemon) forwardToDaemon(ctx context.Context, msg []byte, format *MessageFormat) {
	p.ensureTransports()
	deadline := time.Now().Add(forwardDialTimeout)
	sawRefused := false
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		p.mu.Lock()
		conn := p.conn
		everAlive := p.daemonAlive.Load() || p.reconnectionsTotal.Load() > 0
		p.mu.Unlock()

		if conn == nil {
			// Opportunistic dial so initialize is not stuck behind connectionLoop's 1s sleep.
			// Do not replay init here — caller is about to write the current message.
			err := p.tryConnectOnce(false)
			if err != nil {
				if isConnRefused(err) {
					sawRefused = true
				}
				// Nothing listening yet (and never has been): fail this request now.
				// IDE's "Connect" UI waits on our stdio reply; spinning to deadline
				// makes that feel like a multi‑second hang even when the root cause
				// is simply "daemon not started".
				if !everAlive && sawRefused {
					logging.Fluent(p.logger).Warn("MCP daemon not listening; failing IDE request").
						Addr(p.tcpAddr).WithError(err).Log()
					p.replyDaemonUnavailable(msg, format)
					return
				}
			}
			if time.Now().After(deadline) {
				logging.Fluent(p.logger).Warn("MCP daemon unavailable; failing IDE request").
					Addr(p.tcpAddr).String("waited", forwardDialTimeout.String()).Log()
				p.replyDaemonUnavailable(msg, format)
				return
			}
			time.Sleep(50 * time.Millisecond)
			continue
		}

		writer := bufio.NewWriter(conn)
		if err := p.daemonTransport.WriteMessage(writer, msg, format); err != nil {
			logging.Fluent(p.logger).Warn("Proxy failed to write to daemon").WithError(err).Log()
			p.mu.Lock()
			if p.conn == conn {
				_ = p.conn.Close()
				p.conn = nil
				p.daemonAlive.Store(false)
			}
			p.mu.Unlock()
			if time.Now().After(deadline) {
				p.replyDaemonUnavailable(msg, format)
				return
			}
			continue
		}
		return
	}
}

func isConnRefused(err error) bool {
	if err == nil {
		return false
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		var sysErr *os.SyscallError
		if errors.As(opErr.Err, &sysErr) {
			return errors.Is(sysErr.Err, syscall.ECONNREFUSED)
		}
		return errors.Is(opErr.Err, syscall.ECONNREFUSED)
	}
	return strings.Contains(strings.ToLower(err.Error()), "connection refused")
}

func (p *ProxyDaemon) tryConnectOnce(replayInit bool) error {
	p.mu.Lock()
	if p.conn != nil {
		p.mu.Unlock()
		return nil
	}
	p.mu.Unlock()

	newConn, err := net.DialTimeout("tcp", p.tcpAddr, 250*time.Millisecond)
	if err != nil {
		return err
	}
	logging.Fluent(p.logger).Info("Proxy connected to daemon").Addr(p.tcpAddr).Log()

	p.mu.Lock()
	if p.conn != nil {
		_ = newConn.Close()
		p.mu.Unlock()
		return nil
	}
	p.conn = newConn
	p.daemonAlive.Store(true)
	p.reconnectionsTotal.Add(1)
	initMsg := p.initMsg
	initFormat := p.initFormat
	p.mu.Unlock()

	if replayInit && len(initMsg) > 0 {
		logging.Fluent(p.logger).Info("Proxy re-sending initialize message").Log()
		_ = p.daemonTransport.WriteMessage(bufio.NewWriter(newConn), initMsg, initFormat)
		// IDE will not re-send notifications/initialized after daemon restart; re-subscribe.
		p.ideSubscribed.Store(false)
		goroutinelabels.NewGoroutine("mcp_proxy", "ensure IDE events/subscribe after reconnect init").
			StartSimple(func() {
				// Allow initialize to land before subscribe (same TCP conn).
				time.Sleep(50 * time.Millisecond)
				p.ensureIDEEventSubscription()
			})
	}
	goroutinelabels.NewGoroutine("mcp_proxy", "read from daemon connection").
		StartSimple(func() { p.readFromDaemon(newConn) })
	return nil
}

// StampIDEInitializeClientInfo ensures initialize params use a human IDE clientInfo.name
// so handleAuthenticationFlow skips username/password elicitation for IDE seats.
func StampIDEInitializeClientInfo(msg []byte) []byte {
	return stampIDEInitializeClientInfo(msg)
}

// stampIDEInitializeClientInfo ensures initialize params use a human IDE clientInfo.name
// so handleAuthenticationFlow skips username/password elicitation for IDE proxy seats.
func stampIDEInitializeClientInfo(msg []byte) []byte {
	var envelope map[string]any
	if err := json.Unmarshal(msg, &envelope); err != nil {
		return msg
	}
	params, _ := envelope["params"].(map[string]any)
	if params == nil {
		params = map[string]any{}
		envelope["params"] = params
	}
	ci, _ := params["clientInfo"].(map[string]any)
	if ci == nil {
		ci = map[string]any{}
		params["clientInfo"] = ci
	}
	name, _ := ci[objects.FieldKeyName].(string)
	// Only the clientInfo name counts here — do not pass proxy id as clientID
	// (isHumanClient would always match "ide-ide-proxy" and skip the stamp).
	if isHumanClient(name, "") {
		return msg
	}
	if name != "" {
		ci["originalClientName"] = name
	}
	ci[objects.FieldKeyName] = ideProxySubscriberClientID
	if _, ok := ci[objects.FieldKeyVersion].(string); !ok {
		ci[objects.FieldKeyVersion] = "proxy"
	}
	out, err := json.Marshal(envelope)
	if err != nil {
		return msg
	}
	return out
}

// ensureIDEEventSubscription registers action.required on the long-lived proxy→daemon
// connection so stock IDE (no events/subscribe) still counts as a live subscriber.
func (p *ProxyDaemon) ensureIDEEventSubscription() {
	if p == nil || p.ideSubscribed.Load() {
		return
	}
	p.ensureTransports()
	p.mu.Lock()
	conn := p.conn
	format := p.initFormat
	if format == nil {
		format = &MessageFormat{IsRawJSON: true}
	}
	p.mu.Unlock()
	if conn == nil {
		return
	}
	// Keep id as a JSON string matching proxyEventsSubscribeID (quoted form).
	b := []byte(`{"jsonrpc":"2.0","id":` + proxyEventsSubscribeID + `,"method":"events/subscribe","params":{"eventTypes":["` + string(EventTypeActionRequired) + `"],"clientId":"` + ideProxySubscriberClientID + `"}}`)
	writer := bufio.NewWriter(conn)
	if err := p.daemonTransport.WriteMessage(writer, b, format); err != nil {
		logging.Fluent(p.logger).Warn("Proxy failed to send IDE events/subscribe").WithError(err).Log()
		return
	}
	p.ideSubscribed.Store(true)
	logging.Fluent(p.logger).Info("Proxy subscribed IDE seat to action.required").
		String("client_id", ideProxySubscriberClientID).Log()
}

func (p *ProxyDaemon) replyDaemonUnavailable(msg []byte, format *MessageFormat) {
	var req struct {
		ID     json.RawMessage `json:"id"`
		Method string          `json:"method"`
	}
	if err := json.Unmarshal(msg, &req); err != nil || len(req.ID) == 0 || string(req.ID) == "null" {
		return
	}
	payload := struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Error   map[string]any  `json:"error"`
	}{
		JSONRPC: "2.0",
		ID:      req.ID,
		Error: map[string]any{
			objects.FieldKeyCode: -32001,
			"message":            "MCP daemon unavailable at " + p.tcpAddr + paths.RewriteCanonicalCLIInvocations("; run: zqk mcp daemon --tcp ") + p.tcpAddr,
		},
	}
	resp, err := json.Marshal(payload)
	if err != nil {
		return
	}
	if format == nil {
		format = &MessageFormat{IsRawJSON: true}
	}
	p.mu.Lock()
	_ = p.stdioTransport.WriteMessage(bufio.NewWriter(p.stdout), resp, format)
	p.mu.Unlock()
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
				pingMsg := `{"jsonrpc":"2.0","id":` + proxyHeartbeatID + `,"method":"ping"}`
				writer := bufio.NewWriter(conn)
				p.heartbeatsSentTotal.Add(1)
				if err := p.daemonTransport.WriteMessage(writer, []byte(pingMsg), format); err != nil {
					p.heartbeatFailuresTotal.Add(1)
					logging.Fluent(p.logger).Warn("Proxy heartbeat failed").WithError(err).Log()
					p.mu.Lock()
					if p.conn == conn {
						_ = p.conn.Close()
						p.conn = nil
						p.daemonAlive.Store(false)
						p.ideSubscribed.Store(false)
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

// PublishDaemonEvent sends an action.required notification to the MCP daemon over TCP IPC.
func (p *ProxyDaemon) PublishDaemonEvent(ctx context.Context, message, agentID, eventID string) error {
	dialer := net.Dialer{Timeout: 500 * time.Millisecond}
	conn, err := dialer.DialContext(ctx, "tcp", p.tcpAddr)
	if err != nil {
		return errfmt.Newf("publish daemon event dial").Wrap(err)
	}
	defer conn.Close()

	payload := map[string]any{
		"jsonrpc":              "2.0",
		objects.FieldKeyMethod: "notifications/event",
		"params": map[string]any{
			objects.FieldKeyType:     "action.required",
			"timestamp":              time.Now().UTC().Format(time.RFC3339),
			"message":                message,
			objects.FieldKeySeverity: "info",
			objects.FieldKeyPriority: "high",
			"fields": map[string]any{
				objects.FieldKeyAgentID: agentID,
				"event_id":              eventID,
			},
		},
	}
	b, mErr := json.Marshal(payload)
	if mErr != nil {
		return errfmt.Newf("marshal notification").Wrap(mErr)
	}
	b = append(b, '\n')
	writer := bufio.NewWriter(conn)
	p.ensureTransports()
	if err := p.daemonTransport.WriteMessage(writer, b, &MessageFormat{IsRawJSON: true}); err != nil {
		return errfmt.Newf("write notification").Wrap(err)
	}
	return nil
}

func (p *ProxyDaemon) queryRPC(ctx context.Context, opName, method string, params map[string]any) (map[string]any, error) {
	if p == nil {
		return nil, errfmt.Errorf("nil proxy daemon")
	}
	dialer := net.Dialer{Timeout: 500 * time.Millisecond}
	conn, err := dialer.DialContext(ctx, "tcp", p.tcpAddr)
	if err != nil {
		return nil, errfmt.Newf("%s dial", opName).Wrap(err)
	}
	defer conn.Close()

	deadline := time.Now().Add(1500 * time.Millisecond)
	if dl, ok := ctx.Deadline(); ok && dl.Before(deadline) {
		deadline = dl
	}
	_ = conn.SetDeadline(deadline)

	p.ensureTransports()
	writer := bufio.NewWriter(conn)
	br := bufio.NewReader(conn)
	writeRPC := func(id int, m string, prms map[string]any) error {
		payload := map[string]any{
			"jsonrpc":              "2.0",
			objects.FieldKeyID:     id,
			objects.FieldKeyMethod: m,
			"params":               prms,
		}
		b, mErr := json.Marshal(payload)
		if mErr != nil {
			return errfmt.Newf("marshal %s", m).Wrap(mErr)
		}
		b = append(b, '\n')
		if wErr := p.daemonTransport.WriteMessage(writer, b, &MessageFormat{IsRawJSON: true}); wErr != nil {
			return errfmt.Newf("write %s", m).Wrap(wErr)
		}
		return writer.Flush()
	}

	// Use the feed-steer probe client so initialize skips credential elicitation
	// (ide-ide-proxy elicitation hangs 1s probes → mcp_query_failed / stamp_not_live).
	if err := writeRPC(1, "initialize", map[string]any{
		"protocolVersion":            "2024-11-05",
		objects.FieldKeyCapabilities: map[string]any{},
		"clientInfo":                 map[string]any{objects.FieldKeyName: feedSteerProbeClientID, objects.FieldKeyVersion: "dev"},
	}); err != nil {
		return nil, err
	}
	if _, rErr := br.ReadBytes('\n'); rErr != nil {
		return nil, errfmt.Newf("read initialize response").Wrap(rErr)
	}

	if err := writeRPC(2, method, params); err != nil {
		return nil, err
	}
	line, rErr := br.ReadBytes('\n')
	if rErr != nil {
		return nil, errfmt.Newf("read %s response", method).Wrap(rErr)
	}
	var resp struct {
		Result map[string]any `json:"result"`
		Error  *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if uErr := json.Unmarshal(line, &resp); uErr != nil {
		return nil, errfmt.Newf("decode %s response", method).Wrap(uErr)
	}
	if resp.Error != nil {
		return nil, errfmt.Errorf("%s error: %s", method, resp.Error.Message)
	}
	return resp.Result, nil
}

// QueryEventsSubscriberCount dials the daemon TCP port and invokes events/list to read subscriberCount.
// Returns an error if the daemon is unreachable or returns an error.
func (p *ProxyDaemon) QueryEventsSubscriberCount(ctx context.Context) (int, error) {
	res, err := p.queryRPC(ctx, "events/list", "events/list", map[string]any{})
	if err != nil {
		return 0, err
	}
	raw, ok := res["subscriberCount"]
	if !ok {
		return 0, errfmt.Errorf("events/list missing subscriberCount")
	}
	switch n := raw.(type) {
	case float64:
		return int(n), nil
	case int:
		return n, nil
	case json.Number:
		i, cErr := n.Int64()
		if cErr != nil {
			return 0, errfmt.Newf("subscriberCount number").Wrap(cErr)
		}
		return int(i), nil
	default:
		return 0, errfmt.Errorf("subscriberCount unexpected type %T", raw)
	}
}

// QueryDiagnostics calls system/diagnostics on the MCP daemon and returns the payload.
// Used by supervise to extract telemetry and spec counts for operators without
// coupling directly to kernel_storage objects.
func (p *ProxyDaemon) QueryDiagnostics(ctx context.Context) (map[string]any, error) {
	return p.queryRPC(ctx, "diagnostics", "system/diagnostics", map[string]any{})
}
