package telemetry

import (
	"context"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/logging"
)

func TestTelemetryDaemon_Aggregate(t *testing.T) {
	logger := logging.GetLoggerFromProfile("system")
	daemon := NewDaemon(logger)

	err := daemon.Aggregate(context.Background())
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
}

func TestTelemetryDaemon_RunBackgroundGC(t *testing.T) {
	logger := logging.GetLoggerFromProfile("system")
	daemon := NewDaemon(logger)

	// Use a tiny interval and a short timeout context to ensure it runs without hanging
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	// Should block until context is cancelled, then return
	daemon.RunBackgroundGC(ctx, []string{"."}, time.Hour, 2*time.Millisecond)
}
