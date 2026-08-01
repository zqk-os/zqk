package object

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestObjectCommandAliases(t *testing.T) {
	t.Parallel()
	objectCmd := NewObjectCmd()

	expectAliases := map[string][]string{
		"get":    {"show", "view"},
		"list":   {"ls", "find"},
		"create": {"add", "new"},
		"update": {"edit"},
		"delete": {"rm", "remove"},
	}
	for use, want := range expectAliases {
		var cmd *cobra.Command
		for _, c := range objectCmd.Commands() {
			if strings.Split(c.Use, " ")[0] == use {
				cmd = c
				break
			}
		}
		if cmd == nil {
			t.Errorf("command %q not found", use)
			continue
		}
		if len(cmd.Aliases) != len(want) {
			t.Errorf("command %q: aliases = %v, want %v", use, cmd.Aliases, want)
			continue
		}
		seen := make(map[string]bool)
		for _, a := range cmd.Aliases {
			seen[a] = true
		}
		for _, a := range want {
			if !seen[a] {
				t.Errorf("command %q: missing alias %q, have %v", use, a, cmd.Aliases)
			}
		}
	}
}
