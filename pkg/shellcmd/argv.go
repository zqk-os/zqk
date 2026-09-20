// Package shellcmd turns a configured command string into an argv for execution.
//
// Scheduler callbacks and router actions are authored as shell one-liners:
// redirection, && chains, and quoted arguments that contain spaces. Splitting
// such a string on whitespace passes those tokens to the program as literal
// arguments, so `tee -a log.jsonl >/dev/null && zqk feed steer -m "a b c"`
// makes tee create one file per word in the working directory.
package shellcmd

import "strings"

const (
	// ShellPath is the POSIX shell used for command strings that carry shell syntax.
	ShellPath = "/bin/sh"
	// ShellFlagC tells the shell to run the following operand as a command string.
	ShellFlagC = "-c"
)

// shellMetaChars are the characters a shell interprets: quoting, redirection,
// pipelines, chaining, substitution, globbing, and comments.
const shellMetaChars = "|&;<>()$`\\\"'*?[#\n"

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
