package policy

import (
	"context"
	"fmt"
	"strings"
)

// Result represents the outcome of a policy gate execution.
type Result struct {
	GateName   string   `json:"gate_name"`
	Passed     bool     `json:"passed"`
	Message    string   `json:"message"`
	Violations []string `json:"violations,omitempty"`
	Warnings   []string `json:"warnings,omitempty"`
}

// RunOptions configures policy gate execution.
type RunOptions struct {
	ProjectRoot string   `json:"project_root"`
	Files       []string `json:"files,omitempty"` // Specific files to check (e.g. staged files)
	Scope       string   `json:"scope,omitempty"` // "staged", "working", "all"
	Verbose     bool     `json:"verbose,omitempty"`
}

// Gate defines the interface for repository policy compliance checks.
type Gate interface {
	Name() string
	Description() string
	Run(ctx context.Context, opts RunOptions) (*Result, error)
}

// Registry holds all registered policy gates.
var Registry = map[string]Gate{
	"doc-links":          &DocLinksGate{},
	"secrets":            &SecretsGate{},
	"storage-boundaries": &StorageBoundariesGate{},
	"goroutines":         &GoroutinesGate{},
	"field-keys":         &FieldKeysGate{},
	"project-nesting":    &ProjectNestingGate{},
}

// AvailableGates returns a list of all registered gate names.
func AvailableGates() []string {
	gates := make([]string, 0, len(Registry))
	for name := range Registry {
		gates = append(gates, name)
	}
	return gates
}

// RunGates executes the specified gates (or all if empty) and returns individual results.
func RunGates(ctx context.Context, opts RunOptions, gateNames ...string) ([]*Result, bool) {
	if len(gateNames) == 0 || (len(gateNames) == 1 && gateNames[0] == "all") {
		gateNames = []string{"doc-links", "secrets", "storage-boundaries", "goroutines", "field-keys", "project-nesting"}
	}

	results := make([]*Result, 0, len(gateNames))
	allPassed := true

	for _, name := range gateNames {
		normalized := strings.ToLower(strings.TrimSpace(name))
		gate, exists := Registry[normalized]
		if !exists {
			results = append(results, &Result{
				GateName: normalized,
				Passed:   false,
				Message:  fmt.Sprintf("unknown policy gate: %s (available: %s)", normalized, strings.Join(AvailableGates(), ", ")),
			})
			allPassed = false
			continue
		}

		res, err := gate.Run(ctx, opts)
		if err != nil {
			results = append(results, &Result{
				GateName: normalized,
				Passed:   false,
				Message:  fmt.Sprintf("gate execution error: %v", err),
			})
			allPassed = false
			continue
		}

		if !res.Passed {
			allPassed = false
		}
		results = append(results, res)
	}

	return results, allPassed
}
