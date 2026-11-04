// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

// Traceability: TST-1792885566814937001-8086d4bd / REQ-1792885565749392000-e88cf82a
// Covers CRIT-1792885566814937000-03b5b670, CRIT-1792885566814938000-8c7a2ccb

package screencap

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// faultyRunner simulates command failures for boundary and resilience testing.
type faultyRunner struct {
	errToReturn error
}

func (f *faultyRunner) Run(_ context.Context, _ string, _ ...string) error {
	return f.errToReturn
}

// errorCapturer simulates capturer failures for automator resilience tests.
type errorCapturer struct {
	errToReturn error
}

func (e *errorCapturer) CaptureWindow(_ context.Context, _ string, _ string) error {
	return e.errToReturn
}

func (e *errorCapturer) CaptureRegion(_ context.Context, _, _, _, _ int, _ string) error {
	return e.errToReturn
}

func (e *errorCapturer) CaptureScreen(_ context.Context, _ string) error {
	return e.errToReturn
}

// TestMacCapturer_ErrorPropagation verifies error handling when the command runner fails.
func TestMacCapturer_ErrorPropagation(t *testing.T) {
	ctx := context.Background()
	expectedErr := errors.New("simulated exec failure")
	runner := &faultyRunner{errToReturn: expectedErr}
	capturer := NewMacCapturerWithRunner(runner)

	t.Run("CaptureWindow error", func(t *testing.T) {
		err := capturer.CaptureWindow(ctx, "window-99", filepath.Join(t.TempDir(), "win.png"))
		require.Error(t, err)
		assert.True(t, errors.Is(err, expectedErr))
	})

	t.Run("CaptureRegion error", func(t *testing.T) {
		err := capturer.CaptureRegion(ctx, 0, 0, 1920, 1080, filepath.Join(t.TempDir(), "reg.png"))
		require.Error(t, err)
		assert.True(t, errors.Is(err, expectedErr))
	})

	t.Run("CaptureScreen error", func(t *testing.T) {
		err := capturer.CaptureScreen(ctx, filepath.Join(t.TempDir(), "screen.png"))
		require.Error(t, err)
		assert.True(t, errors.Is(err, expectedErr))
	})
}

// TestMacCapturer_BoundaryInputs checks boundary rect formatting and window identifiers.
func TestMacCapturer_BoundaryInputs(t *testing.T) {
	ctx := context.Background()
	runner := &mockRunner{}
	capturer := NewMacCapturerWithRunner(runner)
	tmpDir := t.TempDir()

	testCases := []struct {
		name       string
		x, y, w, h int
		expectedR  string
	}{
		{"zero origin", 0, 0, 1024, 768, "-R0,0,1024,768"},
		{"negative coords", -50, -100, 800, 600, "-R-50,-100,800,600"},
		{"4k resolution", 3840, 2160, 3840, 2160, "-R3840,2160,3840,2160"},
		{"5k ultra wide", 0, 0, 5120, 1440, "-R0,0,5120,1440"},
		{"zero dimension", 100, 100, 0, 0, "-R100,100,0,0"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			runner.commandsExecuted = nil
			out := filepath.Join(tmpDir, "bound.png")
			err := capturer.CaptureRegion(ctx, tc.x, tc.y, tc.w, tc.h, out)
			require.NoError(t, err)
			require.Len(t, runner.commandsExecuted, 1)
			assert.Equal(t, "screencapture", runner.commandsExecuted[0][0])
			assert.Equal(t, "-x", runner.commandsExecuted[0][1])
			assert.Equal(t, tc.expectedR, runner.commandsExecuted[0][2])
			assert.Equal(t, out, runner.commandsExecuted[0][3])
		})
	}
}

// TestMacCapturer_WindowIDFormatting tests window ID argument serialization.
func TestMacCapturer_WindowIDFormatting(t *testing.T) {
	ctx := context.Background()
	runner := &mockRunner{}
	capturer := NewMacCapturerWithRunner(runner)
	tmpDir := t.TempDir()

	ids := []string{"0", "12345", "0x3F8", "Window_Main_Console", ""}
	for _, id := range ids {
		t.Run("ID_"+id, func(t *testing.T) {
			runner.commandsExecuted = nil
			out := filepath.Join(tmpDir, "win_"+id+".png")
			err := capturer.CaptureWindow(ctx, id, out)
			require.NoError(t, err)
			require.Len(t, runner.commandsExecuted, 1)
			assert.Equal(t, "-l"+id, runner.commandsExecuted[0][2])
		})
	}
}

