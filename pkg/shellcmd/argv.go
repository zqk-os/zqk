// Package shellcmd turns a configured command string into an argv for execution.
//
// Scheduler callbacks and router actions are authored as shell one-liners:
// redirection, && chains, and quoted arguments that contain spaces. Splitting
// such a string on whitespace passes those tokens to the program as literal
// arguments, so `tee -a log.jsonl >/dev/null && zqk feed steer -m "a b c"`
// makes tee create one file per word in the working directory.
package shellcmd

import (
	"os"
	"runtime"
	"strings"
)

const (
	// ShellPathPOSIX is the default POSIX shell used for command strings.
	ShellPathPOSIX = "/bin/sh"
	// ShellFlagCPOSIX tells the POSIX shell to run the following operand as a command string.
	ShellFlagCPOSIX = "-c"

	// ShellPathWindows is the fallback Windows command processor.
	ShellPathWindows = "cmd.exe"
	// ShellFlagCWindows tells cmd.exe to run the following command string and terminate.
	ShellFlagCWindows = "/c"
)

// ShellPath and ShellFlagC are initialized dynamically based on runtime.GOOS and environment.
var (
	ShellPath  = ShellPathPOSIX
	ShellFlagC = ShellFlagCPOSIX
)

func init() {
	ShellPath, ShellFlagC = ResolveShell(runtime.GOOS, os.Getenv("COMSPEC"))
}

// ResolveShell returns the shell executable and flag for the specified operating system and COMSPEC.
func ResolveShell(goos, comspec string) (string, string) {
	if goos == "windows" {
		if comspec != "" {
			return comspec, ShellFlagCWindows
		}
		return ShellPathWindows, ShellFlagCWindows
	}
	return ShellPathPOSIX, ShellFlagCPOSIX
}

// shellMetaChars are the characters a shell interprets: quoting, redirection,
// pipelines, chaining, substitution, globbing, and comments (POSIX and Windows).
const shellMetaChars = "|&;<>()$`\\\"'*?[#\n%^"

// NeedsShell reports whether command uses syntax that only a shell can honor.
func NeedsShell(command string) bool {
	return strings.ContainsAny(command, shellMetaChars)
}

// Argv returns the argv to execute for command: a shell invocation when the
// string carries shell syntax, otherwise its whitespace-separated words.
// A blank command yields nil so callers can fail closed.
func Argv(command string) []string {
	if strings.TrimSpace(command) == "" {
		return nil
	}
	if NeedsShell(command) {
		return []string{ShellPath, ShellFlagC, command}
	}
	return strings.Fields(command)
}

