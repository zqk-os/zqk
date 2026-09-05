package validation

import (
	"strings"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/paths"
)

// NamespaceInfo represents information about a namespace for validation
type NamespaceInfo struct {
	NamespaceID string
	Layer       string
	Domain      string
	Integration *NamespaceIntegration
	Isolation   *NamespaceIsolation
}

// NamespaceIntegration represents integration rules for a namespace
type NamespaceIntegration struct {
	CanReference      []ReferenceRule
	CanBeReferencedBy []ReferenceRule
}

// ReferenceRule represents a rule about which namespaces can reference which object types
type ReferenceRule struct {
	NamespaceID        string
	ObjectTypes        []string
	ReferenceDirection string
	Validation         string
}

// NamespaceIsolation represents isolation rules for a namespace
type NamespaceIsolation struct {
	Validation struct {
		CrossNamespaceValidation bool
		ReferenceValidation      string
	}
}

// ValidateCrossNamespaceReference validates if a reference from one namespace to another is allowed
// Returns error if reference is not allowed, nil if allowed
func ValidateCrossNamespaceReference(fromNamespaceID, toNamespaceID, objectType string, namespaceInfo map[string]*NamespaceInfo) error {
	// Same namespace - always allowed
	if fromNamespaceID == toNamespaceID {
		return nil
	}

	// Get namespace info for source and target
	fromNS := namespaceInfo[fromNamespaceID]
	toNS := namespaceInfo[toNamespaceID]

	// If namespace info not available, allow by default (backward compatibility)
	if fromNS == nil || toNS == nil {
		return nil
	}

	// Check if target namespace allows being referenced by source namespace
	if toNS.Integration != nil {
		for _, rule := range toNS.Integration.CanBeReferencedBy {
			if rule.NamespaceID == fromNamespaceID {
				// Check if object type is allowed
				if len(rule.ObjectTypes) == 0 || stringSliceContains(rule.ObjectTypes, objectType) {
					// Reference is allowed
					return nil
				}
			}
		}
	}

	// Check if source namespace allows referencing target namespace
	if fromNS.Integration != nil {
		for _, rule := range fromNS.Integration.CanReference {
			if rule.NamespaceID == toNamespaceID {
				// Check if object type is allowed
				if len(rule.ObjectTypes) == 0 || stringSliceContains(rule.ObjectTypes, objectType) {
					// Reference is allowed
					return nil
				}
			}
		}
	}

	// Default: kernel namespace can always be referenced
	if toNamespaceID == paths.KernelNamespaceID || strings.HasSuffix(toNamespaceID, ":kernel") {
		return nil
	}

	// Reference not explicitly allowed
	return errfmt.Errorf(ConstMagicea3eff67, fromNamespaceID, objectType, toNamespaceID, toNamespaceID)
}

// ParseNamespaceFromReference parses namespace information from a reference string
// Supports formats:
//   - Full: "zqk:kernel:goal:GOAL-123"
//   - Short: "goal:GOAL-123" (assumes zqk:kernel)
//   - Legacy: "GOAL-123" (assumes zqk:kernel, inferred from kind)
//
// Note: This is a wrapper around ParseNamespace from id_validator.go
func ParseNamespaceFromReference(refID, defaultNamespaceID string) (namespaceID, objectType, objectID string) {
	// Parse using existing ParseNamespace function from id_validator.go
	// ParseNamespace is in the same package, so we can call it directly
	parsed := ParseNamespace(refID)
	if parsed != nil && parsed.NamespaceID != emptyValue {
		return parsed.NamespaceID, parsed.ObjectType, parsed.ObjectID
	}

	// Check for short format "kind:id"
	if strings.Contains(refID, ":") && !strings.HasPrefix(refID, "account:") {
		parts := strings.SplitN(refID, ":", 2)
		if len(parts) == 2 {
			// Short format - use default namespace
			return defaultNamespaceID, parts[0], parts[1]
		}
	}

	// Legacy format - use default namespace, object type will be inferred
	return defaultNamespaceID, "", refID
}

// stringSliceContains checks if a string slice contains a value
func stringSliceContains(slice []string, value string) bool {
	for _, v := range slice {
		if v == value {
			return true
		}
	}
	return false
}
