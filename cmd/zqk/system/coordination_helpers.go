package system

import (
	"context"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
)

// createContextWithLoggingProfile creates a context with LoggingContext embedded from profile string
// This ensures coordinator logging events respect --context profile settings
func createContextWithLoggingProfile(ctx context.Context, profile string) context.Context {
	if ctx == nil {
		ctx = pkgctx.NewSystemContext()
	}

	if profile == emptyValue {
		profile = systemProfileHuman // Default
	}

	// Convert profile string to LoggingProfile enum
	var loggingCtx *pkgctx.LoggingContext
	switch profile {
	case systemProfileMCP:
		loggingCtx = pkgctx.NewLoggingContext(pkgctx.ProfileMCP)
	case systemProfileSystem:
		loggingCtx = pkgctx.NewSystemLoggingContext()
	case systemProfileAIAgent:
		loggingCtx = pkgctx.NewLoggingContext(pkgctx.ProfileAIAgent)
	case systemProfileDebug:
		loggingCtx = pkgctx.NewLoggingContext(pkgctx.ProfileDebug)
	case systemProfileHuman, "":
		loggingCtx = pkgctx.NewHumanLoggingContext()
	default:
		loggingCtx = pkgctx.NewHumanLoggingContext()
	}

	return pkgctx.WithLoggingContext(ctx, loggingCtx)
}
