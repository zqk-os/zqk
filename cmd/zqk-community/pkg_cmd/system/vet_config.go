package system

import (
	"fmt"
	"os"

	"github.com/fatih/color"
	"github.com/lanceman/zqk/internal/cli"
	clicontext "github.com/lanceman/zqk/internal/cli/context"
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/lanceman/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/spf13/cobra"
)

// vetConfigCmd represents the vet-config command
var vetConfigCmd *cobra.Command

// NewVetConfigCmd creates the vet-config command using the generated builder
func NewVetConfigCmd() *cobra.Command {
	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemVetConfigCommandBuilder(), &cobra.Command{Use: "vet-config"})
	cli.BindAsyncProgress(cmd, runVetConfig)
	vetConfigCmd = cmd
	return cmd
}

// runVetConfig executes the vet-config command
func runVetConfig(cmd *cobra.Command, args []string) error {
	projectRoot := cli.ResolveProjectRoot(".")
	if projectRoot == "" {
		return fmt.Errorf("project root not found")
	}

	issues := 0
	_ = cli.WriteOutput(cmd, []byte(color.New(color.FgCyan, color.Bold).Sprint("Vetting Configuration State...\n")))

	// 1. Check brand settings
	settingsPath := clicontext.BrandSettingsPath(projectRoot)
	if _, err := os.Stat(settingsPath); err != nil {
		_ = cli.WriteOutput(cmd, []byte(color.New(color.FgRed).Sprintf("✗ Missing critical brand settings file: %s\n", settingsPath)))
		issues++
	} else {
		if _, _, err := clicontext.LoadBrandSettingsFromFile(settingsPath); err != nil {
			_ = cli.WriteOutput(cmd, []byte(color.New(color.FgRed).Sprintf("✗ Invalid brand settings file: %s (%v)\n", settingsPath, err)))
			issues++
		} else {
			_ = cli.WriteOutput(cmd, []byte(color.New(color.FgGreen).Sprintf("✓ Brand settings valid: %s\n", settingsPath)))
		}
	}

	// Add other critical files as needed here.

	if issues > 0 {
		return fmt.Errorf("found %d configuration issues", issues)
	}

	_ = cli.WriteOutput(cmd, []byte(color.New(color.FgGreen, color.Bold).Sprint("\nConfiguration state is clean.\n")))
	return nil
}
