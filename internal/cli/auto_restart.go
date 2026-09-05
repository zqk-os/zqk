package cli

import (
	"bufio"
	"os"
	"strings"

	"github.com/lanceman/zqk/pkg/execwrap"

	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

// Confirm prompts the user for a yes/no confirmation.
func Confirm(prompt string) bool {
	// If stdin is not a terminal, or we are in a test environment, we can't reliably prompt.
	if strings.HasSuffix(os.Args[0], ".test") {
		return false
	}
	os.Stderr.WriteString(prompt + " ")
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
