package mcp

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/mcp"
)

func TestOSSignalTriggersShutdown(t *testing.T) {
	server := mcp.NewServer()

	// Simulate the signal handling setup from runServe
	sigChan := make(chan os.Signal, 1)
	// Don't use signal.Notify for test, just send to channel directly
	// signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	goroutinelabels.StartNamedGoroutine("mcp_test_shutdown_sig", "simulate os signal", func() {
		sig := <-sigChan
		server.RequestProcessShutdown("received OS signal: " + sig.String())
	})

	// Send an interrupt signal
	sigChan <- os.Interrupt

	// Give it a moment to process
	time.Sleep(10 * time.Millisecond)

	// Verify the server received the shutdown request
	ctx := server.GetShutdownContext()
	assert.NotNil(t, ctx)

	// Another approach:
	errChan := make(chan error, 1)
	goroutinelabels.StartNamedGoroutine("mcp_test_serve", "mcp server run", func() {
		errChan <- server.Serve()
	})

	// Wait a bit, then check if it stopped
	// Wait, Serve() blocks on reading from stdin. So it won't stop unless shutdown is ordered or stdin is closed.
	// RequestShutdown causes serve loop to handleExplicitShutdown. Let's see if we get nil back.

	select {
	case err := <-errChan:
		assert.NoError(t, err)
	case <-time.After(1 * time.Second):
		t.Fatal("Serve() did not exit after shutdown request")
	}
}
