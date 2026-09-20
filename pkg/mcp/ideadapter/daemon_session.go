package ideadapter

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/mcp"
	"github.com/zqk-os/zqk/pkg/objects"
)

// daemonSession is a private JSON-RPC session to the MCP daemon.
// Request IDs are owned entirely by this session; IDE never sees them.
type daemonSession struct {
	cfg    Config
	logger logging.Logger

	mu      sync.Mutex
	conn    net.Conn
	writer  *bufio.Writer
	reader  *bufio.Reader
	xport   mcp.Transport
	writeMu sync.Mutex

	nextID    atomic.Int64
	pending   map[int64]chan json.RawMessage
	pendingMu sync.Mutex

	alive       atomic.Bool
	subscribed  atomic.Bool
	initialized atomic.Bool
	closing     atomic.Bool

	notifyCh     chan []byte
	daemonFormat *mcp.MessageFormat

	// lifeCtx bounds all daemon dial/RPC work. Canceled only by Close() so IDE
	// tearing down cmd.Context mid-tools/call cannot abort an in-flight daemon RPC.
	// Hard cap remains RequestTimeout per call. TRACK: BLI-1784969955962654000-dc689643 —
	// prefer hourglass/context-refresh deadlines when shutdown polish lands.
	lifeCtx    context.Context
	lifeCancel context.CancelFunc

	// Consecutive ping failures before closeConn. TRACK: BLI-CEF-R2-REL-MCP-RECONNECT
	heartbeatFails atomic.Int32
}

func newDaemonSession(cfg Config, logger logging.Logger) *daemonSession {
	lifeCtx, lifeCancel := context.WithCancel(context.Background()) // Background: request-or-shutdown derived
	return &daemonSession{
		cfg:          cfg,
		logger:       logger,
		xport:        mcp.NewDefaultTransport(),
		pending:      make(map[int64]chan json.RawMessage),
		notifyCh:     make(chan []byte, 64),
		daemonFormat: &mcp.MessageFormat{IsRawJSON: true},
		lifeCtx:      lifeCtx,
		lifeCancel:   lifeCancel,
	}
}

// rpcCtx returns a per-call context: session lifetime ∩ RequestTimeout.
// Does not inherit IDE/cmd cancel — that was aborting tools/call in ~1ms.
func (s *daemonSession) rpcCtx() (context.Context, context.CancelFunc) {
	timeout := s.cfg.RequestTimeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return context.WithTimeout(s.lifeCtx, timeout)
}

func (s *daemonSession) Notifications() <-chan []byte {
	return s.notifyCh
}

func (s *daemonSession) Alive() bool {
	return s.alive.Load()
}

// EnsureConnected dials (if needed), initializes, and subscribes.
// The incoming ctx is ignored for dial/RPC deadlines (see rpcCtx); pass nil-safe.
func (s *daemonSession) EnsureConnected(_ context.Context) error {
	s.mu.Lock()
	if s.conn != nil && s.alive.Load() && s.initialized.Load() {
		s.mu.Unlock()
		s.ensureSubscribe()
		return nil
	}
	s.mu.Unlock()
	return s.reconnect()
}

