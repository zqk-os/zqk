package routing_builders

import (
	"gopkg.in/yaml.v3"
)

// BaseRoutingRuleBuilder provides common functionality for routing rule builders
// Note: Routing rules store raw YAML structure due to field name mismatches
type BaseRoutingRuleBuilder struct {
	fileName string
	version  string
	rules    []map[string]any // Raw YAML structure
}

// NewBaseRoutingRuleBuilder creates a new base routing rule builder
func NewBaseRoutingRuleBuilder(fileName, version string) *BaseRoutingRuleBuilder {
	return &BaseRoutingRuleBuilder{
		fileName: fileName,
		version:  version,
		rules:    []map[string]any{},
	}
}

// AddRule adds a routing rule (raw YAML structure)
func (b *BaseRoutingRuleBuilder) AddRule(rule map[string]any) *BaseRoutingRuleBuilder {
	b.rules = append(b.rules, rule)
	return b
}

// Build builds the routing rules as YAML bytes
func (b *BaseRoutingRuleBuilder) Build() ([]byte, error) {
	// Marshal rules array to YAML
	return yaml.Marshal(b.rules)
}

// GetVersion returns the version
func (b *BaseRoutingRuleBuilder) GetVersion() string {
	return b.version
}

// GetFileName returns the file name
func (b *BaseRoutingRuleBuilder) GetFileName() string {
	return b.fileName
}
