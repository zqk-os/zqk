// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package bufferpool_test

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/zqk-os/zqk/pkg/bufferpool"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

func TestGet_ReturnsResetBuffer(t *testing.T) {
	buf := bufferpool.Get()
	if buf == nil {
		t.Fatal("expected non-nil buffer from Get()")
	}
	if buf.Len() != 0 {
		t.Fatalf("expected empty buffer, got length %d", buf.Len())
	}

	n, err := buf.WriteString("first execution string")
	if err != nil {
		t.Fatalf("WriteString failed: %v", err)
	}
	if n != len("first execution string") {
		t.Fatalf("expected %d bytes written, got %d", len("first execution string"), n)
	}

	bufferpool.Put(buf)

	buf2 := bufferpool.Get()
	if buf2.Len() != 0 {
		t.Fatalf("expected recycled buffer to be reset, got length %d", buf2.Len())
	}
	bufferpool.Put(buf2)
}

func TestPut_NilBufferHandling(t *testing.T) {
	// Put(nil) must complete safely without panicking
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Put(nil) panicked: %v", r)
		}
	}()
	bufferpool.Put(nil)
}

func TestPut_OversizedBufferDiscarded(t *testing.T) {
	buf := bufferpool.Get()
	if buf == nil {
		t.Fatal("expected non-nil buffer")
	}

	// Grow capacity to strictly exceed 1MB (1024*1024)
	oversizedBytes := make([]byte, (1024*1024)+1024)
	n, err := buf.Write(oversizedBytes)
	if err != nil {
		t.Fatalf("failed to write oversized buffer: %v", err)
	}
	if n != len(oversizedBytes) {
		t.Fatalf("expected %d bytes written, got %d", len(oversizedBytes), n)
	}
	if buf.Cap() <= 1024*1024 {
		t.Fatalf("expected capacity > 1024*1024, got %d", buf.Cap())
	}

	// Put oversized buffer; it should be discarded rather than returned to pool
	bufferpool.Put(buf)

	// Fetch another buffer to ensure pool remains fully operational
	nextBuf := bufferpool.Get()
	if nextBuf == nil {
		t.Fatal("expected valid buffer after oversized discard")
	}
	if nextBuf.Len() != 0 {
		t.Fatalf("expected clean empty buffer, got %d", nextBuf.Len())
	}
	bufferpool.Put(nextBuf)
}

func TestPut_RetainsWithinLimit(t *testing.T) {
	buf := bufferpool.Get()
	if buf == nil {
		t.Fatal("expected non-nil buffer")
	}

	payload := []byte("standard operational payload within limit")
	n, err := buf.Write(payload)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if n != len(payload) {
		t.Fatalf("expected %d bytes written, got %d", len(payload), n)
	}

	if buf.Cap() > 1024*1024 {
		t.Fatalf("buffer unexpectedly exceeded 1MB limit: %d", buf.Cap())
	}

	bufferpool.Put(buf)

	recycled := bufferpool.Get()
	if recycled.Len() != 0 {
		t.Fatalf("expected recycled buffer to be empty, got %d", recycled.Len())
	}
	bufferpool.Put(recycled)
}

func TestConcurrentAccessSafety(t *testing.T) {
	const workers = 12
	const iterations = 60

	var wg sync.WaitGroup
	errCh := make(chan error, workers*iterations)

	for i := 0; i < workers; i++ {
		wg.Add(1)
		workerID := i
		goroutinelabels.NewGoroutine(
			"bufferpool_test",
			fmt.Sprintf("concurrent worker %d", workerID),
		).StartSimple(func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				buf := bufferpool.Get()
				if buf == nil {
					errCh <- errors.New("nil buffer returned by Get()")
					return
				}
				if buf.Len() != 0 {
					errCh <- fmt.Errorf("worker %d: non-empty buffer returned: %d bytes", workerID, buf.Len())
					bufferpool.Put(buf)
					return
				}

				msg := fmt.Sprintf("worker-%d-cycle-%d", workerID, j)
				n, err := buf.WriteString(msg)
				if err != nil {
					errCh <- fmt.Errorf("WriteString error: %w", err)
					bufferpool.Put(buf)
					return
				}
				if n != len(msg) {
					errCh <- fmt.Errorf("expected %d bytes written, got %d", len(msg), n)
					bufferpool.Put(buf)
					return
				}

				if buf.String() != msg {
					errCh <- fmt.Errorf("unexpected content: got %q, want %q", buf.String(), msg)
					bufferpool.Put(buf)
					return
				}

				bufferpool.Put(buf)
			}
		})
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Errorf("concurrency violation detected: %v", err)
	}
}

func TestStreamRead_EmptyReader(t *testing.T) {
	r := strings.NewReader("")
	called := false
	err := bufferpool.StreamRead(r, func(br *bufio.Reader) error {
		called = true
		b, readErr := br.ReadByte()
		if !errors.Is(readErr, io.EOF) {
			return fmt.Errorf("expected io.EOF on empty reader, got byte %v, err: %w", b, readErr)
		}
		return nil
	})

	if err != nil {
		t.Fatalf("unexpected StreamRead error: %v", err)
	}
	if !called {
		t.Fatal("expected handler function to be invoked")
	}
}

type errFailingReader struct {
	failErr error
}

func (e *errFailingReader) Read(p []byte) (int, error) {
	return 0, e.failErr
}

func TestStreamRead_ReaderError(t *testing.T) {
	expectedErr := errors.New("custom simulated disk read failure")
	failingReader := &errFailingReader{failErr: expectedErr}

	err := bufferpool.StreamRead(failingReader, func(br *bufio.Reader) error {
		_, readErr := br.ReadByte()
		if readErr != nil {
			return readErr
		}
		return nil
	})

	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected error %v, got %v", expectedErr, err)
	}
}

func TestStreamRead_HandlerError(t *testing.T) {
	r := strings.NewReader("sample payload")
	handlerErr := errors.New("handler validation error")

	err := bufferpool.StreamRead(r, func(br *bufio.Reader) error {
		line, readErr := br.ReadString('\n')
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			return readErr
		}
		if len(line) == 0 {
			return errors.New("expected data")
		}
		return handlerErr
	})

	if !errors.Is(err, handlerErr) {
		t.Fatalf("expected handler error %v, got %v", handlerErr, err)
	}
}

func TestStreamRead_LargeStreamChunks(t *testing.T) {
	// 128KB payload to cross the 64KB internal buffer boundary
	const totalSize = 128 * 1024
	srcData := make([]byte, totalSize)
	for i := 0; i < totalSize; i++ {
		srcData[i] = byte(i % 251)
	}

	r := bytes.NewReader(srcData)
	dest := bytes.NewBuffer(make([]byte, 0, totalSize))

	err := bufferpool.StreamRead(r, func(br *bufio.Reader) error {
		buf := make([]byte, 8192)
		for {
			n, readErr := br.Read(buf)
			if n > 0 {
				written, writeErr := dest.Write(buf[:n])
				if writeErr != nil {
					return writeErr
				}
				if written != n {
					return fmt.Errorf("partial write: %d of %d", written, n)
				}
			}
			if readErr != nil {
				if errors.Is(readErr, io.EOF) {
					break
				}
				return readErr
			}
		}
		return nil
	})

	if err != nil {
		t.Fatalf("unexpected stream read error on large stream: %v", err)
	}
	if dest.Len() != totalSize {
		t.Fatalf("expected %d bytes read, got %d", totalSize, dest.Len())
	}
	if !bytes.Equal(dest.Bytes(), srcData) {
		t.Fatal("streamed bytes mismatch original source data")
	}
}
