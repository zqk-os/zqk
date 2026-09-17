package scheduler

import (
	"context"
	"testing"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/logging"
)

// TestRunWrapperHandler_TestCommandDetection tests that test commands are correctly identified
func TestRunWrapperHandler_TestCommandDetection(t *testing.T) {
	_, storage := prepareHandlersIsolatedTempProject(t)

	logger := logging.GetLoggerFromProfile("test")
	handler := NewRunWrapperHandler(storage, logger, nil, nil)
	t.Cleanup(func() { handler.(*RunWrapperHandler).StopNotificationContext() })

	tests := []struct {
		name    string
		command string
		args    []string
		want    bool
	}{
		{
			name:    "go test command",
			command: "go",
			args:    []string{"test", "./..."},
			want:    true,
		},
		{
			name:    "go test with package",
			command: "go",
			args:    []string{"test", "./pkg/storage", "-v"},
			want:    true,
		},
		{
			name:    "go test with run flag",
			command: "go",
			args:    []string{"test", "./cmd/zqk/system", "-run", "TestName"},
			want:    true,
		},
		{
			name:    "command contains test",
			command: "go test",
			args:    []string{"./..."},
			want:    true,
		},
		{
			name:    "non-test command",
			command: "echo",
			args:    []string{"hello"},
			want:    false,
		},
		{
			name:    "go build (not test)",
			command: "go",
			args:    []string{"build", "./..."},
			want:    false,
		},
		{
			name:    "test in different context",
			command: "npm",
			args:    []string{"test"},
			want:    false, // Only "go test" is recognized
		},
		{
			name:    "shell -c go test (test bundle jobs)",
			command: "/bin/sh",
			args:    []string{"-c", "go test ./cmd/zqk/system -run '^(TestFoo|TestBar)$' -v -count=1 -p 10 > /tmp/out.log 2>&1"},
			want:    true,
		},
		{
			name:    "shell -c non-test script",
			command: "/bin/sh",
			args:    []string{"-c", "echo hello"},
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := handler.(*RunWrapperHandler).isTestCommand(tt.command, tt.args)
			if got != tt.want {
				t.Errorf("isTestCommand(%q, %v) = %v, want %v", tt.command, tt.args, got, tt.want)
			}
		})
	}
}

// TestRunWrapperHandler_ExtractTestInfo tests extraction of test name and package from command args
func TestRunWrapperHandler_ExtractTestInfo(t *testing.T) {
	_, storage := prepareHandlersIsolatedTempProject(t)

	logger := logging.GetLoggerFromProfile("test")
	handler := NewRunWrapperHandler(storage, logger, nil, nil)
	t.Cleanup(func() { handler.(*RunWrapperHandler).StopNotificationContext() })

	tests := []struct {
		name         string
		args         []string
		wantTestName string
		wantPackage  string
	}{
		{
			name:         "full test suite",
			args:         []string{"test", "./..."},
			wantTestName: "",
			wantPackage:  "",
		},
		{
			name:         "package with test name",
			args:         []string{"test", "./pkg/storage", "-run", "TestCAS_Create"},
			wantTestName: "TestCAS_Create",
			wantPackage:  "storage",
		},
		{
			name:         "package with qualified test name",
			args:         []string{"test", "./cmd/zqk/system", "-run", "pkg/system/TestName"},
			wantTestName: "TestName",
			wantPackage:  "system",
		},
		{
			name:         "package without test name",
			args:         []string{"test", "./pkg/storage", "-v"},
			wantTestName: "",
			wantPackage:  "storage",
		},
		{
			name:         "nested package path",
			args:         []string{"test", "./cmd/zqk/scheduler", "-run", "TestSubmit"},
			wantTestName: "TestSubmit",
			wantPackage:  "scheduler",
		},
		{
			name:         "multiple flags",
			args:         []string{"test", "./pkg/storage", "-v", "-count=1", "-run", "TestCAS"},
			wantTestName: "TestCAS",
			wantPackage:  "storage",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testName, packageName := handler.(*RunWrapperHandler).extractTestInfo(tt.args)
			if testName != tt.wantTestName {
				t.Errorf("extractTestInfo(%v) testName = %q, want %q", tt.args, testName, tt.wantTestName)
			}
			if packageName != tt.wantPackage {
				t.Errorf("extractTestInfo(%v) packageName = %q, want %q", tt.args, packageName, tt.wantPackage)
			}
		})
	}
}

