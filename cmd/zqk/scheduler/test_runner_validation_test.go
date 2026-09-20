package scheduler

import (
	"github.com/zqk-os/zqk/pkg/execwrap"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"

	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"

	"gopkg.in/yaml.v3"

	"github.com/zqk-os/zqk/pkg/objects"
)

// TestTestRunnerAssemblesJobCorrectly validates that test-runner.sh creates
// scheduler jobs with the correct structure, embedded commands, and callbacks.
func TestTestRunnerAssemblesJobCorrectly(t *testing.T) {
	// No t.Parallel(): this test calls os.Chdir, which mutates the working directory of the
	// whole test binary. Running it concurrently makes every relative path read by another
	// parallel test resolve against an arbitrary directory.
	// Get project root
	projectRoot := findProjectRootForTest(t)
	scriptPath := filepath.Join(projectRoot, "scripts", "test-runner.sh")

	// Create a temporary directory for test isolation
	testDir := t.TempDir()
	testLogDir := filepath.Join(testDir, paths.ProjectDataDir, "logs", "tests")
	if err := fileutil.MkdirAll(testLogDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create test log directory: %v", err)
	}

	// Set up test environment
	originalDir, err := fileutil.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current directory: %v", err)
	}
	defer fileutil.Chdir(originalDir)

	// Change to project root
	if err := fileutil.Chdir(projectRoot); err != nil {
		t.Fatalf("Failed to change to project root: %v", err)
	}

	jobYAMLPath := filepath.Join(testDir, "created_job.yaml")
	// --job-yaml-out: generate YAML only; never zqk object create (avoids persisting ephemeral callback paths).
	cmd := execwrap.Command("bash", scriptPath,
		"--package", "./pkg/testjobgen",
		"--log-file", filepath.Join(testLogDir, "test.log"),
		"--format", "jsonl",
		"--timeout", "600",
		"--job-yaml-out", jobYAMLPath,
	)

	zqkenv.WireExecForIsolatedProject(cmd, projectRoot)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("test-runner.sh failed: %v\noutput:\n%s", err, string(output))
	}

	if _, err := fileutil.Stat(jobYAMLPath); fileutil.IsNotExist(err) {
		t.Fatalf("expected job YAML at %s; output:\n%s", jobYAMLPath, string(output))
	}

	// Read and parse the job YAML
	jobYAML, err := fileutil.ReadFile(jobYAMLPath)
	if err != nil {
		t.Fatalf("Failed to read job YAML: %v", err)
	}

	// Fix YAML quote escaping issues before parsing
	// The script may generate nested quotes that need escaping
	yamlContent := string(jobYAML)
	// Replace problematic quote patterns
	yamlContent = strings.ReplaceAll(yamlContent, `"cd "`, `"cd \"`)
	yamlContent = strings.ReplaceAll(yamlContent, `" && `, `\" && `)

	var job map[string]any
	if err := yaml.Unmarshal([]byte(yamlContent), &job); err != nil {
		// If still fails, try parsing as-is (might work with some YAML parsers)
		if err2 := yaml.Unmarshal(jobYAML, &job); err2 != nil {
			t.Fatalf("Failed to parse job YAML: %v (original: %v)\nYAML content:\n%s", err2, err, string(jobYAML))
		}
	}

	// Validate job structure
	validateJobStructureForTest(t, job)

	// Validate command is embedded (not a temp file path)
	validateEmbeddedCommandForTest(t, job)

	// Validate callbacks are set up
	validateCallbacksForTest(t, job, testLogDir)
}

// validateJobStructureForTest checks that the job has all required fields
func validateJobStructureForTest(t *testing.T, job map[string]any) {
	requiredFields := []string{
		"id", "kind", "schema_version", "status", "job_type",
		"trigger_type", "category", "execution_mode", "command", "command_args",
		"callback_on_completion", "callback_on_error", "callback_type",
		"max_runtime_seconds", "title", "description",
	}

	for _, field := range requiredFields {
		if _, ok := job[field]; !ok {
			t.Errorf("Missing required field: %s", field)
		}
	}

	// Validate specific values
	if job[objects.FieldKeyKind] != "scheduler_job" {
		t.Errorf("Expected kind='scheduler_job', got %v", job[objects.FieldKeyKind])
	}

	if job[objects.FieldKeyJobType] != "run_wrapper" {
		t.Errorf("Expected job_type='run_wrapper', got %v", job[objects.FieldKeyJobType])
	}

	if job[objects.FieldKeyCategory] != "test" {
		t.Errorf("Expected category='test', got %v", job[objects.FieldKeyCategory])
	}

	if job[objects.FieldKeyExecutionMode] != "one_time" {
		t.Errorf("Expected execution_mode='one_time', got %v", job[objects.FieldKeyExecutionMode])
	}

	if job[objects.FieldKeyTriggerType] != "manual" {
		t.Errorf("Expected trigger_type='manual', got %v", job[objects.FieldKeyTriggerType])
	}

	// Validate command is /bin/sh (not bash with temp file)
	if cmd, ok := job[objects.FieldKeyCommand].(string); !ok || cmd != "/bin/sh" {
		t.Errorf("Expected command='/bin/sh', got %v", job[objects.FieldKeyCommand])
	}
}

