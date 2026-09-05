package ideadapter

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/mcp"
	"github.com/lanceman/zqk/pkg/objects"
)

// Adapter is the IDE-facing MCP peer. It speaks a clean full MCP contract on
// stdio while privately maintaining a daemonSession for tools/events.
type Adapter struct {
	cfg     Config
	logger  logging.Logger
	stdin   io.Reader
	stdout  io.Writer
	session *daemonSession

	stdioXport mcp.Transport
	ideFmt     *mcp.MessageFormat
	queue      *mcp.MessageQueue
	queueMu    sync.RWMutex
}

// New creates a IDE adapter with defaults for missing config fields.
func New(cfg Config, logger logging.Logger) *Adapter {
	merged := DefaultConfig()
	mergeConfig(&merged, cfg)
	if strings.TrimSpace(merged.DaemonTCP) == "" {
		merged.DaemonTCP = mcp.DefaultDaemonTCP
	}
	return &Adapter{
		cfg:        merged,
		logger:     logger,
		stdin:      os.Stdin,
		stdout:     os.Stdout,
		session:    newDaemonSession(merged, logger),
		stdioXport: mcp.NewDefaultTransport(),
	}
}

// WithStdio overrides stdin/stdout (tests).
func (a *Adapter) WithStdio(in io.Reader, out io.Writer) *Adapter {
	if in != nil {
		a.stdin = in
	}
	if out != nil {
		a.stdout = out
	}
	return a
}

// Run serves IDE on stdio until stdin EOF.
// ctx drives heartbeat/notify/keepalive only — IDE cancels cmd.Context after tools/call;
// exiting the process on that cancel marks MCP red. Hard stop: stdin EOF (IDE
// disconnect) or SIGTERM closing stdin (see ide_adapter cmd). TRACK:
// REDACTED — hourglass/context-refresh for soft drain.
func (a *Adapter) Run(ctx context.Context) error {
	diagf("run start daemon_tcp=%s request_timeout=%s heartbeat=%s stdio_keepalive=%s",
		a.cfg.DaemonTCP, a.cfg.RequestTimeout, a.cfg.HeartbeatInterval, a.cfg.StdioKeepaliveInterval)

	writer := bufio.NewWriter(a.stdout)
	a.queueMu.Lock()
	a.queue = mcp.NewMessageQueue(writer, &mcp.MessageFormat{IsRawJSON: true}, mcp.DefaultQueueConfig())
	a.queueMu.Unlock()
	defer func() {
		a.queueMu.RLock()
		if a.queue != nil {
			a.queue.Stop()
		}
		a.queueMu.RUnlock()
	}()

	goroutinelabels.NewGoroutine("mcp_ide_adapter", "forward daemon notifications to IDE").
		StartSimple(func() { a.forwardDaemonNotifications(ctx) })
	goroutinelabels.NewGoroutine("mcp_ide_adapter", "daemon heartbeat after first connect").
		StartSimple(func() {
			for !a.session.Alive() {
				select {
				case <-ctx.Done():
					return
				case <-time.After(100 * time.Millisecond):
				}
			}
			a.session.HeartbeatLoop(ctx)
		})
	goroutinelabels.NewGoroutine("mcp_ide_adapter", "ide stdio keepalive").
		StartSimple(func() { a.stdioKeepaliveLoop(ctx) })

	reader := bufio.NewReader(a.stdin)

	for {
		msg, format, err := a.stdioXport.ReadMessage(reader)
		if err != nil {
			a.session.Close()
			if errors.Is(err, io.EOF) || strings.Contains(err.Error(), "EOF") {
				diagf("ide stdio EOF (IDE disconnected)")
				logging.Fluent(a.logger).Info("ide-adapter client disconnected").Log()
				return nil
			}
			diagf("ide stdio read error: %v", err)
			return errfmt.Newf("ide-adapter read stdio").Wrap(err)
		}
		a.queueMu.Lock()
		a.ideFmt = format
		a.queueMu.Unlock()

		if err := a.handleIDEMessage(ctx, writer, msg, format); err != nil {
			diagf("handleIDEMessage error: %v", err)
			logging.Fluent(a.logger).Warn("ide-adapter handle error").WithError(err).Log()
		}
	}
}