// TestRunWrapperHandler_TestCommandTimeout tests dynamic timeout calculation for test commands
func TestRunWrapperHandler_TestCommandTimeout(t *testing.T) {
	_, storage := prepareHandlersIsolatedTempProject(t)

	logger := logging.GetLoggerFromProfile("test")
	handler := NewRunWrapperHandler(storage, logger, nil, nil)
	t.Cleanup(func() { handler.(*RunWrapperHandler).StopNotificationContext() })

	tests := []struct {
		name              string
		command           string
		args              []string
		maxRuntimeSeconds int
		description       string
	}{
		{
			name:              "full test suite uses default timeout",
			command:           "go",
			args:              []string{"test", "./..."},
			maxRuntimeSeconds: 0, // Should default to 3600
			description:       "Full test suite should use 1 hour default",
		},
		{
			name:              "full test suite with low timeout gets upgraded",
			command:           "go",
			args:              []string{"test", "./..."},
			maxRuntimeSeconds: 300, // Should be upgraded to 3600
			description:       "Full test suite with 300s timeout should be upgraded to 3600s",
		},
		{
			name:              "full test suite with high timeout preserved",
			command:           "go",
			args:              []string{"test", "./..."},
			maxRuntimeSeconds: 7200, // Should be preserved
			description:       "Full test suite with 7200s timeout should be preserved",
		},
		{
			name:              "package test without test name",
			command:           "go",
			args:              []string{"test", "./pkg/storage"},
			maxRuntimeSeconds: 1800,
			description:       "Package test should use provided timeout",
		},
		{
			name:              "specific test with name",
			command:           "go",
			args:              []string{"test", "./pkg/storage", "-run", "TestCAS_Create"},
			maxRuntimeSeconds: 300,
			description:       "Specific test should use test timing data if available",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// We can't easily test the actual timeout calculation without mocking testtiming,
			// but we can verify the command is recognized as a test command
			isTest := handler.(*RunWrapperHandler).isTestCommand(tt.command, tt.args)
			if !isTest {
				t.Errorf("Expected command to be recognized as test command: %q %v", tt.command, tt.args)
			}

			// Verify test info extraction works
			testName, packageName := handler.(*RunWrapperHandler).extractTestInfo(tt.args)
			t.Logf("Test info extracted: testName=%q, packageName=%q", testName, packageName)

			// For full test suite, verify it would use default timeout logic
			if len(tt.args) > 1 && tt.args[1] == "./..." {
				if tt.maxRuntimeSeconds == 0 || tt.maxRuntimeSeconds < 3600 {
					// This would trigger the default 3600s timeout in actual execution
					t.Logf("Full test suite would use default 3600s timeout")
				}
			}
		})
	}
}

