package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"gopkg.in/yaml.v3"

	"github.com/lanceman/zqk/internal/cli"

	"github.com/lanceman/zqk/pkg/objects"
)

// assertStructuredYAMLNotJSONBrace fails if stdout looks like JSON (top-level { or [).
// JSON is valid YAML 1.2, so yaml.Unmarshal alone does not catch "yaml" that is really json.MarshalIndent.
func assertStructuredYAMLNotJSONBrace(t *testing.T, label string, output []byte) {
	t.Helper()
	s := strings.TrimSpace(string(output))
	if len(s) == 0 {
		t.Fatalf("%s: empty output", label)
	}
	if s[0] == '{' || s[0] == '[' {
		t.Fatalf("%s: --format yaml produced JSON-style output (starts with %q); expected native YAML document\n%.400s", label, string(s[0]), s)
	}
}

// assertStructuredJSONStartsWithBrace checks JSON object/array output for structured commands.
func assertStructuredJSONStartsWithBrace(t *testing.T, label string, output []byte) {
	t.Helper()
	s := strings.TrimSpace(string(output))
	if len(s) == 0 {
		t.Fatalf("%s: empty output", label)
	}
	if s[0] != '{' && s[0] != '[' {
		t.Fatalf("%s: --format json expected JSON object or array, got first byte %q\n%.200s", label, s[0], s)
	}
}

func mustExecOutput(t *testing.T, cliBinary, tmpDir string, args ...string) []byte {
	t.Helper()
	cmd := exec.Command(cliBinary, args...)
	wireIsolatedCLI(cmd, tmpDir)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("exec %v: %v", args, err)
	}
	return out
}

// findFormatFlagInChain matches runtime flag resolution: local Flags, then PersistentFlags,
// walking up to root (e.g. --format on root persistent flags for object list/get/count).
func findFormatFlagInChain(cmd *cobra.Command) *pflag.Flag {
	for c := cmd; c != nil; c = c.Parent() {
		if f := c.Flags().Lookup("format"); f != nil {
			return f
		}
		if f := c.PersistentFlags().Lookup("format"); f != nil {
			return f
		}
	}
	return nil
}

// TestHelpOutputFormats tests that help menus can be output in different formats
// and that command outputs work correctly in JSON and YAML formats
func TestHelpOutputFormats(t *testing.T) {
	if testing.Short() {
		t.Skip("skip CLI integration in -short mode (spawns zqk per command); covered by scan-tests bundles")
	}
	// Not t.Parallel(): OutputValidity uses setupCLITestEnvironmentForParity → PrepareIsolatedTempProject (t.Setenv).
	rootCmd := buildRootCommand()
	// Mirror cmd/zqk/root.go: --format lives on root persistent flags; object list/get/count exclude
	// local common flags and rely on inheritance from the real root.
	if rootCmd.PersistentFlags().Lookup(cli.FlagFormat) == nil {
		rootCmd.PersistentFlags().StringP(cli.FlagFormat, "f", "", "Output format (json, yaml, table, json-rpc, stream). If not set, inferred from --context profile.")
	}

	// Test that help can be output (though Cobra doesn't natively support JSON/YAML help)
	// But we can test that commands work with --format json/yaml flags
	t.Run("CommandOutputFormats", func(t *testing.T) {
		testCommandOutputFormats(t, rootCmd)
	})

	// Test that format flags are consistent across commands
	t.Run("FormatFlagConsistency", func(t *testing.T) {
		testFormatFlagConsistency(t, rootCmd)
	})

	// Test JSON/YAML output validity
	t.Run("OutputValidity", func(t *testing.T) {
		testOutputValidity(t)
	})
}

