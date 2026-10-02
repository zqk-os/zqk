package cli

import (
	"bufio"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/pkg/execwrap"

	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// EvaluateCommandSchedulerState resolves the project root, checks scheduler running state, and queries the allow-degraded flag.
func EvaluateCommandSchedulerState(cmd *cobra.Command, resolveRoot func(string) string, checkRunning func(string) bool) (projectRoot string, running bool, allowDegraded bool) {
	root := resolveRoot(".")
	running = checkRunning(root)
	allowDegraded, _ = cmd.Flags().GetBool("allow-degraded")
	return root, running, allowDegraded
}

// Confirm prompts the user for a yes/no confirmation.
func Confirm(prompt string) bool {
	// If stdin is not a terminal, or we are in a test environment, we can't reliably prompt.
	if strings.HasSuffix(os.Args[0], ".test") {
		return false
	}
	_, _ = os.Stderr.WriteString(prompt + " ")
	reader := bufio.NewReader(os.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil {
		return false
	}
	response = strings.ToLower(strings.TrimSpace(response))
	return response == "y" || response == "yes"
}

// AutoRestartDaemon attempts to automatically start the ZQK scheduler in the background.
func AutoRestartDaemon(projectRoot string) error {
	exe, err := fileutil.Executable()
	if err != nil {
		return err
	}
	cmd := execwrap.Command(exe, "scheduler", "start", "--background")
	cmd.Dir = projectRoot
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
