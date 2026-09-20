package logging

import (
	"context"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
)

func TestGetLoggerFromLoggingContext(t *testing.T) {
	ctx := context.Background()

	// Verify non-nil logging context
	logCtx := pkgctx.NewLoggingContext(pkgctx.ProfileSystem)
	logger := GetLoggerFromLoggingContext(ctx, logCtx)
	if logger == nil {
		t.Fatal("expected non-nil logger for system profile")
	}

	// Verify nil logging context defaults safely without panic (K:CQ-001)
	nilLogger := GetLoggerFromLoggingContext(ctx, nil)
	if nilLogger == nil {
		t.Fatal("expected non-nil logger for nil logging context fallback")
	}
}
