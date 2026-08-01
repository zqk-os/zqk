package cli

import "fmt"

const DryRunPrefix = "[DRY RUN]"

// DryRunMessage prefixes a message for dry-run output.
func DryRunMessage(msg string) string {
	return DryRunPrefix + " " + msg
}

// DryRunWould returns a standardized "Would ..." dry-run message.
func DryRunWould(action string) string {
	return DryRunMessage("Would " + action)
}

// DryRunWouldRun returns a standardized dry-run command preview message.
func DryRunWouldRun(command string) string {
	return DryRunMessage("Would run: " + command)
}

// DryRunWouldRunf is a fmt.Sprintf variant for DryRunWouldRun.
func DryRunWouldRunf(format string, args ...any) string {
	return DryRunWouldRun(fmt.Sprintf(format, args...))
}