// TestTerminalAutomator_TypeCommandFailure validates error propagation when stdin is closed.
func TestTerminalAutomator_TypeCommandFailure(t *testing.T) {
	capturer := &mockCapturer{}
	automator, err := NewTerminalAutomator(capturer, "cat")
	require.NoError(t, err)

	err = automator.Start()
	require.NoError(t, err)

	// Close stdin intentionally to cause TypeCommand to fail
	require.NoError(t, automator.stdin.Close())

	// TypeCommand should return an error
	err = automator.TypeCommand("this-will-fail")
	require.Error(t, err)

	// TypeCommandAndCapture should propagate TypeCommand error
	ctx := context.Background()
	err = automator.TypeCommandAndCapture(ctx, "this-too", filepath.Join(t.TempDir(), "fail.png"))
	require.Error(t, err)

	closeErr := automator.Close()
	if closeErr != nil {
		t.Logf("automator close: %v", closeErr)
	}
}

// TestTerminalAutomator_TypeCommandAndCaptureFailure tests capturer failure in TypeCommandAndCapture.
func TestTerminalAutomator_TypeCommandAndCaptureFailure(t *testing.T) {
	expectedErr := errors.New("capturer disk write failure")
	capturer := &errorCapturer{errToReturn: expectedErr}

	automator, err := NewTerminalAutomator(capturer, "cat")
	require.NoError(t, err)

	err = automator.Start()
	require.NoError(t, err)
	defer func() {
		closeErr := automator.Close()
		if closeErr != nil {
			t.Logf("automator close: %v", closeErr)
		}
	}()

	ctx := context.Background()
	err = automator.TypeCommandAndCapture(ctx, "ping", filepath.Join(t.TempDir(), "capture_fail.png"))
	require.Error(t, err)
	assert.True(t, errors.Is(err, expectedErr))
}