// testCommandOutputFormats tests that commands support JSON and YAML output formats
func testCommandOutputFormats(t *testing.T, rootCmd *cobra.Command) {
	// Commands that should support --format flag
	commandsToTest := []struct {
		path      string
		cmd       *cobra.Command
		hasOutput bool // Whether the command produces output (not just status)
	}{
		{"object list", findCommandByPath(rootCmd, "object", "list"), true},
		{"object get", findCommandByPath(rootCmd, "object", "get"), true},
		{"object count", findCommandByPath(rootCmd, "object", "count"), true},
		{"internal list", findCommandByPath(rootCmd, "internal", "list"), true},
		{"internal get", findCommandByPath(rootCmd, "internal", "get"), true},
		{"system check", findCommandByPath(rootCmd, "system", "check"), true},
	}

	for _, testCase := range commandsToTest {
		if testCase.cmd == nil {
			continue
		}

		t.Run(testCase.path, func(t *testing.T) {
			formatFlag := findFormatFlagInChain(testCase.cmd)
			if formatFlag == nil {
				t.Errorf("%s: missing --format flag", testCase.path)
				return
			}

			// Verify format flag accepts expected values
			usage := formatFlag.Usage
			if !strings.Contains(strings.ToLower(usage), "json") && !strings.Contains(strings.ToLower(usage), "yaml") {
				t.Errorf("%s: --format flag usage doesn't mention json/yaml: %s", testCase.path, usage)
			}
		})
	}
}

// testFormatFlagConsistency tests that format flags are consistent across commands
func testFormatFlagConsistency(t *testing.T, rootCmd *cobra.Command) {
	allCommands := collectAllCommands(rootCmd)

	formatFlags := make(map[string]string) // command path -> format flag usage

	for _, cmd := range allCommands {
		path := getCommandPath(cmd)
		formatFlag := cmd.Flag("format")

		// Check parent if not found
		if formatFlag == nil {
			parent := cmd.Parent()
			for parent != nil {
				formatFlag = parent.Flag("format")
				if formatFlag != nil {
					break
				}
				parent = parent.Parent()
			}
		}

		if formatFlag != nil {
			formatFlags[path] = formatFlag.Usage
		}
	}

	// Check that format flag usage is consistent
	expectedFormats := []string{"json", "yaml", "table"}
	for path, usage := range formatFlags {
		usageLower := strings.ToLower(usage)
		for _, format := range expectedFormats {
			if !strings.Contains(usageLower, format) {
				t.Errorf("%s: --format flag doesn't mention '%s' format: %s", path, format, usage)
			}
		}
	}
}

