package validation

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/lanceman/zqk/pkg/objects"
)

// TestGoValidator_ValidateChecksum tests the validateChecksum function for DSIA
func TestGoValidator_ValidateChecksum(t *testing.T) {
	t.Parallel()
	validator := &GoValidator{}

	t.Run("no checksum field", func(t *testing.T) {
		obj := map[string]any{
			objects.FieldKeyID:   "test-1",
			objects.FieldKeyName: "no checksum",
		}
		err := validator.validateChecksum(obj)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("valid checksum", func(t *testing.T) {
		obj := map[string]any{
			objects.FieldKeyID:   "test-2",
			objects.FieldKeyName: "valid checksum",
		}
		data, err := json.Marshal(obj)
		if err != nil {
			t.Fatalf("failed to marshal object: %v", err)
		}
		hash := sha256.Sum256(data)
		checksum := fmt.Sprintf("%x", hash)

		obj["sha256_checksum"] = checksum
		validationErr := validator.validateChecksum(obj)
		if validationErr != nil {
			t.Errorf("expected no error, got %v", validationErr)
		}
	})

	t.Run("invalid checksum", func(t *testing.T) {
		obj := map[string]any{
			objects.FieldKeyID:   "test-3",
			objects.FieldKeyName: "invalid checksum",
			"sha256_checksum":    "badchecksum123",
		}
		err := validator.validateChecksum(obj)
		if err == nil {
			t.Errorf("expected error, got nil")
		} else if err.Field != "sha256_checksum" || err.Rule != "checksum" {
			t.Errorf("unexpected error: %+v", err)
		}
	})
}
