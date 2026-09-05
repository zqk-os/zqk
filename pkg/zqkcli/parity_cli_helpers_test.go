package internal

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/utils/fileutil"
)

// Parity/help tests were copied into pkg/zqkcli with the internal→zqkcli move
// but their helpers stayed in cmd/zqk (*_test.go, package main). Keep local
// copies here so this package compiles. setupCLITestEnvironmentForParity
// reuses setupIsolatedCLITestProject (same temp-project + CLI binary layout).

func findProjectRootForParityTest(t *testing.T) string {
	t.Helper()
	return findProjectRootForDNATest(t)
}

func copyFile(src, dst string) error {
	data, err := fileutil.ReadFile(src)
	if err != nil {
		return err
	}
	return fileutil.WriteSecureFile(dst, data)
}

func copySpecFilesForParity(sourceDir, destDir string) error {
	if err := fileutil.EnsureDir(destDir); err != nil {
		return err
	}
	entries, err := fileutil.ReadDir(sourceDir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
			continue
		}
		if err := copyFile(filepath.Join(sourceDir, name), filepath.Join(destDir, name)); err != nil {
			return err
		}
	}
	return nil
}

func setupCLITestEnvironmentForParity(t *testing.T) (tmpDir, cliBinary string) {
	t.Helper()
	return setupIsolatedCLITestProject(t)
}

func collectAllCommands(cmd *cobra.Command) []*cobra.Command {
	commands := []*cobra.Command{cmd}
	for _, sub := range cmd.Commands() {
		commands = append(commands, collectAllCommands(sub)...)
	}
	return commands
}

func getCommandPath(cmd *cobra.Command) string {
	var parts []string
	for current := cmd; current != nil && current.Use != emptyValue; current = current.Parent() {
		parts = append([]string{current.Use}, parts...)
	}
	if len(parts) == 0 {
		return paths.CLICommandName
	}
	return strings.Join(parts, " ")
}

func buildRootCommand() *cobra.Command {
	cliCmd := paths.CLICommandName
	if cliCmd == emptyValue {
		cliCmd = paths.CLICommandNameDefault
	}
	rootCmd := &cobra.Command{
		Use:   cliCmd,
		Short: fmt.Sprintf("%s CLI", strings.ToUpper(cliCmd)),
	}
	rootCmd.AddCommand(NewInternalCmd())
	return rootCmd
}
