package mcp

import (
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestDetectMIMEType_JSONL(t *testing.T) {
	mimeType := DetectMIMEType("/tmp/events.jsonl")
	if mimeType != "application/x-ndjson; charset=utf-8" {
		t.Fatalf("expected jsonl mime type, got %q", mimeType)
	}
}

func TestDetectMIMEType_NDJSON(t *testing.T) {
	mimeType := DetectMIMEType("/tmp/events.ndjson")
	if mimeType != "application/x-ndjson; charset=utf-8" {
		t.Fatalf("expected ndjson mime type, got %q", mimeType)
	}
}

func TestExtractMetadata_JSONWithCharset_UsesJSONAdapter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "resource.json")
	if err := fileutil.WriteSecureFile(path, []byte(`{"title":"hello","description":"world","kind":"sample"}`)); err != nil {
		t.Fatalf("write file: %v", err)
	}

	registry := NewResourceMIMEAdapterRegistry()
	metadata := registry.ExtractMetadata(path, "application/json; charset=utf-8")
	if metadata[objects.FieldKeyTitle] != "hello" {
		t.Fatalf("expected title metadata from JSON adapter, got %q", metadata[objects.FieldKeyTitle])
	}
	if metadata[objects.FieldKeyKind] != "sample" {
		t.Fatalf("expected kind metadata from JSON adapter, got %q", metadata[objects.FieldKeyKind])
	}
}
