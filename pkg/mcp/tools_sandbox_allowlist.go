package mcp

import (
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/lanceman/zqk/pkg/brand"
)

// sandboxDenyPrefix marks allowlist rejections. It deliberately avoids the "access denied"
// wording used by the kernel-path guard so callers can tell the two refusals apart.
const sandboxDenyPrefix = "ALLOWLIST DENY"

// sandboxAllowedExecutableList is the fail-closed execution allowlist for the agent sandbox
// (REQ-CEF-SEC-003 / CRIT-CEF-SEC-003A). Stored as a slice (not a map literal) so FieldKey
// AST gates do not treat executable names like "date" as ontology field keys.
// CEF evidence showed that high-risk token matching alone is bypassable through interpreters,
// encoders, and shell re-entry, so anything absent from this set is refused. Shells,
// interpreters, and network fetchers are intentionally missing.
var sandboxAllowedExecutableList = []string{
	// Read-only inspection of the workspace.
	"cat", "head", "tail", "less", "wc", "grep", "rg",
	"ls", "find", "file", "stat", "diff", "sort", "uniq",
	"cut", "jq", "tree",
	// Trivial output and path helpers used by task scaffolding.
	"echo", "printf", "true", "false", "pwd", "date",
	"basename", "dirname", "realpath",
	// Build, test, and version-control tooling. Destructive git verbs stay gated by the
	// high-risk staging path in isHighRiskBashCommand.
	"go", "gofmt", "make", "git", "golangci-lint",
}

// shellSubstitutionPatterns hide a second command inside an otherwise allowed one, which would
// let the allowlist inspect only the wrapper. They are refused wherever they appear.
var shellSubstitutionPatterns = []string{"$(", "`", "<(", ">("}

// shellSegmentOperators separate the individual commands a shell would run from one string.
const shellSegmentOperators = "|&;\n"

var envAssignmentPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=`)

// checkSandboxAllowlist reports whether every command in a bash string is permitted. Compound
// commands are decomposed so that a permitted head (echo, cat) cannot smuggle a denied tail.
func checkSandboxAllowlist(command string) error {
	for _, pattern := range shellSubstitutionPatterns {
		if strings.Contains(command, pattern) {
			return fmt.Errorf("%s: command substitution %q is not permitted in the agent sandbox", sandboxDenyPrefix, pattern)
		}
	}

	for _, segment := range splitShellSegments(command) {
		executable := extractBaseExecutable(segment)
		if executable == "" {
			continue
		}
		if !slices.Contains(sandboxAllowedExecutableList, filepath.Base(executable)) {
			return fmt.Errorf("%s: %q is not in the agent sandbox allowlist; use a dedicated %s MCP tool instead",
				sandboxDenyPrefix, executable, brand.ExecutableName())
		}
	}

	return nil
}

// splitShellSegments splits a command on shell control operators, honoring quotes so that an
// operator inside a string literal (grep 'a|b') does not produce a phantom segment.
func splitShellSegments(command string) []string {
	var (
		segments []string
		current  strings.Builder
		quote    rune
	)

	for _, r := range command {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			}
			current.WriteRune(r)
		case r == '\'' || r == '"':
			quote = r
			current.WriteRune(r)
		case strings.ContainsRune(shellSegmentOperators, r):
			segments = append(segments, current.String())
			current.Reset()
		default:
			current.WriteRune(r)
		}
	}

	return append(segments, current.String())
}

// parseCommandTokens splits a command segment into tokens, honoring quotes and whitespace.
func parseCommandTokens(segment string) []string {
	var tokens []string
	var current strings.Builder
	var quote rune

	for _, r := range segment {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				current.WriteRune(r)
			}
		case r == '\'' || r == '"':
			quote = r
		case r == ' ' || r == '\t' || r == '\n' || r == '\r':
			if current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}
		default:
			current.WriteRune(r)
		}
	}
	if current.Len() > 0 {
		tokens = append(tokens, current.String())
	}

	// Filter out leading env assignments (e.g. GOFLAGS=-mod=mod)
	var filtered []string
	skippingEnv := true
	for _, tok := range tokens {
		if skippingEnv && envAssignmentPattern.MatchString(tok) {
			continue
		}
		skippingEnv = false
		filtered = append(filtered, tok)
	}

	return filtered
}

// extractBaseExecutable returns the executable a shell would run for one command segment,
// skipping leading environment assignments so "GOFLAGS=-mod=mod go test" resolves to "go".
func extractBaseExecutable(command string) string {
	tokens := parseCommandTokens(command)
	if len(tokens) > 0 {
		return tokens[0]
	}
	return ""
}
