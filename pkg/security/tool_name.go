package security

import (
	"fmt"
	"strings"
)

// ValidateToolName checks that an MCP or CLI tool name conforms to strict security
// constraints: non-empty, max 256 characters, and containing only ASCII alphanumeric,
// underscores, hyphens, dots, and colons.
func ValidateToolName(name string) error {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return fmt.Errorf("%w: %s", ErrMCPInvalidToolName, magicEmptyToolName)
	}

	if len(trimmed) > 256 {
		return fmt.Errorf("%w: %s", ErrMCPInvalidToolName, magicToolNameTooLong)
	}

	for i := 0; i < len(trimmed); i++ {
		c := trimmed[i]
		isAlphaNum := (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
		isAllowedSpecial := c == '_' || c == '-' || c == '.' || c == ':'
		if !isAlphaNum && !isAllowedSpecial {
			return fmt.Errorf("%w: %s (character %q)", ErrMCPInvalidToolName, magicToolNameBadCharset, string(c))
		}
	}

	return nil
}

// IsValidToolName returns true if the tool name passes ValidateToolName without error.
func IsValidToolName(name string) bool {
	return ValidateToolName(name) == nil
}
