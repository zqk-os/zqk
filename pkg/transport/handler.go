package transport

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/zqk-os/zqk/pkg/logging"
)

// Request encapsulates a decoded payload along with the raw HTTP request context.
type Request[Req any] struct {
	Context context.Context
	Payload Req
	Raw     *http.Request
}

// Response encapsulates standard HTTP response elements.
type Response[Res any] struct {
	StatusCode int
	Body       Res
	Headers    map[string]string
}

// Handler defines a strongly-typed, generic function signature for HTTP handlers.
type Handler[Req any, Res any] func(req Request[Req]) (Response[Res], error)

// ErrorHandler is a fallback function to run if the standard handler encounters an unhandled error.
type ErrorHandler func(w http.ResponseWriter, r *http.Request, err error)

// Options configures the StandardHandler.
type Options struct {
	Logger      logging.Logger
	HandlerName string
	OnError     ErrorHandler
}

// NewHTTPHandler creates an http.HandlerFunc from a typed Handler.
// It centralizes payload decoding, error handling, JSON serialization, and telemetry.
func NewHTTPHandler[Req any, Res any](opts Options, handle Handler[Req, Res]) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		var payload Req
		if r.Body != nil && r.ContentLength > 0 {
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil && err != io.EOF {
				if opts.Logger != nil {
					opts.Logger.Warn("Failed to parse request body",
						logging.HandlerField(opts.HandlerName),
						logging.ErrorTextField(err.Error()),
					)
				}
				if opts.OnError != nil {
					opts.OnError(w, r, err)
				} else {
					http.Error(w, "Invalid payload", http.StatusBadRequest)
				}
				return
			}
		}

		req := Request[Req]{
			Context: r.Context(),
			Payload: payload,
			Raw:     r,
		}

		// Execute core business logic
		res, err := handle(req)

		// Telemetry
		if opts.Logger != nil {
			fields := []logging.Field{
				logging.HandlerField(opts.HandlerName),
				logging.MethodField(r.Method),
				logging.PathField(r.URL.Path),
				logging.ElapsedStringField(time.Since(start).String()),
			}

			if err != nil {
				opts.Logger.Error("Request failed", err, fields...)
			} else {
				fields = append(fields, logging.StatusIntField(res.StatusCode))
				opts.Logger.Debug("Request completed", fields...)
			}
		}

		if err != nil {
			if opts.OnError != nil {
				opts.OnError(w, r, err)
			} else {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
			return
		}

		w.Header().Set("Content-Type", "application/json")
		for k, v := range res.Headers {
			w.Header().Set(k, v)
		}

		w.WriteHeader(res.StatusCode)

		// If the response body is not nil-like, encode it.
		// For Res=any, we just encode. If it's a map or struct, it serializes properly.
		_ = json.NewEncoder(w).Encode(res.Body)
	}
}