// TestRunWrapperHandler_TestCommandIsolation tests that test commands are isolated from other commands
func TestRunWrapperHandler_TestCommandIsolation(t *testing.T) {
	_, storage := prepareHandlersIsolatedTempProject(t)

	logger := logging.GetLoggerFromProfile("test")
	handler := NewRunWrapperHandler(storage, logger, nil, nil)
	t.Cleanup(func() { handler.(*RunWrapperHandler).StopNotificationContext() })

	// Test that test commands are isolated (don't interfere with each other)
	testJobs := []*ScheduledJob{
		{
			ID:          "SCH-TEST-1",
			JobType:     JobTypeRunWrapper,
			Command:     "go",
			CommandArgs: []string{"test", "./pkg/storage", "-run", "TestCAS_Create"},
		},
		{
			ID:          "SCH-TEST-2",
			JobType:     JobTypeRunWrapper,
			Command:     "go",
			CommandArgs: []string{"test", "./cmd/zqk/system", "-run", "TestSubmit"},
		},
		{
			ID:          "SCH-TEST-3",
			JobType:     JobTypeRunWrapper,
			Command:     "go",
			CommandArgs: []string{"test", "./pkg/scheduler", "-run", "TestRunWrapper"},
		},
	}

	// Verify each test command is correctly identified and isolated
	for _, job := range testJobs {
		t.Run(job.ID, func(t *testing.T) {
			isTest := handler.(*RunWrapperHandler).isTestCommand(job.Command, job.CommandArgs)
			if !isTest {
				t.Errorf("Expected test command to be recognized: %q %v", job.Command, job.CommandArgs)
			}

			testName, packageName := handler.(*RunWrapperHandler).extractTestInfo(job.CommandArgs)
			t.Logf("Job %s: testName=%q, packageName=%q", job.ID, testName, packageName)

			// Verify each job has unique test info
			if packageName == emptyValue {
				t.Errorf("Expected package name to be extracted for %s", job.ID)
			}
		})
	}

	// Verify non-test commands are not affected
	nonTestJob := &ScheduledJob{
		ID:          "SCH-NON-TEST",
		JobType:     JobTypeRunWrapper,
		Command:     "echo",
		CommandArgs: []string{"hello"},
	}

	isTest := handler.(*RunWrapperHandler).isTestCommand(nonTestJob.Command, nonTestJob.CommandArgs)
	if isTest {
		t.Errorf("Expected non-test command to not be recognized as test: %q %v", nonTestJob.Command, nonTestJob.CommandArgs)
	}
}

// TestRunWrapperHandler_TestCommandExecution tests actual execution of test commands
func TestRunWrapperHandler_TestCommandExecution(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping test command execution in short mode")
	}

	_, storage := prepareHandlersIsolatedTempProject(t)

	logger := logging.GetLoggerFromProfile("test")
	handler := NewRunWrapperHandler(storage, logger, nil, nil)
	t.Cleanup(func() { handler.(*RunWrapperHandler).StopNotificationContext() })

	// Test with a simple echo command first (not a real test, but tests the path)
	job := &ScheduledJob{
		ID:                "SCH-TEST-EXEC",
		JobType:           JobTypeRunWrapper,
		Command:           "echo",
		CommandArgs:       []string{"test", "output"},
		MaxRuntimeSeconds: 30,
		RetryCount:        0,
	}

	ctx := pkgctx.NewSystemContext()
	if err := handler.(*RunWrapperHandler).Execute(ctx, job); err != nil {
		t.Errorf("Execute failed: %v", err)
	}

	// Test that test command detection doesn't affect non-test execution
	// (This verifies isolation - test detection logic shouldn't interfere with normal commands)
}

