package system

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/paths"
)

func TestNewCheckCmd(t *testing.T) {
	t.Parallel()
	cmd := NewCheckCmd()
	if cmd == nil {
		t.Fatal("newCheckCmd() returned nil")
	}

	if !strings.HasPrefix(cmd.Use, "check") {
		t.Errorf("Expected command use to start with 'check', got '%s'", cmd.Use)
	}

	if cmd.Short == emptyValue {
		t.Error("Command should have a short description")
	}
}

func TestCheckCommandFlags(t *testing.T) {
	t.Parallel()
	cmd := NewCheckCmd()

	// Test verbose flag
	if cmd.Flags().Lookup("verbose") == nil {
		t.Error("Command should have --verbose flag")
	}

	// Test format flag
	formatFlag := cmd.Flags().Lookup("format")
	if formatFlag == nil {
		t.Error("Command should have --format flag")
	} else {
		// Verify default format
		defaultVal, err := cmd.Flags().GetString("format")
		if err != nil {
			defaultVal = ""
		}
		if defaultVal == emptyValue {
			t.Error("Format flag should have a default value")
		}
	}

	// Test auto-fix flag
	if cmd.Flags().Lookup("auto-fix") == nil {
		t.Error("Command should have --auto-fix flag")
	}

	// Test background flag (default: wait for results; --background returns immediately with operation ID)
	backgroundFlag := cmd.Flags().Lookup("background")
	if backgroundFlag == nil {
		t.Error("Command should have --background flag")
	} else {
		defaultVal, _ := cmd.Flags().GetBool("background") //nolint:errcheck
		if defaultVal {
			t.Error("--background should default to false (default: wait for completion)")
		}
	}
}

func TestCheckCommandRegistration(t *testing.T) {
	t.Parallel()
	// Verify command can be added to root
	cliCmd := paths.CLICommandName
	if cliCmd == emptyValue {
		cliCmd = paths.CLICommandNameDefault
	}
	rootCmd := &cobra.Command{Use: cliCmd}
	checkCmd := NewCheckCmd()
	rootCmd.AddCommand(checkCmd)

	// Verify command is registered
	found := false
	for _, cmd := range rootCmd.Commands() {
		if strings.HasPrefix(cmd.Use, "check") {
			found = true
			break
		}
	}
	if !found {
		t.Error("Check command should be registered in root command")
	}
}