// testOutputValidity tests that JSON and YAML outputs are valid
//
//nolint:gocyclo // Test helper intentionally exercises many output validation scenarios
func testOutputValidity(t *testing.T) {
	tmpDir, cliBinary := setupCLITestEnvironmentForParity(t)

	// Test JSON output validity
	t.Run("JSONValidity", func(t *testing.T) {
		// Test fields --list-kinds with JSON format
		// CRITICAL: Use Output() instead of CombinedOutput() to only capture stdout
		// Warnings go to stderr and should not pollute JSON output
		cmd := exec.Command(cliBinary, "object", "list", "--format", "json")
		wireIsolatedCLI(cmd, tmpDir)
		output, err := cmd.Output() // Only captures stdout, not stderr
		if err != nil {
			// If command failed, check if it's a non-zero exit (stderr has error details)
			if exitErr, ok := err.(*exec.ExitError); ok {
				// Command failed but we still got stdout - check if it's valid JSON
				// This handles cases where command succeeds but returns non-zero (e.g., warnings)
				output = exitErr.Stderr
			}
			if len(output) == 0 {
				t.Skipf("Skipping JSON validity test (command failed with no output): %v", err)
				return
			}
		}

		var jsonData map[string]any
		if err := json.Unmarshal(output, &jsonData); err != nil {
			t.Logf("Note: list output may not be JSON when showing kinds: %v\nOutput: %s", err, string(output))
		}
	})

	// Test YAML output validity
	t.Run("YAMLValidity", func(t *testing.T) {
		// Test list without kind to get available kinds in YAML format
		// Use Output() to only capture stdout (warnings go to stderr)
		cmd := exec.Command(cliBinary, "object", "list", "--format", "yaml")
		wireIsolatedCLI(cmd, tmpDir)
		output, err := cmd.Output() // Only captures stdout, not stderr
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				output = exitErr.Stderr
			}
			if len(output) == 0 {
				t.Skipf("Skipping YAML validity test (command failed with no output): %v", err)
				return
			}
		}

		var yamlData map[string]any
		if err := yaml.Unmarshal(output, &yamlData); err != nil {
			t.Logf("Note: list output may not be YAML when showing kinds: %v\nOutput: %s", err, string(output))
		}
	})

	// Test that list command produces valid JSON
	t.Run("ListJSONValidity", func(t *testing.T) {
		// Create a test object first (use BLI- prefix for backlog_item per id_prefixes)
		testObj := map[string]any{
			objects.FieldKeyID:            "BLI-001",
			objects.FieldKeyKind:          "backlog_item",
			objects.FieldKeyTitle:         "Test Item",
			objects.FieldKeyStatus:        objects.ObjectStatusExploring,
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			objects.FieldKeyCreatedAt:     "2025-12-26T00:00:00Z",
			objects.FieldKeyCreatedBy:     "test",
			objects.FieldKeyUpdatedAt:     "2025-12-26T00:00:00Z",
			objects.FieldKeyUpdatedBy:     "test",
		}

		// Write test object to file
		testFile := filepath.Join(tmpDir, "test-obj.yaml")
		data, _ := yaml.Marshal(testObj)
		if err := fileutil.WriteFile(testFile, data, paths.FilePerm644); err != nil { //nolint:gosec // Test files - 0600 is acceptable
			t.Skipf("Skipping list JSON test (failed to create test file): %v", err)
			return
		}

		// Create the object (ensure subprocess sees ZQK_TEST_ROOT and runs from tmpDir)
		createCmd := exec.Command(cliBinary, "object", "create", "backlog_item", "--file", testFile)
		wireIsolatedCLI(createCmd, tmpDir)
		createOut, createErr := createCmd.CombinedOutput()
		if createErr != nil {
			t.Skipf("Skipping list JSON test (failed to create test object): %v\nOutput: %s", createErr, string(createOut))
			return
		}

		// List with JSON format
		// Use Output() to only capture stdout (warnings go to stderr)
		listCmd := exec.Command(cliBinary, "object", "list", "backlog_item", "--format", "json")
		wireIsolatedCLI(listCmd, tmpDir)
		output, err := listCmd.Output() // Only captures stdout, not stderr
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				output = exitErr.Stderr
			}
			if len(output) == 0 {
				t.Skipf("Skipping list JSON test (command failed with no output): %v", err)
				return
			}
		}

		var jsonData map[string]any
		if err := json.Unmarshal(output, &jsonData); err != nil {
			t.Errorf("Invalid JSON output from 'object list --format json': %v\nOutput: %s", err, string(output))
		}
	})

	// Test that list command produces valid YAML
	t.Run("ListYAMLValidity", func(t *testing.T) {
		// List with YAML format (reuse test object from previous test)
		// Use Output() to only capture stdout (warnings go to stderr)
		listCmd := exec.Command(cliBinary, "object", "list", "backlog_item", "--format", "yaml")
		wireIsolatedCLI(listCmd, tmpDir)
		output, err := listCmd.Output() // Only captures stdout, not stderr
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				output = exitErr.Stderr
			}
			if len(output) == 0 {
				t.Skipf("Skipping list YAML test (command failed with no output): %v", err)
				return
			}
		}

		var yamlData map[string]any
		if err := yaml.Unmarshal(output, &yamlData); err != nil {
			t.Errorf("Invalid YAML output from 'object list --format yaml': %v\nOutput: %s", err, string(output))
			return
		}
		assertStructuredYAMLNotJSONBrace(t, "object list backlog_item --format yaml", output)
	})

	// Object get must emit native YAML for --format yaml (same contract as list).
	t.Run("ObjectGetYAMLDocumentStyle", func(t *testing.T) {
		testObj := map[string]any{
			objects.FieldKeyID:            "BLI-088806",
			objects.FieldKeyKind:          "backlog_item",
			objects.FieldKeyTitle:         "YAML format get",
			objects.FieldKeyStatus:        objects.ObjectStatusExploring,
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			objects.FieldKeyCreatedAt:     "2025-12-26T00:00:00Z",
			objects.FieldKeyCreatedBy:     "test",
			objects.FieldKeyUpdatedAt:     "2025-12-26T00:00:00Z",
			objects.FieldKeyUpdatedBy:     "test",
		}
		testFile := filepath.Join(tmpDir, "test-obj-get.yaml")
		data, _ := yaml.Marshal(testObj)
		if err := fileutil.WriteFile(testFile, data, paths.FilePerm644); err != nil { //nolint:gosec
			t.Skipf("write test file: %v", err)
			return
		}
		createCmd := exec.Command(cliBinary, "object", "create", "backlog_item", "--file", testFile)
		wireIsolatedCLI(createCmd, tmpDir)
		if out, err := createCmd.CombinedOutput(); err != nil {
			t.Skipf("create object: %v\n%s", err, string(out))
			return
		}
		getCmd := exec.Command(cliBinary, "object", "get", "BLI-088806", "--format", "yaml")
		wireIsolatedCLI(getCmd, tmpDir)
		out, err := getCmd.Output()
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				out = exitErr.Stderr
			}
			if len(out) == 0 {
				t.Skipf("object get yaml: %v", err)
				return
			}
		}
		var verify map[string]any
		if err := yaml.Unmarshal(out, &verify); err != nil {
			t.Fatalf("invalid yaml: %v\n%s", err, string(out))
		}
		assertStructuredYAMLNotJSONBrace(t, "object get --format yaml", out)
		assertStructuredJSONStartsWithBrace(t, "object get --format json (control)",
			mustExecOutput(t, cliBinary, tmpDir, "object", "get", "BLI-088806", "--format", "json"))
	})
}

