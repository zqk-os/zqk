package scheduler

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestStreamingOutputWriter_WriteAndPreview(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "out.txt")
	f, err := fileutil.Create(outPath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	// Small ring: keep last 16 bytes
	w := newStreamingOutputWriter(f, 16)

	_, _ = w.Write([]byte("hello "))
	_, _ = w.Write([]byte("world"))

	if got := w.Preview(); got != "hello world" {
		t.Errorf("Preview() = %q, want %q", got, "hello world")
	}
	if got := w.Len(); got != 11 {
		t.Errorf("Len() = %d, want 11", got)
	}

	// Exceed cap: only last 16 bytes kept in ring
	_, _ = w.Write([]byte(" and more text that overflows"))
	if got := w.Preview(); got != "t that overflows" {
		t.Errorf("after overflow Preview() = %q, want %q", got, "t that overflows")
	}
	if w.Len() != 16 {
		t.Errorf("Len() = %d, want 16", w.Len())
	}

	_ = f.Sync()
	content, _ := fileutil.ReadFile(outPath)
	if full := string(content); full != "hello world and more text that overflows" {
		t.Errorf("file content = %q", full)
	}
}

func TestStreamingOutputWriter_NoFile(t *testing.T) {
	w := newStreamingOutputWriter(nil, 8)
	_, _ = w.Write([]byte("abcd"))
	_, _ = w.Write([]byte("efgh"))
	if got := w.Preview(); got != "abcdefgh" {
		t.Errorf("Preview() = %q", got)
	}
	_, _ = w.Write([]byte("ij"))
	// Ring keeps last 8 bytes of "abcdefghij" = "cdefghij"
	if got := w.Preview(); got != "cdefghij" {
		t.Errorf("after overflow Preview() = %q", got)
	}
}

func TestReadLastBytesFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tail.txt")
	content := []byte("line1\nline2\nline3\nlast")
	if err := fileutil.WriteFile(path, content, paths.FilePerm600); err != nil {
		t.Fatal(err)
	}

	// Read more than file size -> full file
	got, err := readLastBytesFromFile(path, 100)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, content) {
		t.Errorf("read last 100 = %q", got)
	}

	// Read last 10 bytes
	got, err = readLastBytesFromFile(path, 10)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "line3\nlast" {
		t.Errorf("read last 10 = %q", got)
	}

	// Missing file
	got, err = readLastBytesFromFile(filepath.Join(dir, "nonexistent"), 10)
	if err == nil || got != nil {
		t.Errorf("expected error and nil for missing file, got err=%v got=%v", err, got)
	}
}
