package policy

import (
	"context"
	"fmt"
	"os"

	"github.com/zqk-os/zqk/pkg/paths"
)

// ProjectNestingGate validates that the project root has no nested project roots (.zqk dirs)
// and is not itself nested under an ancestor project root.
type ProjectNestingGate struct{}

func (g *ProjectNestingGate) Name() string {
	return "project-nesting"
}

func (g *ProjectNestingGate) Description() string {
	return "Validates that project roots are not nested and no rogue nested .zqk roots exist"
}

func (g *ProjectNestingGate) Run(ctx context.Context, opts RunOptions) (*Result, error) {
	root := opts.ProjectRoot
	if root == "" {
		var err error
		root, err = os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("failed to get current working directory: %w", err)
		}
	}

	var violations []string

	// Check 1: validate candidate root nesting (upward and downward)
	if err := paths.ValidateProjectRootNesting(root); err != nil {
		violations = append(violations, fmt.Sprintf("Project root nesting validation failed: %v", err))
	}

	// Check 2: specifically detect any nested descendant roots under root
	nested, err := paths.FindNestedProjectRoots(root)
	if err != nil {
		violations = append(violations, fmt.Sprintf("Failed to scan for nested project roots: %v", err))
	} else if len(nested) > 0 {
		for _, n := range nested {
			violations = append(violations, fmt.Sprintf("Rogue nested project root detected: %s (purge with 'paths.PurgeNestedProjectRoots' or --auto-fix)", n))
		}
	}

	if len(violations) > 0 {
		return &Result{
			GateName:   g.Name(),
			Passed:     false,
			Message:    fmt.Sprintf("Found %d project root nesting violation(s)", len(violations)),
			Violations: violations,
		}, nil
	}

	return &Result{
		GateName: g.Name(),
		Passed:   true,
		Message:  "No nested project roots detected; root nesting invariant satisfied",
	}, nil
}