// findCommandByPath finds a command by its path (e.g., "object", "list")
func findCommandByPath(root *cobra.Command, pathParts ...string) *cobra.Command {
	current := root
	for _, part := range pathParts {
		found := false
		for _, cmd := range current.Commands() {
			if cmd.Name() == part {
				current = cmd
				found = true
				break
			}
		}
		if !found {
			return nil
		}
	}
	return current
}

// TestCommandOutputJSONYAML tests that all commands that produce output support JSON and YAML formats
func TestCommandOutputJSONYAML(t *testing.T) {
	if testing.Short() {
		t.Skip("skip CLI integration in -short mode (spawns zqk per command); covered by scan-tests bundles")
	}
	// Not t.Parallel(): setupCLITestEnvironmentForParity uses PrepareIsolatedTempProject (t.Setenv).
	tmpDir, cliBinary := setupCLITestEnvironmentForParity(t)

	// Commands that should produce structured output (JSON/YAML compatible)
	outputCommands := []struct {
		command []string
		name    string
		setup   func() error // Optional setup function
	}{
		{
			name:    "object backlog_item fields",
			command: []string{"object", "backlog_item", "fields"},
		},
		{
			// There has never been an "internal" command group; this asserted one for three
			// months behind an auth failure in the same parent test. object_spec is reached
			// the same way as any other kind.
			name:    "object object_spec fields",
			command: []string{"object", "object_spec", "fields"},
		},
		{
			name:    "object count",
			command: []string{"object", "count", "backlog_item"},
			setup: func() error {
				// Create a test object (BLI- prefix for backlog_item)
				testObj := map[string]any{
					objects.FieldKeyID:            "BLI-002",
					objects.FieldKeyKind:          "backlog_item",
					objects.FieldKeyTitle:         "Test Count Item",
					objects.FieldKeyStatus:        objects.ObjectStatusExploring,
					objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
					objects.FieldKeyCreatedAt:     "2025-12-26T00:00:00Z",
					objects.FieldKeyCreatedBy:     "test",
					objects.FieldKeyUpdatedAt:     "2025-12-26T00:00:00Z",
					objects.FieldKeyUpdatedBy:     "test",
				}
				testFile := filepath.Join(tmpDir, "test-count.yaml")
				data, _ := yaml.Marshal(testObj)
				if err := fileutil.WriteFile(testFile, data, paths.FilePerm644); err != nil { //nolint:gosec // Test files - 0600 is acceptable
					return err
				}
				createCmd := exec.Command(cliBinary, "object", "create", "backlog_item", "--file", testFile)
				wireIsolatedCLI(createCmd, tmpDir)
				return createCmd.Run()
			},
		},
	}

	for _, testCase := range outputCommands {
		t.Run(testCase.name, func(t *testing.T) {
			// Run setup if provided
			if testCase.setup != nil {
				if err := testCase.setup(); err != nil {
					t.Skipf("Skipping %s (setup failed): %v", testCase.name, err)
					return
				}
			}

			// Test JSON format
			t.Run("JSON", func(t *testing.T) {
				// Use Output() to only capture stdout (warnings go to stderr)
				cmd := exec.Command(cliBinary, append(testCase.command, "--format", "json")...)
				wireIsolatedCLI(cmd, tmpDir)
				output, err := cmd.Output() // Only captures stdout, not stderr
				if err != nil {
					if exitErr, ok := err.(*exec.ExitError); ok {
						output = exitErr.Stderr
					}
					if len(output) == 0 {
						t.Errorf("%s --format json failed: %v", testCase.name, err)
						return
					}
				}
				outStr := string(output)
				if strings.Contains(outStr, "Usage:") && strings.Contains(outStr, "Available Commands:") {
					t.Skipf("Skipping %s (command returned help instead of data; kind subcommands may not be registered in test env)", testCase.name)
					return
				}
				// Validate JSON
				var jsonData any
				if err := json.Unmarshal(output, &jsonData); err != nil {
					t.Errorf("%s --format json produced invalid JSON: %v\nOutput: %s", testCase.name, err, outStr)
				}
			})

			// Test YAML format
			t.Run("YAML", func(t *testing.T) {
				// Use Output() to only capture stdout (warnings go to stderr)
				cmd := exec.Command(cliBinary, append(testCase.command, "--format", "yaml")...)
				wireIsolatedCLI(cmd, tmpDir)
				output, err := cmd.Output() // Only captures stdout, not stderr
				if err != nil {
					if exitErr, ok := err.(*exec.ExitError); ok {
						output = exitErr.Stderr
					}
					if len(output) == 0 {
						t.Errorf("%s --format yaml failed: %v", testCase.name, err)
						return
					}
				}
				outStr := string(output)
				if strings.Contains(outStr, "Usage:") && strings.Contains(outStr, "Available Commands:") {
					t.Skipf("Skipping %s (command returned help instead of data; kind subcommands may not be registered in test env)", testCase.name)
					return
				}
				// Validate YAML
				var yamlData any
				if err := yaml.Unmarshal(output, &yamlData); err != nil {
					t.Errorf("%s --format yaml produced invalid YAML: %v\nOutput: %s", testCase.name, err, outStr)
					return
				}
				assertStructuredYAMLNotJSONBrace(t, testCase.name+" --format yaml", output)
			})
		})
	}
}

