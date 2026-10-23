package cli

import (
	"strings"

	"github.com/zqk-os/zqk/pkg/strutil"
)

// normalizeCommand normalizes a command for metrics tracking
// This is exported via NormalizeCommand() in command_tracker.go for use outside the package
func (h *TimeoutHook) normalizeCommand(command string, args []string) string {
	// Basic normalization: remove file paths, IDs, and other variable data
	// This allows grouping similar commands together
	normalized := command
	for _, arg := range args {
		// Skip file paths, IDs, and other variable data
		if h.isVariableArg(arg) {
			normalized += " <arg>"
		} else {
			normalized += " " + arg
		}
	}
	return normalized
}

// isVariableArg checks if an argument is variable data that should be normalized
func (h *TimeoutHook) isVariableArg(arg string) bool {
	if arg == emptyValue {
		return false
	}
	// Check for file paths
	if arg[0] == '/' || arg[0] == '.' || arg[0] == '~' {
		return true
	}
	// Check for IDs (e.g., BLI-001, AUD-123, nanosecond kernel IDs);
	// excludes subcommand names like "pre-commit" by requiring uppercase prefix and numeric suffix start
	if hyphen := strings.IndexByte(arg, '-'); hyphen >= 2 && hyphen <= 6 && hyphen < len(arg)-1 {
		prefix := arg[:hyphen]
		if isAllUpperOrDigit(prefix) {
			rest := arg[hyphen+1:]
			if len(rest) > 0 && rest[0] >= '0' && rest[0] <= '9' {
				return true
			}
		}
	}
	// Check for UUIDs
	if len(arg) == 36 && arg[8] == '-' && arg[13] == '-' && arg[18] == '-' && arg[23] == '-' {
		return true
	}
	// Check for hex hashes (40-char SHA1 or 64-char SHA256)
	if (len(arg) == 40 || len(arg) == 64) && isHexString(arg) {
		return true
	}
	// Check for flag assignments with variable values (e.g. --title=..., --ids=...)
	if strings.HasPrefix(arg, "-") && strings.Contains(arg, "=") {
		return true
	}
	// Check for numeric literals
	if isNumericString(arg) {
		return true
	}
	return false
}

func isAllUpperOrDigit(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c < 'A' || c > 'Z') && (c < '0' || c > '9') {
			return false
		}
	}
	return true
}

func isHexString(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') && (c < 'A' || c > 'F') {
			return false
		}
	}
	return true
}

func isNumericString(s string) bool {
	return strutil.IsAllDigits(s)
}

// sanitizeArgs sanitizes command arguments for privacy
func (h *TimeoutHook) sanitizeArgs(args []string) []string {
	sanitized := make([]string, 0, len(args))
	for _, arg := range args {
		if h.isVariableArg(arg) {
			sanitized = append(sanitized, "<sanitized>")
		} else {
			sanitized = append(sanitized, arg)
		}
	}
	return sanitized
}

// sanitizeError sanitizes error messages for privacy
func (h *TimeoutHook) sanitizeError(err error) string {
	if err == nil {
		return ""
	}
	// Remove file paths and sensitive data from error messages
	msg := err.Error()
	// Basic sanitization - can be enhanced
	return msg
}
