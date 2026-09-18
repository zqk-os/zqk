package utility

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/internal/cli"
	bldr_cli_cmd_v1 "github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// VersionInfo holds version information
type VersionInfo struct {
	Version   string
	BuildDate string
	GitCommit string
}

var versionInfo *VersionInfo

// SetVersionInfo sets the version information (called from main)
func SetVersionInfo(v, bd, gc string) {
	versionInfo = &VersionInfo{
		Version:   v,
		BuildDate: bd,
		GitCommit: gc,
	}
}

// getExecutableName returns the name of the current executable
func getExecutableName() string {
	execPath, err := fileutil.Executable()
	if err != nil {
		// Fallback to os.Args[0] if os.Executable() fails
		if len(os.Args) > 0 {
			return filepath.Base(os.Args[0])
		}
		return "zqk"
	}
	return filepath.Base(execPath)
}

// RunVersion prints executable and build metadata. Wired to root and utility version commands
// from `.zqk/cli/specs/root/version_command.yaml` and `utility/version_command.yaml`.
func RunVersion(cmd *cobra.Command, args []string) {
	execName := getExecutableName()
	if versionInfo != nil {
		msg := fmt.Sprintf("Executable: %s\nVersion: %s\nBuild datetime: %s\nCommit hash: %s\n", execName, versionInfo.Version, versionInfo.BuildDate, versionInfo.GitCommit)
		//nolint:errcheck // Output errors are non-critical
		_ = cli.WriteOutput(cmd, []byte(msg))
		return
	}
	msg := fmt.Sprintf("Executable: %s\nVersion: %s\n", execName, cmd.Root().Version)
	//nolint:errcheck // Output errors are non-critical
	_ = cli.WriteOutput(cmd, []byte(msg))
}

// NewRootVersionCmd returns the top-level `version` command (spec: root/version_command.yaml).
func NewRootVersionCmd() *cobra.Command {
	c := bldr_cli_cmd_v1.NewRootVersionCommandBuilder()
	c.Run = RunVersion
	return c
}

// NewVersionCmd creates the `utility version` subcommand (spec: utility/version_command.yaml).
func NewVersionCmd() *cobra.Command {
	c := bldr_cli_cmd_v1.NewUtilityVersionCommandBuilder()
	c.Run = RunVersion
	return c
}
