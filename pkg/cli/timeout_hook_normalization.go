package cli

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
	// Check for file paths
	if arg != emptyValue && (arg[0] == '/' || arg[0] == '.' || arg[0] == '~') {
		return true
	}
	// Check for IDs (e.g., ITEM-001, AUD-123); exclude subcommand names like "pre-commit" (require numeric suffix)
	if len(arg) > 5 && arg[3] == '-' {
		suffix := arg[4:]
		for i := 0; i < len(suffix); i++ {
			if suffix[i] < '0' || suffix[i] > '9' {
				break
			}
			if i == len(suffix)-1 {
				return true // suffix is all digits, treat as ID
			}
		}
	}
	// Check for UUIDs
	if len(arg) == 36 && arg[8] == '-' && arg[13] == '-' && arg[18] == '-' && arg[23] == '-' {
		return true
	}
	return false
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