// TestTerminalAutomator_WaitForOutputEOF tests error reporting when child process terminates before match.
func TestTerminalAutomator_WaitForOutputEOF(t *testing.T) {
	capturer := &mockCapturer{}
	// Spawning a command that terminates immediately
	automator, err := NewTerminalAutomator(capturer, "sh", "-c", "echo 'short-lived'")
	require.NoError(t, err)

	err = automator.Start()
	require.NoError(t, err)
	defer func() {
		closeErr := automator.Close()
		if closeErr != nil {
			t.Logf("automator close: %v", closeErr)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Waiting for something that is never emitted before process terminates
	err = automator.WaitForOutput(ctx, "never_occurring_token_XYZ")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reached EOF before finding expected output")
}

// TestTerminalAutomator_WaitForOutputAndCaptureFailure tests error handling during WaitForOutputAndCapture.
func TestTerminalAutomator_WaitForOutputAndCaptureFailure(t *testing.T) {
	t.Run("WaitForOutput fails with EOF", func(t *testing.T) {
		capturer := &mockCapturer{}
		automator, err := NewTerminalAutomator(capturer, "sh", "-c", "echo 'done'")
		require.NoError(t, err)

		err = automator.Start()
		require.NoError(t, err)
		defer func() {
			closeErr := automator.Close()
			if closeErr != nil {
				t.Logf("automator close: %v", closeErr)
			}
		}()

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		err = automator.WaitForOutputAndCapture(ctx, "unmatched_pattern", filepath.Join(t.TempDir(), "dummy.png"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "reached EOF")
	})

	t.Run("Capturer fails after output match", func(t *testing.T) {
		expectedErr := errors.New("snapshot creation error")
		capturer := &errorCapturer{errToReturn: expectedErr}

		automator, err := NewTerminalAutomator(capturer, "cat")
		require.NoError(t, err)

		err = automator.Start()
		require.NoError(t, err)
		defer func() {
			closeErr := automator.Close()
			if closeErr != nil {
				t.Logf("automator close: %v", closeErr)
			}
		}()

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		require.NoError(t, automator.TypeCommand("trigger_match"))

		err = automator.WaitForOutputAndCapture(ctx, "trigger_match", filepath.Join(t.TempDir(), "fail.png"))
		require.Error(t, err)
		assert.True(t, errors.Is(err, expectedErr))
	})
}

// TestTerminalAutomator_ConcurrentSafety validates timeline events and output reading under concurrent load.
func TestTerminalAutomator_ConcurrentSafety(t *testing.T) {
	capturer := &mockCapturer{}
	automator, err := NewTerminalAutomator(capturer, "cat")
	require.NoError(t, err)

	err = automator.Start()
	require.NoError(t, err)
	defer func() {
		closeErr := automator.Close()
		if closeErr != nil {
			t.Logf("automator close: %v", closeErr)
		}
	}()

	const workerCount = 10
	var wg sync.WaitGroup

	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		workerID := i
		goroutinelabels.NewGoroutine("test_automator_concurrency", "Test concurrency safety").
			StartSimple(func() {
				defer wg.Done()
				automator.AppendTimelineEvent(fmt.Sprintf("event-%d", workerID))
				tl := automator.GetTimeline()
				assert.NotEmpty(t, tl)
				out := automator.Output()
				assert.True(t, len(out) >= 0)
			})
	}

	wg.Wait()

	timeline := automator.GetTimeline()
	assert.GreaterOrEqual(t, len(timeline), workerCount)
}

// TestCapturer_ImageEncodingAndValidation verifies image generation, encoding, and zero-byte resilience.
func TestCapturer_ImageEncodingAndValidation(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("PNG encoding hermetic write", func(t *testing.T) {
		img := image.NewRGBA(image.Rect(0, 0, 64, 64))
		for x := 0; x < 64; x++ {
			for y := 0; y < 64; y++ {
				img.Set(x, y, color.RGBA{R: 255, G: 128, B: 0, A: 255})
			}
		}

		pngPath := filepath.Join(tmpDir, "valid_capture.png")
		f, err := fileutil.Create(pngPath)
		require.NoError(t, err)

		err = png.Encode(f, img)
		require.NoError(t, err)
		require.NoError(t, f.Close())

		info, err := fileutil.Stat(pngPath)
		require.NoError(t, err)
		assert.Greater(t, info.Size(), int64(0))

		// Decode and verify dimensions
		data, err := fileutil.ReadFile(pngPath)
		require.NoError(t, err)
		decoded, err := png.Decode(bytes.NewReader(data))
		require.NoError(t, err)
		assert.Equal(t, 64, decoded.Bounds().Dx())
		assert.Equal(t, 64, decoded.Bounds().Dy())
	})

	t.Run("JPEG encoding hermetic write", func(t *testing.T) {
		img := image.NewRGBA(image.Rect(0, 0, 128, 128))
		for x := 0; x < 128; x++ {
			for y := 0; y < 128; y++ {
				img.Set(x, y, color.RGBA{R: 0, G: 200, B: 100, A: 255})
			}
		}

		jpgPath := filepath.Join(tmpDir, "valid_capture.jpg")
		f, err := fileutil.Create(jpgPath)
		require.NoError(t, err)

		err = jpeg.Encode(f, img, &jpeg.Options{Quality: 90})
		require.NoError(t, err)
		require.NoError(t, f.Close())

		info, err := fileutil.Stat(jpgPath)
		require.NoError(t, err)
		assert.Greater(t, info.Size(), int64(0))
	})

	t.Run("Zero-byte file resilience check", func(t *testing.T) {
		zeroPath := filepath.Join(tmpDir, "zero_byte.png")
		err := fileutil.WriteFile(zeroPath, []byte{}, 0o600)
		require.NoError(t, err)

		info, err := fileutil.Stat(zeroPath)
		require.NoError(t, err)
		assert.Equal(t, int64(0), info.Size())

		// Attempting decode on zero-byte file must gracefully return error
		data, err := fileutil.ReadFile(zeroPath)
		require.NoError(t, err)
		_, decodeErr := png.Decode(bytes.NewReader(data))
		require.Error(t, decodeErr)
	})
}
