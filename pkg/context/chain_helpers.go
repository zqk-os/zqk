package context

// Precedence constants for common context types
// Lower values = higher precedence (processed first)
const (
	PrecedenceSystem  = 0   // System defaults (lowest precedence, processed first)
	PrecedenceUser    = 25  // User configuration
	PrecedenceProject = 50  // Project configuration
	PrecedenceCommand = 75  // Command-line flags (highest precedence, processed last)
	PrecedenceDefault = 100 // Default precedence for unspecified contexts
)

// Depth constants for context hierarchy
const (
	DepthRoot   = 0 // Root level contexts
	DepthNested = 1 // First level nested contexts
)

// BuildContextChain is a DRY helper that builds a context chain from multiple contexts
// Automatically sets precedence and depth based on context type
func BuildContextChain(contexts ...ChainableContext) (*ContextChain, []ValidationError) {
	if len(contexts) == 0 {
		return nil, []ValidationError{{
			Field:   validationErrFieldContexts,
			Message: validationErrMsgNoContextsProvided,
			Context: validationErrContextBuildChain,
		}}
	}

	builder := NewChainBuilder()
	for _, ctx := range contexts {
		if ctx != nil {
			builder.AddContext(ctx)
		}
	}

	return buildAndGetChain(builder)
}

// BuildNestedContextChain builds a context chain with nested contexts
// parent is the root context, children are nested at depth 1
func BuildNestedContextChain(parent ChainableContext, children ...ChainableContext) (*ContextChain, []ValidationError) {
	if parent == nil {
		return nil, []ValidationError{{
			Field:   validationErrFieldParent,
			Message: validationErrMsgParentRequired,
			Context: validationErrContextBuildNested,
		}}
	}

	builder := NewChainBuilder()
	builder.AddContext(parent)

	// Set depth for children
	for _, child := range children {
		if child != nil {
			// Set child depth
			if setter, ok := child.(interface{ SetDepth(int) }); ok {
				setter.SetDepth(DepthNested)
			}
			builder.AddNestedContext(parent, child)
		}
	}

	return buildAndGetChain(builder)
}

func buildAndGetChain(builder *ChainBuilder) (*ContextChain, []ValidationError) {
	result, errors := builder.Build()
	if result == nil {
		return nil, errors
	}
	return builder.GetChain(), errors
}

// MergeContexts merges multiple contexts using chain-based processing
// Returns the merged context and any validation errors
func MergeContexts(contexts ...ChainableContext) (ChainableContext, []ValidationError) {
	chain, errors := BuildContextChain(contexts...)
	if chain == nil {
		return nil, errors
	}

	result, validationErrors := chain.ProcessBreadthFirst()
	allErrors := make([]ValidationError, 0, len(errors)+len(validationErrors))
	allErrors = append(allErrors, errors...)
	allErrors = append(allErrors, validationErrors...)
	return result, allErrors
}

// SetPrecedenceForContext sets precedence on a context if it supports it
// This is a DRY helper to avoid type assertions everywhere
func SetPrecedenceForContext(ctx ChainableContext, precedence int) {
	if setter, ok := ctx.(interface{ SetPrecedence(int) }); ok {
		setter.SetPrecedence(precedence)
	}
}

// SetDepthForContext sets depth on a context if it supports it
// This is a DRY helper to avoid type assertions everywhere
func SetDepthForContext(ctx ChainableContext, depth int) {
	if setter, ok := ctx.(interface{ SetDepth(int) }); ok {
		setter.SetDepth(depth)
	}
}

// ValidateContextChain validates all contexts in a chain
// Returns all validation errors from all contexts
func ValidateContextChain(chain *ContextChain) []ValidationError {
	if chain == nil {
		return nil
	}

	var allErrors []ValidationError

	// Validate all contexts in the chain (breadth-first)
	visited := make(map[ChainableContext]bool)
	toProcess := []*ContextChain{chain}

	for len(toProcess) > 0 {
		nextLevel := []*ContextChain{}

		for _, node := range toProcess {
			if contextChainMissing(node) {
				continue
			}

			// Skip if already validated
			if visited[node.Context] {
				continue
			}
			visited[node.Context] = true

			// Validate current context
			errors := node.Context.Validate()
			allErrors = append(allErrors, errors...)

			// Add siblings to next level
			if node.Next != nil {
				nextLevel = append(nextLevel, node.Next)
			}

			// Add children to next level
			nextLevel = append(nextLevel, node.Children...)
		}

		toProcess = nextLevel
	}

	return allErrors
}
