package bldr_cli_cmd_v1_test

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
)

const (
	testExpectedNonNilResult     = "expected non-nil cobra.Command from NewWorkflowCheckCommandBuilder"
	testExpectedCheckName        = "check"
	testExpectedSubOfWorkflowMsg = "expected check to be a sub-command of workflow"
	testHasShortDescription      = "expected non-empty short description on check command"
	testZeroArgsSucceeded        = "expected zero arguments to succeed"
)

func TestNewWorkflowCheckCommandBuilder_ReturnsNonNil(t *testing.T) {
	cmd := bldr_cli_cmd_v1.NewWorkflowCheckCommandBuilder()
	if cmd == nil {
		t.Fatal(testExpectedNonNilResult)
	}
}

func TestNewWorkflowCheckCommandBuilder_HasName(t *testing.T) {
	cmd := bldr_cli_cmd_v1.NewWorkflowCheckCommandBuilder()
	if got := cmd.Name(); got != testExpectedCheckName {
		t.Errorf("expected command name %q, got %q", testExpectedCheckName, got)
	}
}

func TestNewWorkflowCheckCommandBuilder_HasShortDescription(t *testing.T) {
	cmd := bldr_cli_cmd_v1.NewWorkflowCheckCommandBuilder()
	if cmd.Short == "" {
		t.Error(testHasShortDescription)
	}
}

func TestNewWorkflowCheckCommandBuilder_AcceptsZeroArgs(t *testing.T) {
	cmd := bldr_cli_cmd_v1.NewWorkflowCheckCommandBuilder()
	if err := cmd.RunE(cmd, []string{}); err != nil {
		t.Fatalf("%s: %v", testZeroArgsSucceeded, err)
	}
}
