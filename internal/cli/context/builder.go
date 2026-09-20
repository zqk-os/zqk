package context

import (
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
)

// ContextBuilder helps assemble contexts in different ways
// Supports sequential, hierarchical, and hybrid assembly
type ContextBuilder struct {
	nodes     []*ContextNode
	root      *ContextNode
	mode      ProcessingMode
	processor *ContextProcessor
}

// NewContextBuilder creates a new context builder
func NewContextBuilder(mode ProcessingMode) *ContextBuilder {
	return &ContextBuilder{
		nodes:     make([]*ContextNode, 0),
		mode:      mode,
		processor: NewContextProcessor(mode),
	}
}

// AddContext adds a context to be processed sequentially
// Use this for sequential processing mode
func (cb *ContextBuilder) AddContext(ctx *Context, name string) *ContextBuilder {
	node := NewContextNode(ctx, ModeSequential)
	node.Name = name
	cb.nodes = append(cb.nodes, node)
	return cb
}

// SetRoot sets the root context for hierarchical/hybrid processing
func (cb *ContextBuilder) SetRoot(ctx *Context, name string) *ContextBuilder {
	cb.root = NewContextNode(ctx, cb.mode)
	cb.root.Name = name
	return cb
}

// AddChild adds a child context to the root (for hierarchical/hybrid)
func (cb *ContextBuilder) AddChild(ctx *Context, name string) *ContextBuilder {
	if cb.root == nil {
		// If no root, create one with empty context
		cb.root = NewContextNode(&Context{layers: make(map[ContextLayer]map[string]any)}, cb.mode)
		cb.root.Name = "root"
	}
	child := NewContextNode(ctx, cb.mode)
	child.Name = name
	cb.root.AddChild(child)
	return cb
}

// AddNestedChild adds a child to a specific parent node
// This enables building complex hierarchical structures
func (cb *ContextBuilder) AddNestedChild(parentName string, ctx *Context, name string) (*ContextBuilder, error) {
	parent := cb.findNodeByName(parentName)
	if parent == nil {
		return nil, errfmt.Errorf("parent node '%s' not found", parentName)
	}
	child := NewContextNode(ctx, cb.mode)
	child.Name = name
	parent.AddChild(child)
	return cb, nil
}

// Build processes all contexts according to the builder's mode
// Now uses the chain-based system for all processing modes
func (cb *ContextBuilder) Build() (*Context, error) {
	switch cb.mode {
	case ModeSequential:
		// Process all nodes sequentially using chain system
		chainBuilder := pkgctx.NewChainBuilder()

		for _, node := range cb.nodes {
			if node.Context != nil {
				// Convert to chainable context
				chainable := ToChainableContext(node.Context, LayerCommand) // Sequential nodes use command layer precedence
				chainBuilder.AddContext(chainable)
			}
		}

		chainResult, validationErrors := chainBuilder.Build()
		if len(validationErrors) > 0 {
			// Log validation errors but continue (non-fatal)
		}

		// Extract Context from adapter
		if adapter, ok := chainResult.(*ContextChainAdapter); ok {
			return adapter.GetContext(), nil
		}

		return nil, errfmt.Errorf("chain processing returned unexpected type: %T", chainResult)

	case ModeHierarchical:
		if cb.root == nil {
			return nil, errfmt.Errorf("root context required for hierarchical processing")
		}
		return cb.processor.ProcessHierarchical(cb.root)

	case ModeHybrid:
		if cb.root == nil {
			return nil, errfmt.Errorf("root context required for hybrid processing")
		}
		return cb.processor.ProcessHybrid(cb.root)

	default:
		return nil, errfmt.Errorf("unknown processing mode: %d", cb.mode)
	}
}

// findNodeByName finds a node by name (recursive search)
func (cb *ContextBuilder) findNodeByName(name string) *ContextNode {
	// Search in root and its children
	if cb.root != nil {
		if found := cb.findNodeInTree(cb.root, name); found != nil {
			return found
		}
	}
	// Search in sequential nodes
	for _, node := range cb.nodes {
		if node.Name == name {
			return node
		}
		if found := cb.findNodeInTree(node, name); found != nil {
			return found
		}
	}
	return nil
}

// findNodeInTree recursively searches for a node by name
func (cb *ContextBuilder) findNodeInTree(node *ContextNode, name string) *ContextNode {
	if node.Name == name {
		return node
	}
	for _, child := range node.Children {
		if found := cb.findNodeInTree(child, name); found != nil {
			return found
		}
	}
	return nil
}

// GetNode returns a node by name for further manipulation
func (cb *ContextBuilder) GetNode(name string) *ContextNode {
	return cb.findNodeByName(name)
}

// SetMode changes the processing mode
func (cb *ContextBuilder) SetMode(mode ProcessingMode) *ContextBuilder {
	cb.mode = mode
	cb.processor = NewContextProcessor(mode)
	return cb
}

// Example usage patterns:
//
// Sequential:
//   builder := NewContextBuilder(ModeSequential)
//   builder.AddContext(systemCtx, "system")
//   builder.AddContext(userCtx, "user")
//   builder.AddContext(projectCtx, "project")
//   result, _ := builder.Build()
//
// Hierarchical:
//   builder := NewContextBuilder(ModeHierarchical)
//   builder.SetRoot(systemCtx, "system")
//   builder.AddChild(userCtx, "user")
//   builder.AddChild(projectCtx, "project")
//   result, _ := builder.Build()
//
// Hybrid:
//   builder := NewContextBuilder(ModeHybrid)
//   builder.SetRoot(systemCtx, "system")
//   builder.AddChild(userCtx, "user")
//   builder.AddChild(projectCtx, "project")
//   builder.AddNestedChild("project", featureCtx, "feature")
//   result, _ := builder.Build()
