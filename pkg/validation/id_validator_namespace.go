package validation

import (
	"fmt"
	"strings"

	"github.com/lanceman/zqk/pkg/when"
)

// ParsedNamespace represents a parsed namespace from an object ID
type ParsedNamespace struct {
	NamespaceID string // Full namespace ID (e.g., "zqk:kernel", "domain:organizational")
	Layer       string // Layer: "zqk", "domain", or "integration"
	Domain      string // Domain name (for domain/integration layers)
	Subdomain   string // Subdomain (optional)
	ObjectType  string // Object type (e.g., "goal", "organization")
	ObjectID    string // Object ID without namespace (e.g., "GOAL-123")
	FullID      string // Full ID with namespace (e.g., "zqk:kernel:goal:GOAL-123")
}

// ParseNamespace parses a namespace from an object ID
// Supports formats:
//   - Full format: "zqk:kernel:goal:GOAL-123" or "domain:organizational:organization:ORG-001"
//   - Short format: "goal:GOAL-123" (assumes zqk:kernel)
//   - Legacy format: "GOAL-123" (assumes zqk:kernel, no object type)
//
// Returns nil if the ID doesn't contain namespace information (legacy format)
func ParseNamespace(id string) *ParsedNamespace {
	// Full format: {layer}:{domain}:{subdomain}:{object_type}:{object_id}
	// Examples:
	//   - zqk:kernel:goal:GOAL-123
	//   - domain:organizational:organization:ORG-001
	//   - integration:jira:issue:PROJ-123
	parts := strings.Split(id, ":")

	if len(parts) < 2 {
		// Legacy format: no namespace, just ID (e.g., "GOAL-123")
		return nil
	}

	// Check if first part is a valid layer
	layer := parts[0]
	isValidLayer := false
	for _, validLayer := range ValidNamespaceLayers {
		if layer == validLayer {
			isValidLayer = true
			break
		}
	}
	if !isValidLayer {
		// Not a namespace format, might be short format or legacy
		// Short format: {object_type}:{object_id} (e.g., "goal:GOAL-123")
		if len(parts) == 2 {
			// Assume zqk:kernel for short format
			return &ParsedNamespace{
				NamespaceID: DefaultNamespaceKernel,
				Layer:       NamespaceLayerZqk,
				Domain:      "kernel",
				ObjectType:  parts[0],
				ObjectID:    parts[1],
				FullID:      id,
			}
		}
		// Legacy format
		return nil
	}

	// Full namespace format
	parsed := &ParsedNamespace{
		Layer:  layer,
		FullID: id,
	}

	switch layer {
	case NamespaceLayerZqk:
		// zqk:kernel:goal:GOAL-123
		if len(parts) >= 3 {
			parsed.Domain = parts[1] // "kernel"
			parsed.NamespaceID = fmt.Sprintf("%s:%s", layer, parsed.Domain)
			if len(parts) >= 4 {
				parsed.ObjectType = parts[2]
				parsed.ObjectID = strings.Join(parts[3:], ":")
			} else if len(parts) == 3 {
				// zqk:kernel:GOAL-123 (no object type)
				parsed.ObjectID = parts[2]
			}
		}
	case NamespaceLayerDomain, NamespaceLayerIntegration:
		// domain:organizational:organization:ORG-001 (4 parts: layer:domain:object_type:object_id)
		// domain:financial:accounting:account:ACC-001 (5 parts: layer:domain:subdomain:object_type:object_id)
		// integration:jira:issue:PROJ-123 (4 parts: layer:domain:object_type:object_id)
		if len(parts) >= 2 {
			parsed.Domain = parts[1]
			when.When(func() bool { return len(parts) == 3 }).Then(func() {
				// domain:organizational:ORG-001 (no object type)
				parsed.ObjectID = parts[2]
				parsed.NamespaceID = fmt.Sprintf("%s:%s", layer, parsed.Domain)
			}).OrElseWhen(func() bool { return len(parts) == 4 }).Then(func() {
				// domain:organizational:organization:ORG-001 (no subdomain)
				parsed.ObjectType = parts[2]
				parsed.ObjectID = parts[3]
				parsed.NamespaceID = fmt.Sprintf("%s:%s", layer, parsed.Domain)
			}).OrElseWhen(func() bool { return len(parts) >= 5 }).Then(func() {
				// domain:financial:accounting:account:ACC-001 (has subdomain)
				parsed.Subdomain = parts[2]
				parsed.ObjectType = parts[3]
				parsed.ObjectID = strings.Join(parts[4:], ":")
				parsed.NamespaceID = fmt.Sprintf("%s:%s:%s", layer, parsed.Domain, parsed.Subdomain)
			}).Run()
		}
	}

	return parsed
}
