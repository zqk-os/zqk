package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
)

// Handler handles a JSON-RPC method call
// It receives the request context, method name, and raw params
// Returns the result or an error
type Handler interface {
	Handle(ctx context.Context, method string, params json.RawMessage) (any, error)
}

// HandlerFunc is a function type that implements Handler
type HandlerFunc func(ctx context.Context, method string, params json.RawMessage) (any, error)

// Handle implements Handler for HandlerFunc
func (f HandlerFunc) Handle(ctx context.Context, method string, params json.RawMessage) (any, error) {
	return f(ctx, method, params)
}

// MethodRouter routes method calls to specific handlers
type MethodRouter struct {
	handlers       map[string]Handler
	defaultHandler Handler
}

// NewMethodRouter creates a new method router
func NewMethodRouter() *MethodRouter {
	return &MethodRouter{
		handlers: make(map[string]Handler),
	}
}

// Register registers a handler for a specific method
func (r *MethodRouter) Register(method string, handler Handler) {
	r.handlers[method] = handler
}

// RegisterFunc registers a handler function for a specific method
func (r *MethodRouter) RegisterFunc(method string, handler func(ctx context.Context, method string, params json.RawMessage) (any, error)) {
	r.handlers[method] = HandlerFunc(handler)
}

// SetDefaultHandler sets the default handler for unregistered methods
func (r *MethodRouter) SetDefaultHandler(handler Handler) {
	r.defaultHandler = handler
}

// Handle routes a method call to the appropriate handler
func (r *MethodRouter) Handle(ctx context.Context, method string, params json.RawMessage) (any, error) {
	handler, ok := r.handlers[method]
	if !ok {
		if r.defaultHandler != nil {
			return r.defaultHandler.Handle(ctx, method, params)
		}
		return nil, &JSONRPCError{
			Code:    MethodNotFound,
			Message: fmt.Sprintf("Method not found: %s", method),
		}
	}
	return handler.Handle(ctx, method, params)
}

// Middleware wraps a handler with additional functionality
type Middleware func(Handler) Handler

// Chain chains multiple middlewares together
func Chain(middlewares ...Middleware) Middleware {
	return func(h Handler) Handler {
		for i := len(middlewares) - 1; i >= 0; i-- {
			h = middlewares[i](h)
		}
		return h
	}
}

// TraceWriterFunc is a function that returns the current trace writer
// This allows the trace middleware to use a dynamic trace writer that can change
// (e.g., when switching to client-specific trace files)
type TraceWriterFunc func() io.Writer

// TraceMiddleware creates a middleware that logs requests/responses
// It accepts either a static io.Writer or a TraceWriterFunc for dynamic trace writers
func TraceMiddleware(traceWriterOrFunc any) Middleware {
	var getTraceWriter func() io.Writer

	// Determine if we have a static writer or a function
	switch v := traceWriterOrFunc.(type) {
	case io.Writer:
		// Static writer - create a function that always returns it
		traceWriter := v
		getTraceWriter = func() io.Writer { return traceWriter }
	case TraceWriterFunc:
		// Dynamic function - use it directly
		getTraceWriter = v
	case func() io.Writer:
		// Also accept plain function type
		getTraceWriter = v
	default:
		// Invalid type - no tracing
		getTraceWriter = func() io.Writer { return nil }
	}

	return func(next Handler) Handler {
		return HandlerFunc(func(ctx context.Context, method string, params json.RawMessage) (any, error) {
			startTime := time.Now()
			traceWriter := getTraceWriter()

			// Extract request ID from params if available (for correlation)
			var requestID any
			var paramsMap map[string]any
			if err := json.Unmarshal(params, &paramsMap); err == nil {
				if id, ok := paramsMap[objects.FieldKeyID]; ok {
					requestID = id
				}
			}

			if traceWriter != nil {
				logger := logging.GetLoggerFromProfile(DefaultLoggingProfile)
				if requestID != nil {
					logging.Fluent(logger).Debug("MCP TRACE → Request").
						EmitComponent("mcp_handler").
						TraceMarker().
						RPCMethod(method).
						RequestID(fmt.Sprintf("%v", requestID)).
						Log()
				} else {
					logging.Fluent(logger).Debug("MCP TRACE → Request (notification)").
						EmitComponent("mcp_handler").
						TraceMarker().
						RPCMethod(method).
						RPCMessageType("notification").
						Log()
				}

			}

			result, err := next.Handle(ctx, method, params)
			duration := time.Since(startTime)

			traceWriter = getTraceWriter() // Get fresh writer in case it changed
			if traceWriter != nil {
				logger := logging.GetLoggerFromProfile(DefaultLoggingProfile)
				// Check if this is a notification sentinel (no response should be sent)
				notificationSentinel := &NotificationSentinel{}
				if errors.As(err, &notificationSentinel) {
					// Notifications are fire-and-forget per JSON-RPC 2.0 spec
					// No response is sent for notifications (requests without 'id' field)
					entry := logging.Fluent(logger).Debug("MCP TRACE ← Notification processed (no response sent per JSON-RPC 2.0 spec)").
						EmitComponent("mcp_handler").
						TraceMarker().
						RPCMethod(method).
						ElapsedString(duration.String())
					if requestID != nil {
						entry = entry.RequestID(fmt.Sprintf("%v", requestID))
					}
					entry.RPCMessageType("notification").Log()
				} else if err != nil {
					entry := logging.Fluent(logger).Debug("MCP TRACE ← Response (error)").
						EmitComponent("mcp_handler").
						TraceMarker().
						RPCMethod(method).
						ElapsedString(duration.String())
					if requestID != nil {
						entry = entry.RequestID(fmt.Sprintf("%v", requestID))
					}
					entry.WithError(err).Log()
				} else {
					success := logging.Fluent(logger).Debug("MCP TRACE ← Response (success)").
						EmitComponent("mcp_handler").
						TraceMarker().
						RPCMethod(method).
						ElapsedString(duration.String())
					if requestID != nil {
						success = success.RequestID(fmt.Sprintf("%v", requestID))
					}
					success.Log()
				}
			}

			return result, err
		})
	}
}
