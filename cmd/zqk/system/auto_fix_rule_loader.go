package system

import (
	"github.com/zqk-os/zqk/pkg/systemcheck/autofix"
)

// AutoFixRuleLoader delegates to pkg/systemcheck/autofix.AutoFixRuleLoader.
type AutoFixRuleLoader = autofix.AutoFixRuleLoader

// NewAutoFixRuleLoader creates a loader with default list timeout.
func NewAutoFixRuleLoader() *AutoFixRuleLoader {
	return autofix.NewAutoFixRuleLoader()
}

// GetAutoFixRuleLoader returns the process-wide auto_fix_rule loader (lazy init).
func GetAutoFixRuleLoader() *AutoFixRuleLoader {
	return autofix.GetAutoFixRuleLoader()
}

func substituteFixCommandPlaceholders(template, kind, category string, tier int, rule, message, objID, field string) string {
	return autofix.SubstituteFixCommandPlaceholders(template, kind, category, tier, rule, message, objID, field)
}
