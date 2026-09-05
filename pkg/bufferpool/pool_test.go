// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package bufferpool_test

import (
	"bufio"
	"bytes"
	"io"
	"testing"

	"github.com/lanceman/zqk/pkg/bufferpool"
)

func TestBufferPool(t *testing.T) {
	buf := bufferpool.Get()
	if buf.Len() != 0 {
		t.Errorf("expected empty buffer, got %d bytes", buf.Len())
	}
	buf.WriteString("hello buffer pool")
	if buf.String() != "hello buffer pool" {
		t.Errorf("unexpected content: %s", buf.String())
	}
	bufferpool.Put(buf)

	buf2 := bufferpool.Get()
	if buf2.Len() != 0 {
		t.Errorf("expected recycled buffer to be empty, got %d bytes", buf2.Len())
	}
	bufferpool.Put(buf2)
}

func TestStreamRead(t *testing.T) {
	data := []byte("stream test line 1\nstream test line 2\n")
	r := bytes.NewReader(data)

	var lines []string
	err := bufferpool.StreamRead(r, func(br *bufio.Reader) error {
		for {
			line, err := br.ReadString('\n')
			if err != nil {
				if err == io.EOF {
					if len(line) > 0 {
						lines = append(lines, line)
					}
					break
				}
				return err
			}
			lines = append(lines, line)
		}
		return nil
	})

	if err != nil {
		t.Fatalf("unexpected stream read error: %v", err)
	}
	if len(lines) != 2 {
		t.Errorf("expected 2 lines, got %d", len(lines))
	}
}
