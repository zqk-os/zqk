package object

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestEvomanCommandsExist(t *testing.T) {
	t.Parallel()
	objectCmd := NewObjectCmd()

	var evomanCmd *cobra.Command
	for _, c := range objectCmd.Commands() {
		if strings.TrimSpace(strings.Split(c.Use, " ")[0]) == "evoman" {
			evomanCmd = c
			break
		}
	}
	if evomanCmd == nil {
		t.Fatal("object evoman command not found")
	}

	subs := map[string]bool{}
	for _, c := range evomanCmd.Commands() {
		subs[strings.TrimSpace(strings.Split(c.Use, " ")[0])] = true
	}
	for _, name := range []string{"show", "list"} {
		if !subs[name] {
			t.Errorf("object evoman %s subcommand not found", name)
		}
	}
}

func TestEvomanShowHasRunE(t *testing.T) {
	t.Parallel()
	cmd := NewEvomanShowCmd()
	if cmd.RunE == nil {
		t.Fatal("evoman show has no RunE")
	}
}

func TestEvomanListHasRunE(t *testing.T) {
	t.Parallel()
	cmd := NewEvomanListCmd()
	if cmd.RunE == nil {
		t.Fatal("evoman list has no RunE")
	}
}
