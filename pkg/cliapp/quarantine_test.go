package cli

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestCreateQuarantinedCommand(t *testing.T) {
	cmd := CreateQuarantinedCommand(QuarantinedCommand{
		Name:        "futuristic-feature",
		Description: "A feature from the future",
		Reason:      "Dependencies pending",
		PlannedFor:  "v2.0.0",
	})

	if cmd == nil {
		t.Fatal("expected non-nil command")
	}
	if cmd.Use != "futuristic-feature" {
		t.Errorf("expected Use 'futuristic-feature', got %s", cmd.Use)
	}
	if !strings.Contains(cmd.Long, "v2.0.0") {
		t.Errorf("expected Long description to mention planned version")
	}

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "planned for v2.0.0") {
		t.Errorf("expected execution error mentioning planned version, got: %v", err)
	}

	root := &cobra.Command{Use: "root"}
	RegisterQuarantinedCommands(root)

	// Non-empty quarantined commands list
	orig := QuarantinedCommands
	defer func() { QuarantinedCommands = orig }()
	QuarantinedCommands = []QuarantinedCommand{
		{
			Name:        "experimental-tool",
			Description: "Experimental CLI tool",
			Reason:      "Work in progress",
			PlannedFor:  "v1.5.0",
		},
	}
	RegisterQuarantinedCommands(root)
	if cmd, _, err := root.Find([]string{"experimental-tool"}); err != nil || cmd == nil {
		t.Errorf("expected registered experimental-tool in root")
	}
}
