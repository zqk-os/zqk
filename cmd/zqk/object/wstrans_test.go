package object

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestWstransCommandsExist(t *testing.T) {
	t.Parallel()
	objectCmd := NewObjectCmd()

	var wstransCmd *cobra.Command
	for _, c := range objectCmd.Commands() {
		if strings.TrimSpace(strings.Split(c.Use, " ")[0]) == "wstrans" {
			wstransCmd = c
			break
		}
	}
	if wstransCmd == nil {
		t.Fatal("object wstrans command not found")
	}

	subs := map[string]bool{}
	for _, c := range wstransCmd.Commands() {
		subs[strings.TrimSpace(strings.Split(c.Use, " ")[0])] = true
	}
	for _, name := range []string{"show", "list"} {
		if !subs[name] {
			t.Errorf("object wstrans %s subcommand not found", name)
		}
	}
}

func TestWstransShowHasRunE(t *testing.T) {
	t.Parallel()
	cmd := NewWstransShowCmd()
	if cmd.RunE == nil {
		t.Fatal("wstrans show has no RunE")
	}
}

func TestWstransListHasRunE(t *testing.T) {
	t.Parallel()
	cmd := NewWstransListCmd()
	if cmd.RunE == nil {
		t.Fatal("wstrans list has no RunE")
	}
}
