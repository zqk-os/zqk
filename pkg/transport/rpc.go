package transport

import (
	"context"
	"time"

	"github.com/lanceman/zqk/pkg/logging"
)

// RPCOptions configures the RPC handler.
type RPCOptions struct {
	Logger      logging.Logger
	HandlerName string
	OnError     func(err error) error
}

// RPCHandler defines a generic function signature for RPC handlers.
type RPCHandler[Req any, Res any] func(ctx context.Context, req Req) (Res, error)

// NewRPCHandler wraps a generic RPC handler with standard telemetry and error handling.
func NewRPCHandler[Req any, Res any](opts RPCOptions, handle RPCHandler[Req, Res]) RPCHandler[Req, Res] {
	return func(ctx context.Context, req Req) (Res, error) {
		start := time.Now()

		res, err := handle(ctx, req)

		if opts.Logger != nil {
			fields := []logging.Field{
				logging.HandlerField(opts.HandlerName),
				logging.ProtocolField("rpc"),
				logging.ElapsedStringField(time.Since(start).String()),
			}
			if err != nil {
				opts.Logger.Error("RPC request failed", err, fields...)
			} else {
				opts.Logger.Debug("RPC request completed", fields...)
			}
		}

		if err != nil {
			if opts.OnError != nil {
				return res, opts.OnError(err)
			}
			return res, err
		}

		return res, nil
	}
}
