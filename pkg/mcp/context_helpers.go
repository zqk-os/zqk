package mcp

import (
	"context"

	pkgctx "github.com/lanceman/zqk/pkg/context"
)

// EnsureContext returns ctx if non-nil, otherwise a new system context.
// Use this at tool and handler entry points so downstream code never receives a nil context
// and gets a consistent system context (security, logging) when the request context is missing.
func EnsureContext(ctx context.Context) context.Context {
	if ctx == nil {
		return pkgctx.NewSystemContext()
	}
	return ctx
}
