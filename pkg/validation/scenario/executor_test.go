package scenario

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/logging"
)

func TestExecutor_Run(t *testing.T) {
	logger := logging.NewLogger(os.Stdout, logging.DebugLevel, logging.NewTextFormatter(context.Background()))
	executor := NewExecutor(logger)

	tmpFile := filepath.Join(t.TempDir(), "test_target.txt")

	s := Scenario{
		Name: "Test Scenario",
		Steps: []Step{
			{
				Name:    "Write File",
				Type:    StepTypeCommand,
				Command: "sh",
				Args:    []string{"-c", "echo 'hello world' > " + tmpFile},
			},
			{
				Name:   "Check File Exists",
				Type:   StepTypeExists,
				Target: tmpFile,
			},
			{
				Name:    "Read File",
				Type:    StepTypeCommand,
				Command: "cat",
				Args:    []string{tmpFile},
			},
			{
				Name:    "Verify Output",
				Type:    StepTypeRegex,
				Pattern: "hello world",
			},
			{
				Name:        "Expect Failure",
				Type:        StepTypeCommand,
				Command:     "cat",
				Args:        []string{"/does/not/exist"},
				ExpectError: true,
			},
		},
	}

	res := executor.Run(context.Background(), s)

	if !res.Success {
		t.Errorf("Scenario failed: %+v", res)
		for _, step := range res.StepResults {
			if !step.Success {
				t.Errorf("Step %s failed: %s", step.StepName, step.Error)
			}
		}
	}

	if len(res.StepResults) != 5 {
		t.Errorf("Expected 5 step results, got %d", len(res.StepResults))
	}
}

func TestExecutor_Run_Failure(t *testing.T) {
	logger := logging.NewLogger(os.Stdout, logging.DebugLevel, logging.NewTextFormatter(context.Background()))
	executor := NewExecutor(logger)

	s := Scenario{
		Name: "Failing Scenario",
		Steps: []Step{
			{
				Name:    "This should fail",
				Type:    StepTypeCommand,
				Command: "cat",
				Args:    []string{"/does/not/exist/missing"},
			},
			{
				Name:    "Should not execute",
				Type:    StepTypeCommand,
				Command: "echo",
				Args:    []string{"skipped"},
			},
		},
	}

	res := executor.Run(context.Background(), s)

	if res.Success {
		t.Errorf("Expected scenario to fail")
	}

	if len(res.StepResults) != 1 {
		t.Errorf("Expected scenario to short-circuit after 1 step, ran %d steps", len(res.StepResults))
	}

	if res.StepResults[0].Success {
		t.Errorf("Expected first step to fail")
	}
}
