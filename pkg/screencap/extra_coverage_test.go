package screencap

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMacCapturer_DefaultRunner(t *testing.T) {
	c := NewMacCapturer()
	assert.NotNil(t, c)
	assert.NotNil(t, c.runner)

	// Run echo using defaultRunner
	ctx := context.Background()
	err := c.runner.Run(ctx, "echo", "test")
	assert.NoError(t, err)
}

func TestTerminalAutomator_ExtraCoverage(t *testing.T) {
	cap := &mockCapturer{}
	automator, err := NewTerminalAutomator(cap, "cat")
	require.NoError(t, err)
	defer func() { _ = automator.Close() }()

	err = automator.Start()
	require.NoError(t, err)

	// Test Output and GetTimeline initial state
	assert.Empty(t, automator.Output())
	initialTimeline := automator.GetTimeline()
	assert.Empty(t, initialTimeline)

	// Test TypeCommand and timeline recording
	err = automator.TypeCommand("hello world")
	assert.NoError(t, err)

	timeline := automator.GetTimeline()
	assert.NotEmpty(t, timeline)
	assert.Equal(t, "hello world\n", timeline[0].Text)

	// Test WaitForOutput success
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err = automator.WaitForOutput(ctx, "hello world")
	assert.NoError(t, err)
	assert.Contains(t, automator.Output(), "hello world")

	// Test WaitForOutput with cancelled context
	canceledCtx, cancelImmediate := context.WithCancel(context.Background())
	cancelImmediate()

	err = automator.WaitForOutput(canceledCtx, "will-not-match")
	assert.ErrorIs(t, err, context.Canceled)

	// Test WaitForOutputAndCapture with nil capturer
	automatorNoCap, err := NewTerminalAutomator(nil, "cat")
	require.NoError(t, err)
	defer func() { _ = automatorNoCap.Close() }()
	require.NoError(t, automatorNoCap.Start())
	require.NoError(t, automatorNoCap.TypeCommand("foo"))

	err = automatorNoCap.WaitForOutputAndCapture(ctx, "foo", "dummy.png")
	assert.NoError(t, err)
}
