package object

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/lanceman/zqk/pkg/objects"
)

// TestHelpMenusAccuracy tests that help menus are accurate and complete for all command types
func TestHelpMenusAccuracy(t *testing.T) {
	if raceDetectorEnabled {
		t.Skip("skipping heavy help test under race detector")
	}
	_, cliBinary := setupCLITestEnvironmentForComprehensive(t)

	// Get all discoverable object kinds
	fieldRegistry := objects.GetGlobalFieldRegistry()
	if err := fieldRegistry.LoadFields(); err != nil {
		t.Fatalf("failed to load field registry: %v", err)
	}

	kinds, err := fieldRegistry.GetAllKinds()
	if err != nil {
		t.Fatalf("failed to get all kinds: %v", err)
	}

	// Test help for object commands
	t.Run("ObjectCommands", func(t *testing.T) {
		testObjectCommandHelp(t, cliBinary, kinds)
	})

	// Test help for bulk commands
	t.Run("BulkCommands", func(t *testing.T) {
		testBulkCommandHelp(t, cliBinary)
	})

	// Test help for system commands
	t.Run("SystemCommands", func(t *testing.T) {
		testSystemCommandHelp(t, cliBinary)
	})

	// Test help for internal commands
	t.Run("InternalCommands", func(t *testing.T) {
		testInternalCommandHelp(t, cliBinary)
	})

	// Test help for utility commands
	t.Run("UtilityCommands", func(t *testing.T) {
		testUtilityCommandHelp(t, cliBinary)
	})
}

// Helper functions for help menu tests
func testObjectCommandHelp(t *testing.T, cliBinary string, kinds []string) {
	commands := []string{"create", "get", "list", "update", "delete", "count", "fields", "bulk"}

	for _, cmd := range commands {
		t.Run(cmd, func(t *testing.T) {
			testCmd := exec.Command(cliBinary, "object", cmd, "--help")
			output, err := testCmd.CombinedOutput()
			if err != nil {
				t.Errorf("help failed for object %s: %v\nOutput: %s", cmd, err, string(output))
				return
			}

			// Verify help mentions the command purpose
			helpText := string(output)
			if len(helpText) < 50 {
				t.Errorf("help text too short for object %s", cmd)
			}

			// Verify --force flag is present for create and update commands
			if cmd == "create" || cmd == "update" {
				if !strings.Contains(helpText, "--force") {
					t.Errorf("help for object %s doesn't mention --force flag", cmd)
				}
			}

			// Verify --relaxed flag is present for create and update commands
			if cmd == "create" || cmd == "update" {
				if !strings.Contains(helpText, "--relaxed") {
					t.Errorf("help for object %s doesn't mention --relaxed flag", cmd)
				}
			}

			// For commands that take kind, verify examples mention object kinds
			if cmd != "bulk" && cmd != "fields" {
				hasKindExample := false
				for _, kind := range kinds[:minInt(3, len(kinds))] {
					if strings.Contains(helpText, kind) {
						hasKindExample = true
						break
					}
				}
				if !hasKindExample && len(kinds) > 0 {
					t.Logf("help for object %s doesn't mention specific kinds (may be intentional)", cmd)
				}
			}
		})
	}
}

func testBulkCommandHelp(t *testing.T, cliBinary string) {
	commands := []string{"create", "update", "get", "delete"}

	for _, cmd := range commands {
		t.Run(cmd, func(t *testing.T) {
			testCmd := exec.Command(cliBinary, "object", "bulk", cmd, "--help")
			output, err := testCmd.CombinedOutput()
			if err != nil {
				t.Errorf("help failed for bulk %s: %v\nOutput: %s", cmd, err, string(output))
				return
			}

			helpText := string(output)
			if len(helpText) < 50 {
				t.Errorf("help text too short for bulk %s", cmd)
			}

			// Verify help mentions bulk/atomic operations
			if !strings.Contains(strings.ToLower(helpText), "bulk") && !strings.Contains(strings.ToLower(helpText), "multiple") {
				t.Errorf("help for bulk %s doesn't mention bulk operations", cmd)
			}

			// Bulk create supports --force/--relaxed; bulk update uses --filter/--set only (see bulk_update.go).
			if cmd == "create" {
				if !strings.Contains(helpText, "--force") {
					t.Errorf("help for bulk %s doesn't mention --force flag", cmd)
				}
				if !strings.Contains(helpText, "--relaxed") {
					t.Errorf("help for bulk %s doesn't mention --relaxed flag", cmd)
				}
			}
		})
	}
}

func testSystemCommandHelp(t *testing.T, cliBinary string) {
	testCmd := exec.Command(cliBinary, "system", "check", "--help")
	output, err := testCmd.CombinedOutput()
	if err != nil {
		t.Errorf("help failed for system check: %v\nOutput: %s", err, string(output))
		return
	}

	helpText := string(output)
	if len(helpText) < 50 {
		t.Errorf("help text too short for system check")
	}
}

func testInternalCommandHelp(t *testing.T, cliBinary string) {
	commands := []string{"list", "get", "create", "update", "delete"}

	for _, cmd := range commands {
		t.Run(cmd, func(t *testing.T) {
			testCmd := exec.Command(cliBinary, "internal", cmd, "--help")
			output, err := testCmd.CombinedOutput()
			if err != nil {
				t.Errorf("help failed for internal %s: %v\nOutput: %s", cmd, err, string(output))
				return
			}

			helpText := string(output)
			if len(helpText) < 50 {
				t.Errorf("help text too short for internal %s", cmd)
			}

			// Verify help mentions admin/internal
			if !strings.Contains(strings.ToLower(helpText), "admin") && !strings.Contains(strings.ToLower(helpText), "internal") {
				t.Logf("help for internal %s doesn't explicitly mention admin/internal", cmd)
			}
		})
	}
}

func testUtilityCommandHelp(t *testing.T, cliBinary string) {
	commands := []string{"version", "migrate", "scenario-builder"}

	for _, cmd := range commands {
		t.Run(cmd, func(t *testing.T) {
			testCmd := exec.Command(cliBinary, "utility", cmd, "--help")
			output, err := testCmd.CombinedOutput()
			if err != nil {
				t.Errorf("help failed for utility %s: %v\nOutput: %s", cmd, err, string(output))
				return
			}

			helpText := string(output)
			if len(helpText) < 30 {
				t.Errorf("help text too short for utility %s", cmd)
			}

			// Verify --force flag is present for scenario-builder command
			if cmd == "scenario-builder" {
				if !strings.Contains(helpText, "--force") {
					t.Errorf("help for utility %s doesn't mention --force flag", cmd)
				}
			}
		})
	}
}
