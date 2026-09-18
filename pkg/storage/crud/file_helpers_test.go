package crud_test

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/storage"
)

func TestVerifyEmbeddedChecksum(t *testing.T) {
	// Create filtered data manually
	filteredData := []byte("field1: value1\nfield2: value2\n")
	expectedChecksum := storage.CalculateSHA256Hash(filteredData)

	// Update original data with actual expected checksum
	dataWithCorrectChecksum := []byte("field1: value1\nsha256_checksum: " + expectedChecksum + "\nfield2: value2\n")

	obj := map[string]any{
		"field1":          "value1",
		"field2":          "value2",
		"sha256_checksum": expectedChecksum,
	}

	err := storage.VerifyEmbeddedChecksum(dataWithCorrectChecksum, obj)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// Test with incorrect checksum
	objIncorrect := map[string]any{
		"field1":          "value1",
		"field2":          "value2",
		"sha256_checksum": "incorrect_checksum",
	}

	err = storage.VerifyEmbeddedChecksum(dataWithCorrectChecksum, objIncorrect)
	if err == nil {
		t.Fatalf("Expected error with incorrect checksum, got nil")
	}
}
