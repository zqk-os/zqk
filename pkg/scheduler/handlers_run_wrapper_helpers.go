package scheduler

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/lanceman/zqk/pkg/errfmt"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
	"github.com/lanceman/zqk/pkg/zqkenv"
)

// failureKindAndReason returns unambiguous failure_kind and failure_reason for log entries and notifications.
// Callers should use these so failures are never ambiguous (no guessing between timeout, exec error, or script exit).
func failureKindAndReason(exitCode int, stderr string, isTimeout bool, timeoutSeconds int) (kind, reason string) {
	if isTimeout {
		return "timeout", fmt.Sprintf("command timed out after %d seconds", timeoutSeconds)
	}
	if exitCode == 137 || strings.Contains(strings.ToLower(stderr), "out of memory") {
		detail := firstLine(stderr, 200)
		if detail == emptyValue {
			detail = "process was killed after exhausting its memory budget"
		}
		return "resource_exhausted", detail
	}
	if exitCode == 126 {
		detail := firstLine(stderr, 200)
		if detail == emptyValue {
			detail = "see stderr field"
		}
		return "shell_exec", fmt.Sprintf("shell could not execute (exit 126). Typical cause: shell binary passed as script. stderr: %s", detail)
	}
	detail := firstLine(stderr, 200)
	if detail == emptyValue {
		detail = "see stderr field"
	}
	return "script_exit", fmt.Sprintf("command exited with code %d. %s", exitCode, detail)
}

func firstLine(s string, maxLen int) string {
	s = strings.TrimSpace(s)
	if s == emptyValue {
		return ""
	}
	if idx := strings.Index(s, "\n"); idx >= 0 {
		s = s[:idx]
	}
	s = strings.TrimSpace(s)
	if len(s) > maxLen {
		s = s[:maxLen] + "..."
	}
	return s
}

// isShellWithInlineScript reports whether the job is stored as command=shell with an inline script.
// Two formats are accepted:
//   - args=[shellOrPath, "-c", script] (3 elements): run execwrap.Command(shell, "-c", script) using command as shell.
//   - args=["-c", script] (2 elements): run execwrap.Command(command, "-c", script). Common for jobs that set command="/bin/sh", command_args=["-c", "go test ... > file 2>&1"].
func isShellWithInlineScript(command string, args []string) bool {
	baseCmd := filepath.Base(command)
	if baseCmd != "sh" && baseCmd != "bash" && baseCmd != "dash" && baseCmd != "zsh" {
		return false
	}
	// Format: ["-c", script]
	if len(args) >= 2 && args[0] == "-c" {
		return true
	}
	// Format: [shellOrPath, "-c", script]
	if len(args) < 3 {
		return false
	}
	if args[1] != "-c" {
		return false
	}
	baseArg0 := filepath.Base(args[0])
	return args[0] == command || baseArg0 == baseCmd
}

// ensureEnvHasSchedulerTools gives run-wrapper jobs a deterministic baseline PATH.
// launchd/systemd commonly omit Homebrew and Go locations, which makes policy
// scripts fail on tools such as go or rg despite those tools being installed.
func ensureEnvHasSchedulerTools(env []string) []string {
	candidateDirs := []string{
		"/usr/local/go/bin",
		"/opt/homebrew/bin", // Homebrew on Apple Silicon
		"/usr/local/bin",
		filepath.Join(os.Getenv(zqkenv.POSIXHome()), "go", "bin"),
	}
	for _, tool := range []string{"go", "rg"} {
		if toolPath, err := exec.LookPath(tool); err == nil {
			candidateDirs = append(candidateDirs, filepath.Dir(toolPath))
		}
	}

	currentPath := os.Getenv(zqkenv.POSIXPath())
	prefix := "PATH="
	for _, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			currentPath = strings.TrimPrefix(entry, prefix)
			break
		}
	}
	pathParts := filepath.SplitList(currentPath)
	seen := make(map[string]struct{}, len(pathParts))
	for _, pathPart := range pathParts {
		seen[pathPart] = struct{}{}
	}
	var additions []string
	for _, candidate := range candidateDirs {
		if candidate == emptyValue {
			continue
		}
		if _, exists := seen[candidate]; exists {
			continue
		}
		if info, err := fileutil.Stat(candidate); err == nil && info.IsDir() {
			additions = append(additions, candidate)
			seen[candidate] = struct{}{}
		}
	}
	newPath := strings.Join(append(additions, pathParts...), string(fileutil.PathListSeparator))
	out := make([]string, 0, len(env)+1)
	replaced := false
	for _, e := range env {
		if strings.HasPrefix(e, prefix) {
			out = append(out, prefix+newPath)
			replaced = true
			continue
		}
		out = append(out, e)
	}
	if !replaced {
		out = append(out, prefix+newPath)
	}
	return out
}

// isTestCommand delegates to the configurable TestCommandDetector (loaded from
// .zqk/scheduler/test_command_rules.yaml or built-in defaults).
func (h *RunWrapperHandler) isTestCommand(command string, args []string) bool {
	if h.testCommandDetector != nil {
		return h.testCommandDetector.IsTestCommand(command, args)
	}
	// Fallback if detector was not set (e.g. in tests that construct handler manually)
	return DefaultTestCommandDetector().IsTestCommand(command, args)
}

