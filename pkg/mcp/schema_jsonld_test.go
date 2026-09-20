package mcp

import (
	"testing"
)

func TestJSONLDOntologyNamespace(t *testing.T) {
	orig := GetJSONLDOntologyNamespace()
	defer SetJSONLDOntologyNamespace(orig)

	custom := "https://zqk.org/ns/v1/"
	if err := SetJSONLDOntologyNamespace(custom); err != nil {
		t.Fatalf("SetJSONLDOntologyNamespace failed: %v", err)
	}

	if GetJSONLDOntologyNamespace() != custom {
		t.Errorf("Expected ontology namespace '%s', got '%s'", custom, GetJSONLDOntologyNamespace())
	}

	ctxMap := getJSONLDContext()
	if ctxMap["zqk"] != custom {
		t.Errorf("Expected zqk key in context map to be '%s', got '%v'", custom, ctxMap["zqk"])
	}

	// Test invalid URIs
	if err := SetJSONLDOntologyNamespace(""); err == nil {
		t.Error("Expected error when setting empty URI")
	}
	if err := SetJSONLDOntologyNamespace("ftp://invalid.uri"); err == nil {
		t.Error("Expected error when setting non-http/https URI")
	}
	if err := SetJSONLDOntologyNamespace("http://[invalid-host"); err == nil {
		t.Error("Expected error when setting malformed URL")
	}
}
