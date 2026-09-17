package context

// ValidationError labels for chain builders live in validation_error_labels.go.

// ContextChain represents a linked list of contexts assembled in order of precedence
// Contexts are processed breadth-first for sequential preference
// Nested contexts are processed in order of depth
type ContextChain struct {
	// Current context node
	Context ChainableContext

	// Next context in the chain (higher precedence)
	Next *ContextChain

	// Parent chain (for nested contexts)
	Parent *ContextChain

	// Child chains (for nested contexts, processed by depth)
	Children []*ContextChain

	// Precedence level (lower = higher precedence, processed first)
	Precedence int

	// Depth in the hierarchy (0 = root, 1 = first level nested, etc.)
	Depth int
}

// ChainableContext represents a context value that can be chained
// This is a generic interface that all context types implement for chain-based processing
type ChainableContext interface {
	// GetPrecedence returns the precedence level for this context
	// Lower values = higher precedence (processed first)
	GetPrecedence() int

	// GetDepth returns the depth of this context in the hierarchy
	// 0 = root level, 1+ = nested levels
	GetDepth() int

	// Validate performs validation on this context node
	// Returns validation errors if any
	Validate() []ValidationError

	// Merge merges this context into the target context
	// Used during breadth-first processing
	Merge(target ChainableContext) ChainableContext
}

// ValidationError represents a validation error for a context node
type ValidationError struct {
	Field   string
	Message string
	Context string // Context name/identifier
}

// NewContextChain creates a new context chain node
func NewContextChain(ctx ChainableContext) *ContextChain {
	precedence := 0
	depth := 0
	if ctx != nil {
		precedence = ctx.GetPrecedence()
		depth = ctx.GetDepth()
	}
	return &ContextChain{
		Context:    ctx,
		Precedence: precedence,
		Depth:      depth,
		Children:   make([]*ContextChain, 0),
	}
}

// Append adds a context to the end of the chain (lowest precedence)
func (cc *ContextChain) Append(ctx ChainableContext) *ContextChain {
	newChain := NewContextChain(ctx)
	newChain.Parent = cc.Parent // Inherit parent from chain
	newChain.Depth = cc.Depth   // Same depth level

	// Find the end of the chain
	current := cc
	for current.Next != nil {
		current = current.Next
	}
	current.Next = newChain

	return newChain
}

// Prepend adds a context to the beginning of the chain (highest precedence)
func (cc *ContextChain) Prepend(ctx ChainableContext) *ContextChain {
	newChain := NewContextChain(ctx)
	newChain.Parent = cc.Parent
	newChain.Depth = cc.Depth
	newChain.Next = cc

	// If this chain has a parent, update parent's reference
	if cc.Parent != nil {
		for i, child := range cc.Parent.Children {
			if child == cc {
				cc.Parent.Children[i] = newChain
				break
			}
		}
	}

	return newChain
}

// AddChild adds a nested context (processed by depth)
func (cc *ContextChain) AddChild(ctx ChainableContext) *ContextChain {
	child := NewContextChain(ctx)
	child.Parent = cc
	child.Depth = cc.Depth + 1
	cc.Children = append(cc.Children, child)
	return child
}

// contextChainMissing reports whether cc or its embedded Context is nil.
func contextChainMissing(cc *ContextChain) bool {
	return cc == nil || cc.Context == nil
}

// ProcessBreadthFirst processes the context chain using breadth-first traversal
// This ensures sequential preference: contexts at the same depth are processed in precedence order
// Nested contexts are processed after all contexts at the current depth
func (cc *ContextChain) ProcessBreadthFirst() (ChainableContext, []ValidationError) {
	if contextChainMissing(cc) {
		return nil, nil
	}

	// Collect all contexts by depth (breadth-first collection)
	contextsByDepth := make(map[int][]*ContextChain)
	cc.collectByDepth(0, contextsByDepth)

	// Find max depth
	maxDepth := 0
	for depth := range contextsByDepth {
		if depth > maxDepth {
			maxDepth = depth
		}
	}

	// Process breadth-first: all depth 0, then all depth 1, etc.
	var result ChainableContext
	var allErrors []ValidationError

	for depth := 0; depth <= maxDepth; depth++ {
		chains := contextsByDepth[depth]
		if len(chains) == 0 {
			continue
		}

		// Sort by precedence (lower = higher precedence, processed first)
		chains = sortByPrecedence(chains)

		// Process all contexts at this depth
		for _, chain := range chains {
			// Validate
			if chain.Context != nil {
				errors := chain.Context.Validate()
				if len(errors) > 0 {
					allErrors = append(allErrors, errors...)
					continue // Skip invalid contexts
				}

				// Merge into result
				if result == nil {
					result = chain.Context
				} else {
					result = chain.Context.Merge(result)
				}
			}
		}
	}

	return result, allErrors
}

// collectByDepth collects all contexts in the chain by depth (breadth-first)
func (cc *ContextChain) collectByDepth(currentDepth int, contextsByDepth map[int][]*ContextChain) {
	if cc == nil {
		return
	}

	// Add current context to its depth level
	if cc.Context != nil {
		contextsByDepth[cc.Depth] = append(contextsByDepth[cc.Depth], cc)
	}

	// Process siblings (same depth) first
	if cc.Next != nil {
		cc.Next.collectByDepth(currentDepth, contextsByDepth)
	}

	// Then process children (next depth level)
	for _, child := range cc.Children {
		child.collectByDepth(currentDepth+1, contextsByDepth)
	}
}

// sortByPrecedence sorts context chains by precedence (lower = higher precedence)
func sortByPrecedence(chains []*ContextChain) []*ContextChain {
	// Simple insertion sort for small lists
	result := make([]*ContextChain, len(chains))
	copy(result, chains)

	for i := 1; i < len(result); i++ {
		key := result[i]
		j := i - 1
		for j >= 0 && result[j].Precedence > key.Precedence {
			result[j+1] = result[j]
			j--
		}
		result[j+1] = key
	}

	return result
}

// GetChainHead returns the head of the chain (highest precedence)
func (cc *ContextChain) GetChainHead() *ContextChain {
	if cc == nil {
		return nil
	}

	// Traverse to the beginning of the chain
	current := cc
	for current.Parent != nil {
		// If we have a parent, find the head of the parent's chain
		parent := current.Parent
		for parent.Parent != nil {
			parent = parent.Parent
		}
		// Find the first sibling in parent's chain
		for parent.Next != nil && parent.Next.Precedence < current.Precedence {
			parent = parent.Next
		}
		current = parent
	}

	// Find the first node in the current chain
	for current.Next != nil && current.Next.Precedence < current.Precedence {
		current = current.Next
	}

	// Now find the absolute head (lowest precedence value)
	head := current
	for current.Next != nil {
		if current.Next.Precedence < head.Precedence {
			head = current.Next
		}
		current = current.Next
	}

	return head
}

// GetChainTail returns the tail of the chain (lowest precedence)
func (cc *ContextChain) GetChainTail() *ContextChain {
	if cc == nil {
		return nil
	}

	current := cc
	for current.Next != nil {
		current = current.Next
	}
	return current
}
