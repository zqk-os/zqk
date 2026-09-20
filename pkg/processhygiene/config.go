package processhygiene

// RulesFile is the on-disk YAML schema for process hygiene rules (versioned for forward compatibility).
type RulesFile struct {
	Version int          `json:"version" yaml:"version"`
	Rules   []RuleConfig `json:"rules" yaml:"rules"`
}

// RuleConfig is one declarative rule.
type RuleConfig struct {
	ID          string `json:"id" yaml:"id"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
	Match       Match  `json:"match" yaml:"match"`
}

// Match selects a field and one of prefix / suffix / equals / regex.
type Match struct {
	Field  string `json:"field" yaml:"field"`
	Prefix string `json:"prefix,omitempty" yaml:"prefix,omitempty"`
	Suffix string `json:"suffix,omitempty" yaml:"suffix,omitempty"`
	Equals string `json:"equals,omitempty" yaml:"equals,omitempty"`
	Regex  string `json:"regex,omitempty" yaml:"regex,omitempty"`
}