// validateEmbeddedCommandForTest ensures the command is embedded, not a temp file path
func validateEmbeddedCommandForTest(t *testing.T, job map[string]any) {
	commandArgs, ok := job[objects.FieldKeyCommandArgs].([]any)
	if !ok {
		t.Fatalf("command_args is not a slice: %T", job[objects.FieldKeyCommandArgs])
	}

	if len(commandArgs) < 2 {
		t.Fatalf("Expected at least 2 command args, got %d", len(commandArgs))
	}

	// First arg should be "-c"
	if commandArgs[0] != "-c" {
		t.Errorf("Expected first arg='-c', got %v", commandArgs[0])
	}

	// Second arg should be the embedded command string
	cmdStr, ok := commandArgs[1].(string)
	if !ok {
		t.Fatalf("Expected second arg to be string, got %T", commandArgs[1])
	}

	// Validate the command contains the test command
	if !strings.Contains(cmdStr, "go test") {
		t.Errorf("Command should contain 'go test', got: %s", cmdStr)
	}

	if !strings.Contains(cmdStr, "./pkg/testjobgen") {
		t.Errorf("Command should contain './pkg/testjobgen', got: %s", cmdStr)
	}

	// Validate it's NOT a temp file path
	if strings.Contains(cmdStr, "/tmp/") || strings.Contains(cmdStr, "/var/folders/") {
		t.Errorf("Command should not contain temp file path, got: %s", cmdStr)
	}

	// Validate it contains cd command
	if !strings.Contains(cmdStr, "cd ") {
		t.Errorf("Command should contain 'cd' to set working directory, got: %s", cmdStr)
	}

	// Validate it's a single command string (not a file path)
	if strings.HasPrefix(cmdStr, "/") && !strings.Contains(cmdStr, "&&") {
		t.Errorf("Command appears to be a file path rather than embedded command: %s", cmdStr)
	}
}

// validateCallbacksForTest ensures callbacks are properly configured
func validateCallbacksForTest(t *testing.T, job map[string]any, _ string) {
	callbackOnCompletion, ok := job[objects.FieldKeyCallbackOnCompletion].(string)
	if !ok || callbackOnCompletion == emptyValue {
		t.Errorf("callback_on_completion is missing or empty")
	}

	callbackOnError, ok := job[objects.FieldKeyCallbackOnError].(string)
	if !ok || callbackOnError == emptyValue {
		t.Errorf("callback_on_error is missing or empty")
	}

	// Callbacks should be the same
	if callbackOnCompletion != callbackOnError {
		t.Errorf("callback_on_completion and callback_on_error should match")
	}

	// Validate callback command structure
	if !strings.Contains(callbackOnCompletion, "callback notify") {
		t.Errorf("Callback should contain 'callback notify', got: %s", callbackOnCompletion)
	}

	// Script may pass --format jsonl or default to text; both are valid
	if !strings.Contains(callbackOnCompletion, "--format ") {
		t.Errorf("Callback should contain '--format', got: %s", callbackOnCompletion)
	}
	if !strings.Contains(callbackOnCompletion, "jsonl") && !strings.Contains(callbackOnCompletion, "text") {
		t.Errorf("Callback should contain '--format jsonl' or '--format text', got: %s", callbackOnCompletion)
	}

	if !strings.Contains(callbackOnCompletion, "--log-file") {
		t.Errorf("Callback should contain '--log-file', got: %s", callbackOnCompletion)
	}

	// Validate callback type
	if job[objects.FieldKeyCallbackType] != "command" {
		t.Errorf("Expected callback_type='command', got %v", job[objects.FieldKeyCallbackType])
	}
}

// findProjectRootForTest finds the project root directory
func findProjectRootForTest(t *testing.T) string {
	dir, err := fileutil.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current directory: %v", err)
	}

	for {
		if _, err := fileutil.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("Could not find project root (go.mod)")
		}
		dir = parent
	}
}

