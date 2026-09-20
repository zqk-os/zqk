// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

// Package bufferpool provides sync.Pool byte buffers and streaming readers for hot read/write paths (L:F-PERF-01).
package bufferpool

import (
	"bufio"
	"bytes"
	"io"
	"sync"
)

var defaultBufferPool = sync.Pool{
	New: func() interface{} {
		return new(bytes.Buffer)
	},
}

// Get returns a pooled *bytes.Buffer with its buffer reset.
func Get() *bytes.Buffer {
	buf := defaultBufferPool.Get().(*bytes.Buffer)
	buf.Reset()
	return buf
}

// Put returns a *bytes.Buffer to the pool if it hasn't exceeded max capacity.
func Put(buf *bytes.Buffer) {
	if buf == nil {
		return
	}
	// Avoid retaining oversized buffers
	if buf.Cap() > 1024*1024 { // 1MB
		return
	}
	buf.Reset()
	defaultBufferPool.Put(buf)
}

// StreamRead reads from r using a pooled bufio.Reader to prevent eager whole-file allocation.
func StreamRead(r io.Reader, handle func(reader *bufio.Reader) error) error {
	br := bufio.NewReaderSize(r, 64*1024)
	return handle(br)
}
