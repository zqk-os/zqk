package transport

import (
	"context"
	"io"
	"time"

	"github.com/zqk-os/zqk/pkg/logging"
)

// FTPRequest encapsulates an FTP command and its arguments, along with a context.
type FTPRequest[Req any] struct {
	Context context.Context
	Command string
	Payload Req
	Data    io.Reader
}

// FTPResponse encapsulates standard FTP response elements.
type FTPResponse[Res any] struct {
	Code    int
	Message string
	Body    Res
	Data    io.Writer
}

// FTPOptions configures the FTP handler.
type FTPOptions struct {
	Logger      logging.Logger
	HandlerName string
	OnError     func(err error) (int, string)
}

// FTPHandler defines a generic function signature for FTP handlers.
type FTPHandler[Req any, Res any] func(req FTPRequest[Req]) (FTPResponse[Res], error)

// NewFTPHandler wraps an FTP handler with standard telemetry and error handling.
func NewFTPHandler[Req any, Res any](opts FTPOptions, handle FTPHandler[Req, Res]) FTPHandler[Req, Res] {
	return func(req FTPRequest[Req]) (FTPResponse[Res], error) {
		start := time.Now()

		res, err := handle(req)

		if opts.Logger != nil {
			fields := []logging.Field{
				logging.HandlerField(opts.HandlerName),
				logging.ProtocolField("ftp"),
				logging.CommandField(req.Command),
				logging.ElapsedStringField(time.Since(start).String()),
			}
			if err != nil {
				opts.Logger.Error("FTP request failed", err, fields...)
			} else {
				fields = append(fields, logging.CodeField(res.Code))
				opts.Logger.Debug("FTP request completed", fields...)
			}
		}

		if err != nil {
			if opts.OnError != nil {
				code, msg := opts.OnError(err)
				res.Code = code
				res.Message = msg
			} else {
				res.Code = 500
				res.Message = "Internal Server Error"
			}
			return res, err
		}

		return res, nil
	}
}
