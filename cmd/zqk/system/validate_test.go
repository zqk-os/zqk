package system

import (
	"strings"
	"testing"
)

func TestNewValidateCmd(t *testing.T) {
	t.Parallel()
	cmd := NewValidateCmd()
	if cmd == nil {
		t.Fatal("NewValidateCmd() returned nil")
	}

	if !strings.HasPrefix(cmd.Use, "validate") {
		t.Errorf("Expected command use to start with 'validate', got '%s'", cmd.Use)
	}

	if cmd.Short == emptyValue {
		t.Error("Command should have a short description")
	}
}

func TestValidateCommandFlags(t *testing.T) {
	t.Parallel()
	cmd := NewValidateCmd()

	// Test all flag
	if cmd.Flags().Lookup("all") == nil {
		t.Error("Command should have --all flag")
	}

	// Test kind flag
	if cmd.Flags().Lookup("kind") == nil {
		t.Error("Command should have --kind flag")
	}

	// Test fix flag
	if cmd.Flags().Lookup("fix") == nil {
		t.Error("Command should have --fix flag")
	}

	// Test quiet flag
	if cmd.Flags().Lookup("quiet") == nil {
		t.Error("Command should have --quiet flag")
	}
}

// Note: Full integration test for validate would require a properly initialized
// ZQK project with objects. This is tested manually or in integration tests.
