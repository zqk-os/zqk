package translator_test

import (
	"encoding/json"
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/semantic/translator"
)

func TestManifestDrivenTranslator(t *testing.T) {
	manifest := translator.TranslationManifest{
		SourceFormat: "openapi-v3",
		TargetClass:  "APIEndpoint",
		Mappings: map[string]string{
			"operationId":               "name",
			objects.FieldKeyDescription: "docs",
			objects.FieldKeyTags:        "categories",
		},
	}

	trans := translator.NewManifestTranslator(manifest)

	if trans.SupportedFormat() != "openapi-v3" {
		t.Errorf("Expected openapi-v3, got %s", trans.SupportedFormat())
	}

	externalJSON := []byte(`{
		"operationId": "getUser",
		"description": "Gets a user",
		"tags": ["users", "read"],
		"ignoredField": "this should be dropped"
	}`)

	internalData, err := trans.TranslateToInternal("APIEndpoint", externalJSON)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if internalData[objects.FieldKeyName] != "getUser" {
		t.Errorf("Expected getUser, got %v", internalData[objects.FieldKeyName])
	}
	if internalData["docs"] != "Gets a user" {
		t.Errorf("Expected Gets a user, got %v", internalData["docs"])
	}
	if _, exists := internalData["ignoredField"]; exists {
		t.Errorf("Ignored field should not be present")
	}

	// Test mapping back to external
	externalBytes, err := trans.TranslateToExternal("APIEndpoint", internalData)
	if err != nil {
		t.Fatalf("Unexpected error mapping to external: %v", err)
	}

	var extMap map[string]any
	_ = json.Unmarshal(externalBytes, &extMap)

	if extMap["operationId"] != "getUser" {
		t.Errorf("Expected operationId: getUser, got %v", extMap["operationId"])
	}

	// Test mismatch error
	_, err = trans.TranslateToInternal("WrongClass", externalJSON)
	if err != translator.ErrManifestMismatch {
		t.Errorf("Expected ErrManifestMismatch, got: %v", err)
	}
}
