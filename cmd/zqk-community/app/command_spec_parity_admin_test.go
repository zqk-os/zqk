package app

import (
	"strings"

	"github.com/spf13/cobra"
)

// excludeAdminGroupCommands drops paths under GroupID=admin commands.
// The community binary strips those after registration; *.test binaries keep them for unit tests.
func excludeAdminGroupCommands(root *cobra.Command, paths []string) []string {
	adminRoots := map[string]struct{}{}
	for _, cmd := range root.Commands() {
		if cmd.GroupID == "admin" {
			adminRoots[cmd.Name()] = struct{}{}
		}
	}
	if len(adminRoots) == 0 {
		return paths
	}
	filtered := make([]string, 0, len(paths))
	for _, path := range paths {
		rootName, _, _ := strings.Cut(path, " ")
		if _, ok := adminRoots[rootName]; ok {
			continue
		}
		filtered = append(filtered, path)
	}
	return filtered
}