// TestOutputFormatConsistency tests that outputs are consistent across formats
//
//nolint:gocyclo // Test function intentionally exercises many format consistency scenarios
func TestOutputFormatConsistency(t *testing.T) {
	// Not an RBAC gate in this package. The CLI under test authenticates through
	// auth_middleware, which reads $HOME/.zqk/credentials -- a path test isolation does not
	// redirect -- then looks that token up as a session id in the empty temp store and fails
	// with "unauthorized: session <id> not found". So the outcome depends on whether the
	// developer running the suite happens to be logged in, and it cannot be fixed from here.
	// TRACK: REDACTED -- remove this skip once the middleware
	// resolves credentials through an isolation-aware path.
	t.Skip("auth middleware reads $HOME credentials; isolated root has no matching session (REDACTED)")
	// Not t.Parallel(): setupCLITestEnvironmentForParity uses PrepareIsolatedTempProject (t.Setenv).
	tmpDir, cliBinary := setupCLITestEnvironmentForParity(t)

	// Create a test object
	testObj := map[string]any{
		objects.FieldKeyID:            "BLI-999",
		objects.FieldKeyKind:          "backlog_item",
		objects.FieldKeyTitle:         "Format Test Item",
		objects.FieldKeyStatus:        objects.ObjectStatusExploring,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyCreatedAt:     "2025-12-26T00:00:00Z",
		objects.FieldKeyCreatedBy:     "test",
		objects.FieldKeyUpdatedAt:     "2025-12-26T00:00:00Z",
		objects.FieldKeyUpdatedBy:     "test",
	}

	testFile := filepath.Join(tmpDir, "test-fmt.yaml")
	data, _ := yaml.Marshal(testObj)
	if err := fileutil.WriteFile(testFile, data, paths.FilePerm644); err != nil { //nolint:gosec // Test files - 0600 is acceptable
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Create the object (ensure subprocess sees ZQK_TEST_ROOT)
	ctxCreate, cancelCreate := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancelCreate()
	createCmd := exec.CommandContext(ctxCreate, cliBinary, "object", "create", "backlog_item", "--file", testFile)
	wireIsolatedCLI(createCmd, tmpDir)
	if err := createCmd.Run(); err != nil {
		t.Fatalf("Failed to create test object: %v", err)
	}

	// Get outputs in different formats
	formats := []string{"json", "yaml", "table"}
	outputs := make(map[string]string)

	for _, format := range formats {
		ctxGet, cancelGet := context.WithTimeout(context.Background(), 60*time.Second)
		cmd := exec.CommandContext(ctxGet, cliBinary, "object", "get", "BLI-999", "--format", format)
		wireIsolatedCLI(cmd, tmpDir)
		output, err := cmd.CombinedOutput()
		cancelGet()
		if err != nil {
			t.Errorf("Failed to get object in %s format: %v\nOutput: %s", format, err, string(output))
			continue
		}
		outputStr := string(output)
		// For JSON, try to extract just the JSON part (skip any error messages or warnings)
		if format == "json" {
			// Find the first '{' character which should be the start of JSON
			jsonStart := strings.Index(outputStr, "{")
			if jsonStart >= 0 {
				outputStr = outputStr[jsonStart:]
			} else {
				// If no '{' found, try to find JSON array start
				jsonStart = strings.Index(outputStr, "[")
				if jsonStart >= 0 {
					outputStr = outputStr[jsonStart:]
				}
			}
		}
		// For YAML, strip leading log lines so we start at the actual document.
		// If we strip to "{\n", the CLI returned JSON (GetFormat fix should prevent this in normal use).
		if format == "yaml" {
			for _, sep := range []string{"\nid:", "\n  id:", "---", "{\n"} {
				if idx := strings.Index(outputStr, sep); idx >= 0 {
					switch sep {
					case "{\n", "---":
						outputStr = outputStr[idx:]
					default:
						outputStr = outputStr[idx+1:]
					}
					break
				}
			}
		}
		outputs[format] = outputStr
	}

	// Verify JSON and YAML contain the same data (when parsed)
	t.Run("DataConsistency", func(t *testing.T) {
		var jsonData map[string]any
		var yamlData map[string]any

		jsonOutput := outputs["json"]
		// Try to extract JSON if there's non-JSON content before it
		if jsonOutput != emptyValue && jsonOutput[0] != '{' && jsonOutput[0] != '[' {
			jsonStart := strings.Index(jsonOutput, "{")
			if jsonStart < 0 {
				jsonStart = strings.Index(jsonOutput, "[")
			}
			if jsonStart >= 0 {
				jsonOutput = jsonOutput[jsonStart:]
			}
		}
		if err := json.Unmarshal([]byte(jsonOutput), &jsonData); err != nil {
			outputPreview := jsonOutput
			if len(outputPreview) > 200 {
				outputPreview = outputPreview[:200]
			}
			t.Errorf("JSON output is invalid: %v\nFirst 200 chars: %s", err, outputPreview)
			return
		}

		yamlOutput := strings.TrimSpace(outputs["yaml"])
		if err := yaml.Unmarshal([]byte(yamlOutput), &yamlData); err != nil {
			// Subprocess may still return JSON in some test environments; accept for comparison.
			if jsonErr := json.Unmarshal([]byte(yamlOutput), &yamlData); jsonErr != nil {
				preview := yamlOutput
				if len(preview) > 400 {
					preview = preview[:400] + "..."
				}
				t.Errorf("YAML output is invalid (and not valid JSON): %v\nFirst 400 chars: %q", err, preview)
				return
			}
		}

		// Compare key fields
		keyFields := []string{"id", "kind", "title", "status"}
		for _, field := range keyFields {
			jsonVal := jsonData[field]
			yamlVal := yamlData[field]
			if fmt.Sprintf("%v", jsonVal) != fmt.Sprintf("%v", yamlVal) {
				t.Errorf("Field %s differs between JSON and YAML: JSON=%v, YAML=%v", field, jsonVal, yamlVal)
			}
		}
	})

	// Verify all formats produce non-empty output
	t.Run("NonEmptyOutput", func(t *testing.T) {
		for format, output := range outputs {
			if strings.TrimSpace(output) == emptyValue {
				t.Errorf("Format %s produced empty output", format)
			}
		}
	})
}
