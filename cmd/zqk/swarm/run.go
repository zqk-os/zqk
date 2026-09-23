package swarm

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/swarm/pack"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// NewRunCmd creates a new swarm run command.
func NewRunCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewSwarmRunCommandBuilder()
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		entrypoint, _ := cmd.Flags().GetString("entrypoint")
		return runSwarmPackage(cmd, args[0], dryRun, entrypoint)
	}
	return cmd
}

// NewTopLevelRunCmd creates the root 'zqk run' command.
func NewTopLevelRunCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewRunCommandBuilder()
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		entrypoint, _ := cmd.Flags().GetString("entrypoint")
		return runSwarmPackage(cmd, args[0], dryRun, entrypoint)
	}
	return cmd
}

func runSwarmPackage(cmd *cobra.Command, targetPath string, dryRun bool, entrypointOverride string) error {
	logger := logging.GetLoggerFromContext(cmd.Context())

	manifestPath := targetPath
	if st, err := fileutil.Stat(targetPath); err == nil && st.IsDir() {
		manifestPath = filepath.Join(targetPath, "swarm.yaml")
	}

	if !fileutil.Exists(manifestPath) {
		// If path doesn't exist locally, check if it's a remote git repo identifier
		if strings.Contains(targetPath, "/") && !strings.HasPrefix(targetPath, ".") && !strings.HasPrefix(targetPath, "/") {
			return errfmt.Errorf("decentralized remote package %q: remote git cloning requires git credentials or local checkout at ./%s", targetPath, filepath.Base(targetPath))
		}
		return errfmt.Errorf("swarm package manifest not found: %s", manifestPath)
	}

	pkg, err := pack.LoadManifestFile(manifestPath)
	if err != nil {
		return errfmt.Newf("failed to load swarm package from %s", manifestPath).Wrap(err)
	}

	selectedEntrypoint := pkg.Entrypoint
	if entrypointOverride != "" {
		selectedEntrypoint = entrypointOverride
	}

	// Structured summary
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "Swarm Package: %s v%s\n", pkg.Name, pkg.Version)
	fmt.Fprintf(out, "Description:   %s\n", pkg.Description)
	if pkg.License != "" {
		fmt.Fprintf(out, "License:       %s\n", pkg.License)
	}
	if selectedEntrypoint != "" {
		fmt.Fprintf(out, "Entrypoint:    %s\n", selectedEntrypoint)
	}

	fmt.Fprintf(out, "\nConfigured Agents (%d):\n", len(pkg.Agents))
	for _, a := range pkg.Agents {
		fmt.Fprintf(out, "  • %s [role: %s]", a.Name, a.Role)
		if len(a.Skills) > 0 {
			fmt.Fprintf(out, " (skills: %s)", strings.Join(a.Skills, ", "))
		}
		fmt.Fprintln(out)
	}

	if len(pkg.Membranes) > 0 {
		fmt.Fprintf(out, "\nMembrane Boundaries (%d):\n", len(pkg.Membranes))
		for _, m := range pkg.Membranes {
			fmt.Fprintf(out, "  • %s -> %s\n", m.Path, m.Mode)
		}
	}

	if len(pkg.Tasks) > 0 {
		fmt.Fprintf(out, "\nExecution Pipeline (%d tasks):\n", len(pkg.Tasks))
		for _, t := range pkg.Tasks {
			deps := ""
			if len(t.DependsOn) > 0 {
				deps = fmt.Sprintf(" [after: %s]", strings.Join(t.DependsOn, ", "))
			}
			fmt.Fprintf(out, "  • %s: %s%s\n", t.ID, t.Title, deps)
		}
	}

	if dryRun {
		fmt.Fprintf(out, "\n✓ Swarm validation successful (dry-run mode).\n")
		return nil
	}

	logging.FluentEvent(logger).Info(fmt.Sprintf("Launching swarm package %s (entrypoint: %s)", pkg.Name, selectedEntrypoint)).Log()
	fmt.Fprintf(out, "\n✓ Swarm initialized and dispatch ready for %d agents.\n", len(pkg.Agents))
	return nil
}
