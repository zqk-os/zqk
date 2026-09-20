package docman

import (
	"github.com/zqk-os/zqk/pkg/paths"
	"os"
	"path/filepath"
	"testing"
)

func TestToTitleCase(t *testing.T) {
	t.Parallel()
	cases := []struct {
		input    string
		expected string
	}{
		{"hello world", "Hello World"},
		{"quick start guide", "Quick Start Guide"},
		{"already Title", "Already Title"},
		{"", ""},
		{"a", "A"},
	}

	for _, tc := range cases {
		got := toTitleCase(tc.input)
		if got != tc.expected {
			t.Errorf("toTitleCase(%q) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}

func TestParser_ParseFileWithTitleAndSummary(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	docFile := filepath.Join(tmpDir, "sample-doc.md")

	content := `# Architecture Guidelines v1.0

## Overview

This is the system architecture overview document explaining the kernel layers.

## Details

Some details.
`
	if err := os.WriteFile(docFile, []byte(content), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write test doc: %v", err)
	}

	p := NewParser()
	meta, err := p.Parse(docFile)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if meta.Title != "Architecture Guidelines" {
		t.Errorf("expected Title 'Architecture Guidelines', got %q", meta.Title)
	}
	if meta.Summary != "This is the system architecture overview document explaining the kernel layers." {
		t.Errorf("unexpected Summary: %q", meta.Summary)
	}
}

func TestParser_FallbackToFilename(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	docFile := filepath.Join(tmpDir, "kernel_storage_engine.md")

	content := `Just some content without an h1 heading.
`
	if err := os.WriteFile(docFile, []byte(content), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write test doc: %v", err)
	}

	p := NewParser()
	meta, err := p.Parse(docFile)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if meta.Title != "Kernel Storage Engine" {
		t.Errorf("expected Title 'Kernel Storage Engine', got %q", meta.Title)
	}
}
