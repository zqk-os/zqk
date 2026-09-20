package mcp

import "testing"

func TestDetermineContentType_UsesOutputTypeRegistryForJSONL(t *testing.T) {
	s := &Server{}

	contentType, mimeType := s.determineContentType(nil, "jsonl")
	if contentType != "text" {
		t.Fatalf("expected content type text, got %q", contentType)
	}
	if mimeType != "application/x-ndjson; charset=utf-8" {
		t.Fatalf("expected jsonl mime type, got %q", mimeType)
	}
}

func TestDetermineContentType_AllowedFormatFallbackUsesCanonicalMIME(t *testing.T) {
	s := &Server{
		allowedFormats: []string{"yaml"},
	}

	contentType, mimeType := s.determineContentType(nil, "json")
	if contentType != "text" {
		t.Fatalf("expected content type text, got %q", contentType)
	}
	if mimeType != "application/yaml; charset=utf-8" {
		t.Fatalf("expected canonical yaml mime type, got %q", mimeType)
	}
}
