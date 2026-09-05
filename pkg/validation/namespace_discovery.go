package validation

import (
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/objects"
)

// NamespaceDiscovery provides functionality to discover and load namespace objects
// from the namespace registry
type NamespaceDiscovery struct {
	// NamespaceInfo cache - maps namespace_id to NamespaceInfo
	namespaceCache map[string]*NamespaceInfo
}

// NewNamespaceDiscovery creates a new namespace discovery instance
func NewNamespaceDiscovery() *NamespaceDiscovery {
	return &NamespaceDiscovery{
		namespaceCache: make(map[string]*NamespaceInfo),
	}
}

// LoadNamespaceFromObject loads namespace information from a namespace object
// This converts a namespace object (from storage) into NamespaceInfo for validation
func LoadNamespaceFromObject(namespaceObj map[string]any) (*NamespaceInfo, error) {
	namespaceID, ok := namespaceObj[objects.FieldKeyNamespaceID].(string)
	if !ok || namespaceID == emptyValue {
		return nil, errfmt.Errorf(ConstMagic40cc4f1c)
	}

	info := &NamespaceInfo{
		NamespaceID: namespaceID,
	}

	// Extract layer
	if layer := objects.GetString(namespaceObj, objects.FieldKeyLayer); ok {
		info.Layer = layer
	}

	// Extract domain
	if domain := objects.GetString(namespaceObj, objects.FieldKeyDomain); ok {
		info.Domain = domain
	}

	// Extract integration rules
	if integrationRaw, ok := namespaceObj[objects.FieldKeyIntegration].(map[string]any); ok {
		integration := &NamespaceIntegration{}

		// Parse can_reference
		if canRefRaw, ok := integrationRaw["can_reference"].([]any); ok {
			integration.CanReference = parseReferenceRules(canRefRaw)
		}

		// Parse can_be_referenced_by
		if canBeRefRaw, ok := integrationRaw[ConstMagicExtracted_62].([]any); ok {
			integration.CanBeReferencedBy = parseReferenceRules(canBeRefRaw)
		}

		info.Integration = integration
	}

	// Extract isolation rules
	if isolationRaw, ok := namespaceObj[objects.FieldKeyIsolation].(map[string]any); ok {
		isolation := &NamespaceIsolation{}

		if validationRaw, ok := isolationRaw["validation"].(map[string]any); ok {
			if crossNS, ok := validationRaw[ConstMagicExtracted_63].(bool); ok {
				isolation.Validation.CrossNamespaceValidation = crossNS
			}
			if refVal := objects.GetString(validationRaw, ConstMagicExtracted_64); ok {
				isolation.Validation.ReferenceValidation = refVal
			}
		}

		info.Isolation = isolation
	}

	return info, nil
}

// parseReferenceRules parses reference rules from raw YAML data
func parseReferenceRules(rulesRaw []any) []ReferenceRule {
	rules := make([]ReferenceRule, 0, len(rulesRaw))

	for _, ruleRaw := range rulesRaw {
		ruleMap, ok := ruleRaw.(map[string]any)
		if !ok {
			continue
		}
		rule := ReferenceRule{}

		if nsID := objects.GetString(ruleMap, objects.FieldKeyNamespaceID); ok {
			rule.NamespaceID = nsID
		}

		if objTypesRaw, ok := ruleMap["object_types"].([]any); ok {
			rule.ObjectTypes = make([]string, 0, len(objTypesRaw))
			for _, objTypeRaw := range objTypesRaw {
				if objType, ok := objTypeRaw.(string); ok {
					rule.ObjectTypes = append(rule.ObjectTypes, objType)
				}
			}
		}

		if dir := objects.GetString(ruleMap, ConstMagicExtracted_65); ok {
			rule.ReferenceDirection = dir
		}

		if val := objects.GetString(ruleMap, "validation"); ok {
			rule.Validation = val
		}

		rules = append(rules, rule)
	}

	return rules
}

// DiscoverNamespacesFromRegistry loads namespace objects from a namespace_registry object
// This is a helper that can be called when you have access to the namespace_registry object
// Returns a map of namespace_id -> NamespaceInfo for use in validation
func DiscoverNamespacesFromRegistry(registryObj map[string]any, namespaceObjects []map[string]any) (map[string]*NamespaceInfo, error) {
	result := make(map[string]*NamespaceInfo)

	// Process each namespace object
	for _, nsObj := range namespaceObjects {
		info, err := LoadNamespaceFromObject(nsObj)
		if err != nil {
			continue // Skip invalid namespace objects
		}
		result[info.NamespaceID] = info
	}

	return result, nil
}