func (a *Adapter) forwardDaemonNotifications(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-a.session.Notifications():
			if !ok {
				return
			}

			// IDE's strict MCP parser crashes on unknown or non-standard notifications.
			// Furthermore, if IDE didn't explicitly request 'logging' capabilities,
			// sending standard notifications/message will ALSO crash it.
			// Therefore, we must simply DROP any custom daemon events to prevent crashes.
			var envelope map[string]any
			if err := json.Unmarshal(msg, &envelope); err == nil {
				if method, _ := envelope["method"].(string); method != "" {
					// Drop ZQK custom notifications entirely
					if method == "notifications/event" || method == "notifications/message" || method == "notifications/progress" {
						continue
					}
				}
			}

			a.queueMu.RLock()
			format := a.ideFmt
			if format == nil {
				format = &mcp.MessageFormat{IsRawJSON: true}
			}
			if a.queue != nil {
				a.queue.Enqueue(msg, format, "normal", nil)
			}
			a.queueMu.RUnlock()
		}
	}
}

// stdioKeepaliveLoop was previously used to send keepalives, but IDE's strict MCP parser
// crashes on virtually all unsolicited server->client traffic (custom notifications, un-negotiated
// standard notifications, unmatched responses, and unhandled requests).
// Standard MCP servers do not send keepalives, and IDE does not drop them.
// Therefore, we disable this completely so we behave exactly like a normal MCP server.
func (a *Adapter) stdioKeepaliveLoop(ctx context.Context) {
}

func (a *Adapter) handleIDEMessage(ctx context.Context, writer *bufio.Writer, msg []byte, format *mcp.MessageFormat) error {
	var req struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Method  string          `json:"method"`
		Params  json.RawMessage `json:"params"`
	}
	if err := json.Unmarshal(msg, &req); err != nil {
		return a.writeError(writer, format, nil, mcp.ParseError, "Parse error")
	}

	// JSON-RPC response (result/error, no method) — ignore, we don't expect responses from IDE.
	if strings.TrimSpace(req.Method) == "" && len(req.ID) > 0 && string(req.ID) != "null" {
		return nil
	}

	if len(req.ID) == 0 || string(req.ID) == "null" {
		switch req.Method {
		case methodInitializedNotification:
			_ = a.session.EnsureConnected(ctx)
			a.session.ensureSubscribe()
		default:
		}
		return nil
	}

	switch req.Method {
	case methodInitialize:
		return a.handleInitialize(ctx, writer, format, req.ID, req.Params)
	case methodPing:
		// Client→server ping: answer immediately (also resets our view of liveness).
		return a.writeResult(writer, format, req.ID, map[string]any{})
	case methodToolsList, methodToolsCall, methodPromptsList, methodPromptsGet,
		methodResourcesList, methodResourcesRead, methodResourcesTemplatesList,
		methodCompletionComplete, methodLoggingSetLevel:
		return a.forwardRPC(ctx, writer, format, req.ID, req.Method, req.Params)
	default:
		if err := a.session.EnsureConnected(ctx); err != nil {
			return a.writeError(writer, format, req.ID, mcp.MethodNotFound, "Method not found: "+req.Method)
		}
		return a.forwardRPC(ctx, writer, format, req.ID, req.Method, req.Params)
	}
}

func (a *Adapter) handleInitialize(ctx context.Context, writer *bufio.Writer, format *mcp.MessageFormat, id json.RawMessage, params json.RawMessage) error {
	// Ensure daemon is up; failures still return a clean IDE-facing initialize
	// so the IDE stays green while reconnect/heartbeat recovers the seat.
	_ = a.session.EnsureConnected(ctx)

	caps := map[string]any{
		wireTools:     map[string]any{},
		wirePrompts:   map[string]any{},
		wireResources: map[string]any{},
		wireLogging: map[string]any{
			wireLevel: "info",
		},
		wireRoots: map[string]any{},
	}
	if a.cfg.AdvertiseElicitation {
		caps[wireElicitation] = map[string]any{
			objects.FieldKeyEnabled: true,
		}
	}

	result := map[string]any{
		wireProtocolVersion:          defaultProtocolVersion,
		objects.FieldKeyCapabilities: caps,
		wireServerInfo: map[string]any{
			objects.FieldKeyName:    a.cfg.ServerName,
			objects.FieldKeyVersion: a.cfg.ServerVersion,
		},
	}

	if params != nil {
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		_ = json.Unmarshal(params, &p)
		if p.ProtocolVersion != "" {
			result[wireProtocolVersion] = p.ProtocolVersion
		}
	}

	return a.writeResult(writer, format, id, result)
}