func (s *daemonSession) reconnect() error {
	s.closeConn()

	ctx, cancel := s.rpcCtx()
	defer cancel()

	diagf("daemon dial begin tcp=%s", s.cfg.DaemonTCP)
	dialer := net.Dialer{Timeout: 2 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", s.cfg.DaemonTCP)
	if err != nil {
		diagf("daemon dial FAIL tcp=%s err=%v", s.cfg.DaemonTCP, err)
		return errfmt.Newf("ide-adapter dial daemon %s", s.cfg.DaemonTCP).Wrap(err)
	}

	s.mu.Lock()
	s.conn = conn
	s.reader = bufio.NewReader(conn)
	s.writer = bufio.NewWriter(conn)
	s.xport = mcp.NewDefaultTransport()
	s.closing.Store(false)
	s.alive.Store(true)
	s.initialized.Store(false)
	s.subscribed.Store(false)
	s.mu.Unlock()

	goroutinelabels.NewGoroutine("mcp_ide_adapter", "daemon session read loop").
		StartSimple(func() { s.readLoop() })

	if err := s.initialize(ctx); err != nil {
		diagf("daemon initialize FAIL err=%v", err)
		s.closeConn()
		return err
	}
	diagf("daemon initialize ok")
	s.ensureSubscribe()
	return nil
}

func (s *daemonSession) closeConn() {
	s.closing.Store(true)
	s.mu.Lock()
	conn := s.conn
	had := conn != nil
	s.conn = nil
	s.reader = nil
	s.writer = nil
	s.alive.Store(false)
	s.initialized.Store(false)
	s.subscribed.Store(false)
	s.mu.Unlock()
	if conn != nil {
		_ = conn.Close()
	}
	if had {
		diagf("daemon session closed (failPending)")
	}
	s.failPending()
}

func (s *daemonSession) failPending() {
	s.pendingMu.Lock()
	defer s.pendingMu.Unlock()
	for id, ch := range s.pending {
		close(ch)
		delete(s.pending, id)
	}
}

func (s *daemonSession) initialize(ctx context.Context) error {
	params := map[string]any{
		wireProtocolVersion:          defaultProtocolVersion,
		objects.FieldKeyCapabilities: map[string]any{},
		wireClientInfo: map[string]any{
			objects.FieldKeyName:    s.cfg.ClientInfoName,
			objects.FieldKeyVersion: s.cfg.ClientInfoVersion,
		},
	}
	raw, err := s.call(ctx, methodInitialize, params)
	if err != nil {
		return errfmt.Newf("daemon initialize").Wrap(err)
	}
	var resp struct {
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return errfmt.Newf("parse initialize response").Wrap(err)
	}
	if resp.Error != nil {
		return errfmt.Errorf("daemon initialize error: %s", resp.Error.Message)
	}
	_ = s.notify(ctx, methodInitializedNotification, map[string]any{})
	s.initialized.Store(true)
	return nil
}

func (s *daemonSession) ensureSubscribe() {
	if s.subscribed.Load() || !s.alive.Load() {
		return
	}
	types := s.cfg.AutoSubscribeEventTypes
	if len(types) == 0 {
		types = []string{string(mcp.EventTypeActionRequired)}
	}
	params := map[string]any{
		wireEventTypes: types,
		wireClientID:   s.cfg.ClientInfoName,
	}
	ctx, cancel := s.rpcCtx()
	defer cancel()
	if _, err := s.call(ctx, methodEventsSubscribe, params); err != nil {
		diagf("events/subscribe FAIL err=%v", err)
		logging.Fluent(s.logger).Warn("ide-adapter events/subscribe failed").WithError(err).Log()
		return
	}
	s.subscribed.Store(true)
	diagf("events/subscribe ok client_id=%s", s.cfg.ClientInfoName)
	logging.Fluent(s.logger).Info("ide-adapter subscribed daemon seat").
		String("client_id", s.cfg.ClientInfoName).Log()
}

// Call forwards a JSON-RPC method to the daemon and returns the full response envelope
// with the daemon id still present (caller rewrites id for IDE).
// Parent IDE/cmd context is ignored for the wait; control is RequestTimeout + Close().
func (s *daemonSession) Call(_ context.Context, method string, params any) (json.RawMessage, error) {
	if err := s.EnsureConnected(nil); err != nil {
		return nil, err
	}
	ctx, cancel := s.rpcCtx()
	defer cancel()
	raw, err := s.call(ctx, method, params)
	if err != nil {
		diagf("daemon Call retry after err method=%s err=%v", method, err)
		if rerr := s.reconnect(); rerr != nil {
			return nil, err
		}
		ctx2, cancel2 := s.rpcCtx()
		defer cancel2()
		return s.call(ctx2, method, params)
	}
	return raw, nil
}

func (s *daemonSession) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	id := s.nextID.Add(1)
	payload := map[string]any{
		rpcKeyJSONRPC:      mcp.JSONRPCVersion,
		objects.FieldKeyID: id,
		rpcKeyMethod:       method,
	}
	if params != nil {
		payload[rpcKeyParams] = params
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, errfmt.Newf("marshal %s", method).Wrap(err)
	}

	ch := make(chan json.RawMessage, 1)
	s.pendingMu.Lock()
	s.pending[id] = ch
	s.pendingMu.Unlock()

	if err := s.writeDaemon(body); err != nil {
		s.pendingMu.Lock()
		delete(s.pending, id)
		s.pendingMu.Unlock()
		s.closeConn()
		return nil, errfmt.Newf("write %s", method).Wrap(err)
	}

	timeout := s.cfg.RequestTimeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	// ctx is already rpcCtx (life ∩ timeout). Wait on ctx.Done for shutdown/timeout;
	// do not also race a second timer that could desync from dial deadline.
	select {
	case <-ctx.Done():
		s.pendingMu.Lock()
		delete(s.pending, id)
		s.pendingMu.Unlock()
		err := ctx.Err()
		if err == context.DeadlineExceeded {
			diagf("daemon call TIMEOUT method=%s id=%d timeout=%s", method, id, timeout)
			return nil, errfmt.Errorf("daemon %s timed out", method)
		}
		diagf("daemon call session shutdown method=%s id=%d err=%v", method, id, err)
		return nil, err
	case raw, ok := <-ch:
		if !ok {
			diagf("daemon call closed method=%s id=%d", method, id)
			return nil, errfmt.Errorf("daemon connection closed during %s", method)
		}
		diagf("daemon call ok method=%s id=%d bytes=%d", method, id, len(raw))
		return raw, nil
	}
}

