package mcp

// ResourceURISchemeRuleConfig represents a URI scheme rule in configuration
// This is the YAML-serializable version of ResourceURISchemeRule
type ResourceURISchemeRuleConfig struct {
	Pattern     string `yaml:"pattern"`     // Path pattern to match (supports glob patterns)
	Scheme      string `yaml:"scheme"`      // URI scheme to use (e.g., "file://", "docs://", "internal://")
	Category    string `yaml:"category"`    // Resource category (e.g., "guide", "architecture", "internal")
	Description string `yaml:"description"` // Description of this rule
}

// ToResourceURISchemeRule converts a config rule to a runtime rule
func (c *ResourceURISchemeRuleConfig) ToResourceURISchemeRule() ResourceURISchemeRule {
	return ResourceURISchemeRule{
		Pattern:     c.Pattern,
		Scheme:      c.Scheme,
		Category:    c.Category,
		Description: c.Description,
	}
}

// LoadResourceURISchemeRulesFromConfig loads URI scheme rules from ServerConfig
// Returns configured rules if available, otherwise returns default rules
func LoadResourceURISchemeRulesFromConfig(config *ServerConfig) []ResourceURISchemeRule {
	if config == nil {
		return getDefaultResourceURISchemeRules()
	}

	// Check if custom rules are configured
	if len(config.MCPServer.Resources.URISchemeRules) > 0 {
		rules := make([]ResourceURISchemeRule, 0, len(config.MCPServer.Resources.URISchemeRules))
		for _, ruleConfig := range config.MCPServer.Resources.URISchemeRules {
			rules = append(rules, ruleConfig.ToResourceURISchemeRule())
		}
		return rules
	}

	// Use default rules
	return getDefaultResourceURISchemeRules()
}
