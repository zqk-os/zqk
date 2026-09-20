package system

// This file has been split into focused modules for better maintainability:
//
// - spec_auto_fixer_types_init.go: Types, initialization, and regex variables
// - spec_auto_fixer_main.go: Main fix logic (FixInstanceValidationIssue, fixInstanceValidationIssueLayer1, attemptFixes)
// - spec_auto_fixer_coercion.go: Type coercion functions
// - spec_auto_fixer_pattern.go: Pattern fixing functions
// - spec_auto_fixer_length.go: Length constraint functions
// - spec_auto_fixer_helpers.go: Helper functions (extractDefaultValue, getValidationMap, extractEnumValue, applySpecFix)
//
// All public APIs remain unchanged - this split is purely organizational.