func (s *daemonSession) notify(ctx context.Context, method string, params any) error {
	_ = ctx
	payload := map[string]any{
		rpcKeyJSONRPC: mcp.JSONRPCVersion,
		rpcKeyMethod:  method,
	}
	if params != nil {
		payload[rpcKeyParams] = params
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return s.writeDaemon(body)
}

func (s *daemonSession) writeDaemon(body []byte) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	s.mu.Lock()
	w, xport, format := s.writer, s.xport, s.daemonFormat
	s.mu.Unlock()
	if w == nil || xport == nil {
		return errfmt.Errorf("daemon session not connected")
	}
	return xport.WriteMessage(w, body, format)
}

func (s *daemonSession) readLoop() {
	for {
		s.mu.Lock()
		r, xport, conn := s.reader, s.xport, s.conn
		s.mu.Unlock()
		if r == nil || conn == nil {
			return
		}
		msg, _, err := xport.ReadMessage(r)
		if err != nil {
			if !errors.Is(err, io.EOF) && !s.closing.Load() && !isExpectedDaemonDisconnect(err) {
				diagf("daemon readLoop error: %v", err)
				logging.Fluent(s.logger).Warn("ide-adapter daemon read error").WithError(err).Log()
			} else {
				diagf("daemon readLoop EOF")
			}
			s.closeConn()
			return
		}

		var envelope struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Error  any             `json:"error"`
		}
		if err := json.Unmarshal(msg, &envelope); err != nil {
			diagf("daemon readLoop unmarshal skip err=%v", err)
			continue
		}

		if len(envelope.ID) == 0 || string(envelope.ID) == "null" {
			if envelope.Method != "" {
				select {
				case s.notifyCh <- append([]byte(nil), msg...):
					diagf("daemon notify queued method=%s bytes=%d", envelope.Method, len(msg))
				default:
					diagf("daemon notify DROP method=%s (buffer full)", envelope.Method)
					logging.Fluent(s.logger).Warn("ide-adapter notify drop (buffer full)").
						String(rpcKeyMethod, envelope.Method).Log()
				}
			}
			continue
		}

		var idNum float64
		if err := json.Unmarshal(envelope.ID, &idNum); err != nil {
			diagf("daemon readLoop non-numeric id=%s", string(envelope.ID))
			continue
		}
		id := int64(idNum)

		s.pendingMu.Lock()
		ch, ok := s.pending[id]
		if ok {
			delete(s.pending, id)
		}
		pendingN := len(s.pending)
		s.pendingMu.Unlock()
		if ok {
			ch <- append(json.RawMessage(nil), msg...)
			close(ch)
			continue
		}
		diagf("daemon readLoop UNMATCHED response id=%d method=%q pending=%d bytes=%d", id, envelope.Method, pendingN, len(msg))
	}
}

func (s *daemonSession) pendingCount() int {
	s.pendingMu.Lock()
	defer s.pendingMu.Unlock()
	return len(s.pending)
}

// HeartbeatLoop pings the daemon; on failure clears the connection for reconnect.
// Never closeConn while another RPC is pending — that aborted tools/call when the
// daemon was wedged and ping timed out first (AGY-2 ServeLoop leak). Skip ping
// entirely while work is in flight. TRACK: BLI-1784969955962654000-dc689643.
func (s *daemonSession) HeartbeatLoop(ctx context.Context) {
	interval := s.cfg.HeartbeatInterval
	if interval <= 0 {
		interval = 5 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if !s.alive.Load() {
				_ = s.EnsureConnected(nil)
				continue
			}
			if s.pendingCount() > 0 {
				continue
			}
			pingCtx, cancel := context.WithTimeout(s.lifeCtx, heartbeatPingTimeout)
			_, err := s.call(pingCtx, methodPing, map[string]any{})
			cancel()
			if err != nil {
				if s.lifeCtx.Err() != nil {
					return
				}
				if s.pendingCount() > 0 {
					diagf("heartbeat FAIL with pending RPC; skip closeConn err=%v", err)
					continue
				}
				fails := int(s.heartbeatFails.Add(1))
				logging.Fluent(s.logger).Warn("ide-adapter heartbeat failed").
					WithError(err).
					Int("consecutive_fails", fails).
					Log()
				if !heartbeatDropAfterConsecutiveFails(fails) {
					diagf("heartbeat FAIL %d/%d skip closeConn err=%v", fails, heartbeatFailBeforeDrop, err)
					continue
				}
				s.heartbeatFails.Store(0)
				diagf("heartbeat FAIL threshold; closeConn err=%v", err)
				s.closeConn()
				continue
			}
			s.heartbeatFails.Store(0)
		}
	}
}

func isExpectedDaemonDisconnect(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "use of closed network connection") ||
		strings.Contains(msg, "broken pipe") ||
		strings.Contains(msg, "connection reset")
}

// Close tears down the daemon session and cancels in-flight RPC waiters (bounded).
func (s *daemonSession) Close() {
	if s.lifeCancel != nil {
		s.lifeCancel()
	}
	s.closeConn()
}