// sanitizeTestOutput removes test framework output from command output
// This prevents test output pollution in logs (=== RUN, --- PASS, --- FAIL, etc.)
func (h *RunWrapperHandler) sanitizeTestOutput(output string) string {
	if output == emptyValue {
		return ""
	}

	lines := strings.Split(output, "\n")
	var filtered []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		// Filter out Go test framework output patterns
		// Check for test framework markers
		isTestFrameworkLine := strings.HasPrefix(trimmed, "=== RUN") ||
			strings.HasPrefix(trimmed, "--- PASS:") ||
			strings.HasPrefix(trimmed, "--- FAIL:") ||
			strings.HasPrefix(trimmed, "ok  \t") ||
			strings.HasPrefix(trimmed, "FAIL\t") ||
			strings.HasPrefix(trimmed, "?   \t") ||
			// Lines starting with spaces (indented test output) that contain test framework patterns
			(strings.HasPrefix(line, "    ") && (strings.Contains(trimmed, "_test.go:") ||
				strings.Contains(trimmed, "Expected") ||
				strings.Contains(trimmed, "WARNING:") ||
				strings.Contains(trimmed, "Loaded") ||
				strings.Contains(trimmed, "Total ") && strings.Contains(trimmed, " in storage"))) ||
			// Lines that are just test file paths with line numbers (even without leading spaces)
			(strings.Contains(trimmed, "_test.go:") && strings.Count(trimmed, ":") >= 2)

		if isTestFrameworkLine {
			// Skip test framework lines
			continue
		}
		// Keep other lines (actual test output, errors, etc.)
		filtered = append(filtered, line)
	}

	result := strings.Join(filtered, "\n")
	// If all lines were filtered, return empty string
	if strings.TrimSpace(result) == emptyValue {
		return ""
	}
	return result
}

// extractTestInfo extracts test name and package from test command arguments
// Examples:
//   - "go test ./..." -> ("", "")
//   - "go test ./pkg/storage -run TestName" -> ("TestName", "storage")
//   - "go test ./pkg/zqkcli -run TestBatchInternalUpdate" -> ("TestBatchInternalUpdate", "internal")
func (h *RunWrapperHandler) extractTestInfo(args []string) (testName, packageName string) {
	var packagePath = ""

	for i, arg := range args {
		if arg == "-run" && i+1 < len(args) {
			runIndex := i + 1
			testName = args[runIndex]
			// Remove any package qualifiers (e.g., "pkg/storage/TestName" -> "TestName")
			if strings.Contains(testName, "/") {
				parts := strings.Split(testName, "/")
				testName = parts[len(parts)-1]
			}
		}
		// Look for package path (e.g., "./pkg/storage", "./pkg/zqkcli")
		if strings.HasPrefix(arg, "./") && !strings.HasPrefix(arg, "./...") {
			packagePath = arg
		}
	}

	// Extract package name from path
	if packagePath != emptyValue {
		// Remove "./" prefix
		packagePath = strings.TrimPrefix(packagePath, "./")
		// Get last component (package name)
		parts := strings.Split(packagePath, "/")
		if len(parts) > 0 {
			packageName = parts[len(parts)-1]
		}
	}

	return testName, packageName
}

// isShellScriptPath reports whether command looks like a path to a shell script
// (e.g. ./scripts/foo.sh or /path/to/bar.sh). Used to decide whether to run sh -n before exec.
func isShellScriptPath(command string) bool {
	if command == emptyValue || strings.Contains(command, "\n") || strings.Contains(command, " ") {
		return false
	}
	lower := strings.ToLower(filepath.Base(command))
	return strings.HasSuffix(lower, ".sh") || strings.HasSuffix(lower, ".bash")
}

// resolveScriptPath resolves command to an absolute path using workingDir or projectRoot.
// Returns the absolute path if the file exists, otherwise error.
func resolveScriptPath(command, workingDir, projectRoot string) (string, error) {
	baseDir := workingDir
	if baseDir == emptyValue {
		baseDir = projectRoot
	}
	var absPath string
	if filepath.IsAbs(command) {
		absPath = command
	} else {
		if baseDir == emptyValue {
			return "", errfmt.Errorf("cannot resolve relative script path: no working directory or project root")
		}
		absPath = filepath.Join(baseDir, command)
	}
	_, err := fileutil.Stat(absPath)
	if err != nil {
		return "", err
	}
	return absPath, nil
}

// validateShellScriptSyntax runs "sh -n scriptPath" to check for syntax errors.
// dir is used as the command working dir (for relative paths in the script).
// Returns stderr and a non-nil error if the syntax check failed.
func validateShellScriptSyntax(ctx context.Context, executor CommandExecutor, scriptPath, dir string) (stderr string, err error) {
	if executor == nil {
		executor = &NativeExecutor{}
	}
	cmd := executor.CommandContext(ctx, "/bin/sh", "-n", scriptPath)
	if dir != emptyValue {
		cmd.SetDir(dir)
	}
	var buf strings.Builder
	cmd.SetStderr(&buf)
	runErr := cmd.Run()
	if runErr == nil {
		return "", nil
	}
	return buf.String(), runErr
}
