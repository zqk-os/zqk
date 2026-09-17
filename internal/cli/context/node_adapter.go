package context

import (
	pkgctx "github.com/lanceman/zqk/pkg/context"
)

// ContextNodeAdapter adapts ContextNode to work with the chain system
// This allows hierarchical and hybrid processing to use the chain-based approach
type ContextNodeAdapter struct {
	node       *ContextNode
	precedence int
	depth      int
}

// NewContextNodeAdapter creates an adapter for a ContextNode
func NewContextNodeAdapter(node *ContextNode, precedence, depth int) *ContextNodeAdapter {
	return &ContextNodeAdapter{
		node:       node,
		precedence: precedence,
		depth:      depth,
	}
}

// GetNode returns the underlying ContextNode
func (a *ContextNodeAdapter) GetNode() *ContextNode {
	return a.node
}

// GetPrecedence returns the precedence level for this context
func (a *ContextNodeAdapter) GetPrecedence() int {
	return a.precedence
}

// SetPrecedence sets the precedence level for this context
func (a *ContextNodeAdapter) SetPrecedence(precedence int) {
	a.precedence = precedence
}

// GetDepth returns the depth of this context in the hierarchy
func (a *ContextNodeAdapter) GetDepth() int {
	return a.depth
}

// SetDepth sets the depth of this context in the hierarchy
func (a *ContextNodeAdapter) SetDepth(depth int) {
	a.depth = depth
}

// Validate performs validation on this context node
func (a *ContextNodeAdapter) Validate() []pkgctx.ValidationError {
	var errors []pkgctx.ValidationError
	if a.node == nil {
		errors = append(errors, pkgctx.ValidationError{
			Field:   "Node",
			Message: "context node cannot be nil",
			Context: "ContextNodeAdapter",
		})
		return errors
	}
	if a.node.Context == nil {
		errors = append(errors, pkgctx.ValidationError{
			Field:   "Context",
			Message: "context cannot be nil",
			Context: "ContextNodeAdapter",
		})
	}
	return errors
}

// Merge merges this context into the target context
// For hierarchical processing, children inherit from parent
func (a *ContextNodeAdapter) Merge(target pkgctx.ChainableContext) pkgctx.ChainableContext {
	if target == nil {
		return a
	}

	// If target is also a ContextNodeAdapter, merge the underlying contexts
	if targetAdapter, ok := target.(*ContextNodeAdapter); ok {
		// Use the existing ContextProcessor merge logic
		processor := NewContextProcessor(ModeSequential)

		// Start with target (accumulated result)
		var merged *Context
		if targetAdapter.node != nil && targetAdapter.node.Context != nil {
			merged = processor.cloneContext(targetAdapter.node.Context)
		} else {
			merged = &Context{
				layers: make(map[ContextLayer]map[string]any),
			}
		}

		// Merge source (this context) into merged
		if a.node != nil && a.node.Context != nil {
			processor.mergeContext(merged, a.node.Context)
		}

		// Create new node with merged context
		mergedNode := &ContextNode{
			Context:  merged,
			Children: a.node.Children, // Preserve children structure
			Mode:     a.node.Mode,
			Name:     a.node.Name,
			Priority: a.node.Priority,
		}

		// Use the higher precedence value
		resultPrecedence := a.precedence
		if targetAdapter.precedence < a.precedence {
			resultPrecedence = targetAdapter.precedence
		}

		return NewContextNodeAdapter(mergedNode, resultPrecedence, a.depth)
	}

	// Different context types - return this one
	return a
}

// ToChainableContextNode converts a ContextNode to ChainableContext
// Uses the node's Priority as precedence (lower = higher precedence)
func ToChainableContextNode(node *ContextNode, depth int) pkgctx.ChainableContext {
	if node == nil {
		return nil
	}

	// Use Priority as precedence (lower priority = higher precedence)
	precedence := node.Priority
	if precedence == 0 {
		precedence = pkgctx.PrecedenceDefault
	}

	return NewContextNodeAdapter(node, precedence, depth)
}
