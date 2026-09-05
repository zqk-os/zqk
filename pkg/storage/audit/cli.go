package audit

import (
	"context"

	pkgctx "github.com/lanceman/zqk/pkg/context"
)

// CLIMarkerKey is the context value key for CLI-privileged storage operations.
type CLIMarkerKey struct{}

// HasCLIMarker reports whether ctx was wrapped by WithCLIOperation.
func HasCLIMarker(ctx context.Context) bool {
	return ctx != nil && ctx.Value(CLIMarkerKey{}) != nil
}

// IsCLIOperation is true when the context is a CLI operation or the security
// context carries bypass_policy.
func IsCLIOperation(ctx context.Context, secCtx *pkgctx.SecurityContext) bool {
	if HasCLIMarker(ctx) {
		return true
	}
	if secCtx != nil {
		for _, p := range secCtx.Permissions {
			if p == "bypass_policy" {
				return true
			}
		}
	}
	return false
}

// WithCLIOperation marks a context as a CLI operation.
func WithCLIOperation(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, CLIMarkerKey{}, true)
}