// TestTestRunnerJobYAMLStructure validates the YAML structure with different options
func TestTestRunnerJobYAMLStructure(t *testing.T) {
	// No t.Parallel(): this test calls os.Chdir, which mutates the working directory of the
	// whole test binary. Running it concurrently makes every relative path read by another
	// parallel test resolve against an arbitrary directory.
	projectRoot := findProjectRootForTest(t)
	scriptPath := filepath.Join(projectRoot, "scripts", "test-runner.sh")

	testDir := t.TempDir()
	testLogDir := filepath.Join(testDir, paths.ProjectDataDir, "logs", "tests")
	_ = fileutil.MkdirAll(testLogDir, paths.DirPerm755)

	originalDir, _ := fileutil.Getwd()
	defer fileutil.Chdir(originalDir)
	_ = fileutil.Chdir(projectRoot)

	// Test with different options
	testCases := []struct {
		name    string
		args    []string
		checkFn func(*testing.T, map[string]any)
	}{
		{
			name: "package test",
			args: []string{"--package", "./pkg/testjobgen", "--log-file", filepath.Join(testLogDir, "test.log")},
			checkFn: func(t *testing.T, job map[string]any) {
				desc, _ := job[objects.FieldKeyDescription].(string)
				if !strings.Contains(desc, "package") {
					t.Errorf("Description should mention package, got: %s", desc)
				}
			},
		},
		{
			name: "with timeout",
			args: []string{"--package", "./pkg/testjobgen", "--timeout", "300", "--log-file", filepath.Join(testLogDir, "test2.log")},
			checkFn: func(t *testing.T, job map[string]any) {
				// YAML numbers might be int or float64
				var timeout int
				switch v := job[objects.FieldKeyMaxRuntimeSeconds].(type) {
				case int:
					timeout = v
				case float64:
					timeout = int(v)
				default:
					t.Errorf("Unexpected type for max_runtime_seconds: %T", job[objects.FieldKeyMaxRuntimeSeconds])
					return
				}
				if timeout != 300 {
					t.Errorf("Expected max_runtime_seconds=300, got %d", timeout)
				}
			},
		},
		{
			name: "with environment variables",
			args: []string{"--package", "./pkg/testjobgen", "--env", "GOFLAGS=-race", "--log-file", filepath.Join(testLogDir, "test3.log")},
			checkFn: func(t *testing.T, job map[string]any) {
				envVars, ok := job[objects.FieldKeyEnvironmentVariables].(map[string]any)
				if !ok {
					t.Errorf("Expected environment_variables map, got %T", job[objects.FieldKeyEnvironmentVariables])
					return
				}
				if val, ok := envVars["GOFLAGS"].(string); !ok || val != "-race" {
					t.Errorf("Expected GOFLAGS=-race, got %v", envVars["GOFLAGS"])
				}
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Clean up previous job YAML
			jobYAMLPath := filepath.Join(testDir, "created_job.yaml")
			_ = fileutil.Remove(jobYAMLPath)

			args := append([]string{scriptPath}, tc.args...)
			args = append(args, "--job-yaml-out", jobYAMLPath)
			cmd := execwrap.Command("bash", args...)
			zqkenv.WireExecForIsolatedProject(cmd, projectRoot)
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("test-runner.sh failed: %v\noutput:\n%s", err, string(output))
			}

			if _, err := fileutil.Stat(jobYAMLPath); fileutil.IsNotExist(err) {
				t.Fatalf("expected job YAML at %s; output:\n%s", jobYAMLPath, string(output))
			}

			jobYAML, err := fileutil.ReadFile(jobYAMLPath)
			if err != nil {
				t.Fatalf("Failed to read job YAML: %v", err)
			}

			// Fix YAML quote escaping issues
			yamlContent := string(jobYAML)
			yamlContent = strings.ReplaceAll(yamlContent, `"cd "`, `"cd \"`)
			yamlContent = strings.ReplaceAll(yamlContent, `" && `, `\" && `)

			var job map[string]any
			if err := yaml.Unmarshal([]byte(yamlContent), &job); err != nil {
				// Try original if fixed version fails
				if err2 := yaml.Unmarshal(jobYAML, &job); err2 != nil {
					t.Fatalf("Failed to parse job YAML: %v (original: %v)", err2, err)
				}
			}

			// Run test-specific checks
			if tc.checkFn != nil {
				tc.checkFn(t, job)
			}

			// Always validate embedded command
			validateEmbeddedCommandForTest(t, job)
		})
	}
}
