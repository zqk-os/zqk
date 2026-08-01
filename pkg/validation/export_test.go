package validation

import "github.com/lanceman/zqk/pkg/graph/provider"

// Export parseGraphSpecNode for testing
func ParseGraphSpecNodeForTest(v *IDValidator, node *provider.Node) *IDPatternConfig {
	return v.parseGraphSpecNode(node)
}