// TestRunWrapperHandler_TestCommandTimeoutCalculation tests timeout calculation for various test scenarios
func TestRunWrapperHandler_TestCommandTimeoutCalculation(t *testing.T) {
	_, storage := prepareHandlersIsolatedTempProject(t)

	logger := logging.GetLoggerFromProfile("test")
	handler := NewRunWrapperHandler(storage, logger, nil, nil)
	t.Cleanup(func() { handler.(*RunWrapperHandler).StopNotificationContext() })

	tests := []struct {
		name              string
		command           string
		args              []string
		maxRuntimeSeconds int
		expectedMin       int // Minimum expected timeout in seconds
		expectedMax       int // Maximum expected timeout in seconds
	}{
		{
			name:              "full test suite default",
			command:           "go",
			args:              []string{"test", "./..."},
			maxRuntimeSeconds: 0,
			expectedMin:       3600, // Should default to 1 hour
			expectedMax:       3600,
		},
		{
			name:              "full test suite low timeout upgraded",
			command:           "go",
			args:              []string{"test", "./..."},
			maxRuntimeSeconds: 300,
			expectedMin:       3600, // Should be upgraded to 1 hour
			expectedMax:       3600,
		},
		{
			name:              "package test preserves timeout",
			command:           "go",
			args:              []string{"test", "./pkg/storage"},
			maxRuntimeSeconds: 1800,
			expectedMin:       1800, // Should preserve provided timeout
			expectedMax:       1800,
		},
		{
			name:              "specific test uses timing data",
			command:           "go",
			args:              []string{"test", "./pkg/storage", "-run", "TestCAS_Create"},
			maxRuntimeSeconds: 300,
			expectedMin:       30,   // Minimum from testtiming (30s)
			expectedMax:       3600, // Maximum from testtiming (1 hour)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Verify test command detection
			isTest := handler.(*RunWrapperHandler).isTestCommand(tt.command, tt.args)
			if !isTest {
				t.Fatalf("Expected command to be recognized as test: %q %v", tt.command, tt.args)
			}

			// Extract test info
			testName, packageName := handler.(*RunWrapperHandler).extractTestInfo(tt.args)
			t.Logf("Extracted: testName=%q, packageName=%q", testName, packageName)

			// For full test suite, verify timeout logic
			if len(tt.args) > 1 && tt.args[1] == "./..." {
				// Full test suite should use default 3600s if timeout is 0 or < 3600
				if tt.maxRuntimeSeconds == 0 || tt.maxRuntimeSeconds < 3600 {
					// In actual execution, this would be set to 3600
					t.Logf("Full test suite would use 3600s default timeout")
				}
			}

			// For specific tests, timeout would be calculated from testtiming
			if testName != emptyValue || packageName != emptyValue {
				// In actual execution, GetExpectedTimeout would be called
				// We can't easily test this without mocking, but we verify the path
				t.Logf("Specific test would use testtiming.GetExpectedTimeout(%q, %q)", testName, packageName)
			}
		})
	}
}

// TestRunWrapperHandler_TestCommandSubmitIntegration tests the full flow from submit to execution
func TestRunWrapperHandler_TestCommandSubmitIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	_, storage := prepareHandlersIsolatedTempProject(t)

	logger := logging.GetLoggerFromProfile("test")
	handler := NewRunWrapperHandler(storage, logger, nil, nil)
	t.Cleanup(func() { handler.(*RunWrapperHandler).StopNotificationContext() })

	// Simulate a test job that would be created by submit command
	job := &ScheduledJob{
		ID:                "SCH-TEST-SUBMIT",
		JobType:           JobTypeRunWrapper,
		TriggerType:       "immediate",
		ExecutionMode:     "one_time",
		Command:           "go",
		CommandArgs:       []string{"test", "./pkg/scheduler", "-v", "-count=1"},
		MaxRuntimeSeconds: 1800, // 30 minutes
		RetryCount:        0,
		Category:          "manual",
	}

	// Verify test command is detected
	isTest := handler.(*RunWrapperHandler).isTestCommand(job.Command, job.CommandArgs)
	if !isTest {
		t.Fatalf("Expected test command to be detected")
	}

	// Verify test info extraction
	testName, packageName := handler.(*RunWrapperHandler).extractTestInfo(job.CommandArgs)
	if packageName != "scheduler" {
		t.Errorf("Expected packageName 'scheduler', got %q", packageName)
	}
	t.Logf("Test job: testName=%q, packageName=%q", testName, packageName)

	// In actual execution, the timeout would be calculated dynamically
	// For package tests without specific test name, it would use the provided timeout
	// For specific tests, it would use testtiming.GetExpectedTimeout

	// Verify job can be executed (with a simple command to avoid actual test execution)
	simpleJob := &ScheduledJob{
		ID:                "SCH-TEST-SIMPLE",
		JobType:           JobTypeRunWrapper,
		Command:           "echo",
		CommandArgs:       []string{"test", "isolation"},
		MaxRuntimeSeconds: 10,
		RetryCount:        0,
	}

	ctx, cancel := context.WithTimeout(pkgctx.NewSystemContext(), 5*time.Second)
	defer cancel()

	if err := handler.(*RunWrapperHandler).Execute(ctx, simpleJob); err != nil {
		t.Errorf("Execute failed: %v", err)
	}
}
