package zqksession

import (
	"context"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
)

type sessionIDKey struct{}

// GetIDFromContext returns the session ID attached to ctx.
func GetIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return EmptyValue
	}
	sessionID, _ := ctx.Value(sessionIDKey{}).(string)
	return sessionID
}

// WithID attaches sessionID to ctx.
func WithID(ctx context.Context, sessionID string) context.Context {
	if ctx == nil {
		ctx = pkgctx.NewSystemContext()
	}
	return context.WithValue(ctx, sessionIDKey{}, sessionID)
}
