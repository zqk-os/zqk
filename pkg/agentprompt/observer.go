package agentprompt

import (
	"context"
	"fmt"
	"strings"

	"github.com/lanceman/zqk/pkg/observer"
	"github.com/lanceman/zqk/pkg/utilities/sortutil"
)

// ObserverContext represents the AST and structural graph context for an agent.
type ObserverContext struct {
	Result *observer.ExtractResult
}

// ExtractObserverContext extracts the structural AST graph from the given project directory.
func ExtractObserverContext(ctx context.Context, dir string) (*ObserverContext, error) {
	// Extract structural information from the codebase using the observer package
	res, err := observer.ExtractFromDir(ctx, nil, dir, []observer.Extractor{
		observer.GoExtractor{},
	})
	if err != nil {
		return nil, err
	}

	return &ObserverContext{
		Result: res,
	}, nil
}

// GeneratePromptSection formats the extracted AST structure into a markdown section
// for the agent prompt, providing deep semantic context.
func (o *ObserverContext) GeneratePromptSection() string {
	if o.Result == nil || len(o.Result.Entities) == 0 {
		return "## Codebase Structural Context\nNo significant AST structures discovered.\n"
	}

	var sb strings.Builder
	sb.WriteString("## Codebase Structural Context (Top 30 Highest Complexity Entities)\n")
	sb.WriteString("The following high-complexity structural components (AST) were discovered in the codebase to assist your operations:\n\n")

	// Sort entities by complexity descending
	entities := make([]observer.Entity, len(o.Result.Entities))
	copy(entities, o.Result.Entities)

	// Sort using our generic bubble sort util
	sortutil.BubbleSort(entities, func(a, b observer.Entity) bool {
		return a.Complexity > b.Complexity // sort descending by complexity
	})

	// Limit to top 30
	limit := 30
	if len(entities) < limit {
		limit = len(entities)
	}
	topEntities := entities[:limit]

	// Group top entities by file to provide a clean overview
	grouped := make(map[string][]observer.Entity)
	for _, entity := range topEntities {
		grouped[entity.File] = append(grouped[entity.File], entity)
	}

	// Extract keys and sort them for deterministic output
	var files []string
	for file := range grouped {
		files = append(files, file)
	}
	sortutil.BubbleSort(files, func(a, b string) bool {
		return a < b
	})

	for _, file := range files {
		fileEntities := grouped[file]
		sb.WriteString(fmt.Sprintf("### File: `%s`\n", file))
		for _, entity := range fileEntities {
			complexityStr := ""
			if entity.Complexity > 0 {
				complexityStr = fmt.Sprintf(" (Complexity: %d)", entity.Complexity)
				if entity.Complexity > 10 {
					complexityStr += " ⚠️ HIGH COMPLEXITY"
				}
			}
			sb.WriteString(fmt.Sprintf("- **%s** (`%s`): `%s` %s\n", entity.Name, entity.Kind, entity.Signature, complexityStr))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}
