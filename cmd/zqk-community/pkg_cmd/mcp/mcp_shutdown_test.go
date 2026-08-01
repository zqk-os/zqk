package mcp

import (
	"os"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/mcp"
	"github.com/stretchr/testify/assert"
)

func TestOSSignalTriggersShutdown(t *testing.T) {
	server := mcp.NewServer()

	// Simulate the signal handling setup from runServe
	sigChan := make(chan os.Signal, 1)
	// Don't use signal.Notify for test, just send to channel directly
	// signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		sig := <-sigChan
		server.RequestShutdown("received OS signal: " + sig.String())
	}()

	// Send an interrupt signal
	sigChan <- os.Interrupt

	// Give it a moment to process
	time.Sleep(10 * time.Millisecond)

	// Verify the server received the shutdown request
	ctx := server.GetShutdownContext()
	assert.NotNil(t, ctx)

	// Check the internal state (we can only observe side effects in public API or if we check GetShutdownContext)
	// Let's call order shutdown just to check if it returns a cancelled context if requested
	// Oh wait, RequestShutdown just sets the flag. We need to check if the flag is set.
	// Since we are outside the mcp package here, we can't read shutdownRequested.
	// We can try to serve and see if it exits immediately.

	// Another approach:
	errChan := make(chan error, 1)
	go func() {
		errChan <- server.Serve()
	}()

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