func (a *Adapter) forwardRPC(ctx context.Context, writer *bufio.Writer, format *mcp.MessageFormat, ideID json.RawMessage, method string, params json.RawMessage) error {
	var paramsVal any
	if len(params) > 0 && string(params) != "null" {
		if err := json.Unmarshal(params, &paramsVal); err != nil {
			return a.writeError(writer, format, ideID, mcp.InvalidParams, "Invalid params")
		}
	} else {
		paramsVal = map[string]any{}
	}

	start := time.Now()
	diagf("forwardRPC begin method=%s ide_id=%s", method, string(ideID))
	raw, err := a.session.Call(ctx, method, paramsVal)
	if err != nil {
		diagf("forwardRPC FAIL method=%s ide_id=%s dur=%s err=%v", method, string(ideID), time.Since(start), err)
		return a.writeError(writer, format, ideID, errDaemonUnavailable, "MCP daemon unavailable: "+err.Error())
	}

	var envelope map[string]any
	if err := json.Unmarshal(raw, &envelope); err != nil {
		diagf("forwardRPC bad daemon JSON method=%s err=%v", method, err)
		return a.writeError(writer, format, ideID, mcp.InternalError, "Invalid daemon response")
	}
	var ideIDVal any
	if err := json.Unmarshal(ideID, &ideIDVal); err != nil {
		ideIDVal = string(ideID)
	}
	envelope[objects.FieldKeyID] = ideIDVal
	envelope[rpcKeyJSONRPC] = mcp.JSONRPCVersion

	out, err := json.Marshal(envelope)
	if err != nil {
		return a.writeError(writer, format, ideID, mcp.InternalError, "Marshal response failed")
	}
	if werr := a.writeStdout(writer, out, format); werr != nil {
		diagf("forwardRPC stdout write FAIL method=%s err=%v", method, werr)
		return werr
	}
	diagf("forwardRPC ok method=%s ide_id=%s dur=%s bytes=%d", method, string(ideID), time.Since(start), len(out))
	return nil
}

func (a *Adapter) writeResult(writer *bufio.Writer, format *mcp.MessageFormat, id json.RawMessage, result any) error {
	var idVal any
	_ = json.Unmarshal(id, &idVal)
	payload := map[string]any{
		rpcKeyJSONRPC:      mcp.JSONRPCVersion,
		objects.FieldKeyID: idVal,
		rpcKeyResult:       result,
	}
	out, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return a.writeStdout(writer, out, format)
}

func (a *Adapter) writeError(writer *bufio.Writer, format *mcp.MessageFormat, id json.RawMessage, code int, message string) error {
	payload := map[string]any{
		rpcKeyJSONRPC: mcp.JSONRPCVersion,
		rpcKeyError: map[string]any{
			rpcKeyCode:    code,
			rpcKeyMessage: message,
		},
	}
	if len(id) > 0 && string(id) != "null" {
		var idVal any
		_ = json.Unmarshal(id, &idVal)
		payload[objects.FieldKeyID] = idVal
	}
	out, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if format == nil {
		format = &mcp.MessageFormat{IsRawJSON: true}
	}
	return a.writeStdout(writer, out, format)
}

func (a *Adapter) writeStdout(writer *bufio.Writer, out []byte, format *mcp.MessageFormat) error {
	a.queueMu.RLock()
	defer a.queueMu.RUnlock()
	if a.queue != nil {
		enqueued := a.queue.Enqueue(out, format, "normal", nil)
		if !enqueued {
			return fmt.Errorf("message dropped by queue")
		}
		return nil
	}
	return fmt.Errorf("queue not initialized")
}
