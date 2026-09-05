package graph

import (
	"regexp"

	"github.com/lanceman/zqk/pkg/errfmt"
)

var validIdentifierRegex = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// ValidateCypherIdentifier guarantees that dynamic label and relationship names
// match a strict identifier format to prevent Cypher injection vulnerabilities (L:F-SEC-01).
func ValidateCypherIdentifier(identifier string) error {
	if identifier == "" {
		return errfmt.Errorf("empty Cypher identifier")
	}
	if !validIdentifierRegex.MatchString(identifier) {
		return errfmt.Errorf("invalid Cypher identifier %q: must match ^[A-Za-z_][A-Za-z0-9_]*$", identifier)
	}
	return nil
}
