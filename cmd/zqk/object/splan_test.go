package object

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// TestSPlanCommandsExist verifies the splan command group and subcommands are registered.
func TestSPlanCommandsExist(t *testing.T) {
	t.Parallel()
	objectCmd := NewObjectCmd()

	var splanCmd *cobra.Command
	for _, c := range objectCmd.Commands() {
		if strings.TrimSpace(strings.Split(c.Use, " ")[0]) == "splan" {
			splanCmd = c
			break
		}
	}
	if splanCmd == nil {
		t.Fatal("object splan command not found")
	}

	subs := map[string]bool{}
	for _, c := range splanCmd.Commands() {
		subs[strings.TrimSpace(strings.Split(c.Use, " ")[0])] = true
	}
	for _, name := range []string{"show", "list"} {
		if !subs[name] {
			t.Errorf("object splan %s subcommand not found", name)
		}
	}
}

// TestSPlanShowHasRunE verifies splan show is wired with a RunE handler.
func TestSPlanShowHasRunE(t *testing.T) {
	t.Parallel()
	cmd := NewSPlanShowCmd()
	if cmd.RunE == nil {
		t.Fatal("splan show has no RunE")
	}
}

// TestSPlanListHasRunE verifies splan list is wired with a RunE handler.
func TestSPlanListHasRunE(t *testing.T) {
	t.Parallel()
	cmd := NewSPlanListCmd()
	if cmd.RunE == nil {
		t.Fatal("splan list has no RunE")
	}
}
