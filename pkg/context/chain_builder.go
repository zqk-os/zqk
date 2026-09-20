package context

// ChainBuilder helps build context chains with precedence ordering
// Provides a DRY way to assemble contexts in order of precedence
// Supports both sequential chains and nested hierarchies
type ChainBuilder struct {
	chain *ContextChain
}

// NewChainBuilder creates a new chain builder
func NewChainBuilder() *ChainBuilder {
	return &ChainBuilder{}
}

// AddContext adds a context to the chain in order of precedence
// Lower precedence values are processed first (higher priority)
func (cb *ChainBuilder) AddContext(ctx ChainableContext) *ChainBuilder {
	if cb.chain == nil {
		cb.chain = NewContextChain(ctx)
		return cb
	}

	// Insert in precedence order (lower precedence = higher priority = earlier in chain)
	current := cb.chain
	prev := (*ContextChain)(nil)
	newChain := NewContextChain(ctx)

	// Find insertion point based on precedence
	for current != nil && current.Precedence <= ctx.GetPrecedence() {
		prev = current
		current = current.Next
	}

	// Insert new chain
	if prev == nil {
		// Insert at head (highest priority)
		newChain.Next = cb.chain
		cb.chain = newChain
	} else {
		// Insert in middle or at end
		newChain.Next = current
		prev.Next = newChain
	}

	return cb
}

// AddNestedContext adds a nested context to a parent context
// Nested contexts are processed by depth after all contexts at the parent depth
func (cb *ChainBuilder) AddNestedContext(parent, child ChainableContext) *ChainBuilder {
	if cb.chain == nil {
		// If no chain yet, create root with child
		root := NewContextChain(parent)
		root.AddChild(child)
		cb.chain = root
		return cb
	}

	// Find parent in chain
	parentChain := cb.findChainByContext(parent)
	if parentChain == nil {
		// Parent not found, add as new root with child
		root := NewContextChain(parent)
		root.AddChild(child)
		root.Next = cb.chain
		cb.chain = root
		return cb
	}

	// Add child to parent
	parentChain.AddChild(child)
	return cb
}

// Build processes the chain using breadth-first traversal
// Returns the merged context and any validation errors
func (cb *ChainBuilder) Build() (ChainableContext, []ValidationError) {
	if cb.chain == nil {
		return nil, []ValidationError{{
			Field:   validationErrFieldChain,
			Message: validationErrMsgNoContextsInChain,
			Context: validationErrContextChainBuilder,
		}}
	}

	return cb.chain.ProcessBreadthFirst()
}

// findChainByContext finds a chain node containing the given context
func (cb *ChainBuilder) findChainByContext(ctx ChainableContext) *ContextChain {
	if cb.chain == nil {
		return nil
	}

	return cb.findInChain(cb.chain, ctx)
}

// findInChain recursively searches for a context in the chain
func (cb *ChainBuilder) findInChain(chain *ContextChain, ctx ChainableContext) *ContextChain {
	if chain == nil {
		return nil
	}

	// Check current node
	if chain.Context == ctx {
		return chain
	}

	// Check siblings
	if chain.Next != nil {
		if found := cb.findInChain(chain.Next, ctx); found != nil {
			return found
		}
	}

	// Check children
	for _, child := range chain.Children {
		if found := cb.findInChain(child, ctx); found != nil {
			return found
		}
	}

	return nil
}

// GetChain returns the current chain (for inspection/debugging)
func (cb *ChainBuilder) GetChain() *ContextChain {
	return cb.chain
}

// Example usage:
//
// builder := NewChainBuilder()
// builder.AddContext(systemCtx)      // precedence: 0
// builder.AddContext(userCtx)       // precedence: 1
// builder.AddContext(projectCtx)    // precedence: 2
// builder.AddNestedContext(projectCtx, featureCtx) // depth: 1
// result, errors := builder.Build()
