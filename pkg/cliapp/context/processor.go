package context

import (
	"maps"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
)

// sharedProfileLoader resolves CLI profiles during context merge (Derive, chain adapters) where no ContextManager is available.
var sharedProfileLoader = NewProfileLoader("")

func copyProfileFieldsFromContext(target, source *Context) {
	if target == nil || source == nil {
		return
	}
	if source.Format != emptyValue {
		target.Format = source.Format
	}
	target.Verbose = source.Verbose
	target.Quiet = source.Quiet
	if source.StorageMaxPageSize > 0 {
		target.StorageMaxPageSize = source.StorageMaxPageSize
	}
	if source.StorageDefaultPageSize > 0 {
		target.StorageDefaultPageSize = source.StorageDefaultPageSize
	}
	target.StorageEnableGrouping = source.StorageEnableGrouping
	if source.StorageMaxGroupSize > 0 {
		target.StorageMaxGroupSize = source.StorageMaxGroupSize
	}
}

func mergeProfileIntoTarget(target, source *Context) {
	if source == nil || source.Profile == emptyValue {
		return
	}
	target.Profile = source.Profile
	if err := sharedProfileLoader.ApplyNamedProfile(target, source.Profile); err != nil {
		copyProfileFieldsFromContext(target, source)
	}
}

// ProcessingMode defines how contexts should be processed
type ProcessingMode int

const (
	// ModeSequential processes contexts one after another in order
	// Each context overrides values from previous contexts
	ModeSequential ProcessingMode = iota

	// ModeHierarchical processes contexts in a tree structure
	// Child contexts inherit from parent and override parent values
	ModeHierarchical

	// ModeHybrid processes contexts with both sequential and hierarchical aspects
	// Siblings are processed sequentially, children inherit hierarchically
	ModeHybrid
)

// contextNodeMissing reports whether n or its embedded Context is nil. Keeps the guard in one place;
// see docs/architecture/WRAPPER_INNER_CONTEXT_GUARD_PATTERN.md (related sites).
func contextNodeMissing(n *ContextNode) bool {
	return n == nil || n.Context == nil
}

// ContextNode represents a single context that can have child contexts
// This enables both sequential and hierarchical processing
type ContextNode struct {
	// The actual context data
	Context *Context

	// Child contexts (for hierarchical processing)
	Children []*ContextNode

	// Processing mode for this node and its children
	Mode ProcessingMode

	// Optional metadata
	Name        string // Name/identifier for this context node
	Description string // Description of what this context represents
	Priority    int    // Priority for ordering (lower = higher priority)
}

// ContextProcessor processes contexts in different modes
type ContextProcessor struct {
	// Default processing mode
	defaultMode ProcessingMode
}

// NewContextProcessor creates a new context processor
func NewContextProcessor(defaultMode ProcessingMode) *ContextProcessor {
	return &ContextProcessor{
		defaultMode: defaultMode,
	}
}

// ProcessSequential processes a list of contexts sequentially
// Each context in the list overrides values from previous contexts
// Now uses the chain-based system for consistent processing
func (cp *ContextProcessor) ProcessSequential(contexts []*Context) (*Context, error) {
	if len(contexts) == 0 {
		return nil, errfmt.Errorf("no contexts provided for sequential processing")
	}

	// Use chain-based system for sequential processing
	chainBuilder := pkgctx.NewChainBuilder()

	// Add contexts in order (each gets incrementing precedence)
	for i, ctx := range contexts {
		if ctx != nil {
			// Convert to chainable context with sequential precedence
			chainable := ToChainableContext(ctx, LayerCommand) // Use command layer for sequential
			// Adjust precedence to maintain order (first = highest precedence)
			if setter, ok := chainable.(interface{ SetPrecedence(int) }); ok {
				setter.SetPrecedence(i) // Lower index = higher precedence
			}
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
}

func (cp *ContextProcessor) processBreadthFirstChain(root *ContextNode) (*Context, error) {
	chainBuilder := pkgctx.NewChainBuilder()
	chain := cp.buildChainFromNode(root, 0, chainBuilder)
	if chain == nil {
		return nil, errfmt.Errorf("failed to build chain from node tree")
	}

	chainResult, validationErrors := chain.ProcessBreadthFirst()
	if len(validationErrors) > 0 {
		// Log validation errors but continue (non-fatal)
	}

	if adapter, ok := chainResult.(*ContextNodeAdapter); ok {
		return adapter.GetNode().Context, nil
	}

	return nil, errfmt.Errorf("chain processing returned unexpected type: %T", chainResult)
}

// ProcessHierarchical processes contexts in a hierarchical tree structure
// Child contexts inherit from parent and override parent values
// Now uses the chain-based system for consistent processing
func (cp *ContextProcessor) ProcessHierarchical(root *ContextNode) (*Context, error) {
	if contextNodeMissing(root) {
		return nil, errfmt.Errorf("root context node is required for hierarchical processing")
	}
	return cp.processBreadthFirstChain(root)
}

// ProcessHybrid processes contexts with both sequential and hierarchical aspects
// Siblings at the same level are processed sequentially
// Children inherit from their parent hierarchically
// Now uses the chain-based system for consistent processing
func (cp *ContextProcessor) ProcessHybrid(root *ContextNode) (*Context, error) {
	if contextNodeMissing(root) {
		return nil, errfmt.Errorf("root context node is required for hybrid processing")
	}
	return cp.processBreadthFirstChain(root)
}

// ProcessNode processes a context node according to its mode
func (cp *ContextProcessor) ProcessNode(node *ContextNode) (*Context, error) {
	if contextNodeMissing(node) {
		return nil, errfmt.Errorf("context node is required")
	}

	mode := node.Mode
	if mode == ModeHybrid {
		// For hybrid, use default mode if not explicitly set
		mode = cp.defaultMode
	}

	switch mode {
	case ModeSequential:
		// For sequential, process children as a list
		contexts := []*Context{node.Context}
		for _, child := range node.Children {
			if child.Context != nil {
				contexts = append(contexts, child.Context)
			}
		}
		return cp.ProcessSequential(contexts)

	case ModeHierarchical:
		return cp.ProcessHierarchical(node)

	case ModeHybrid:
		return cp.ProcessHybrid(node)

	default:
		return nil, errfmt.Errorf("unknown processing mode: %d", mode)
	}
}

// buildChainFromNode converts a ContextNode tree to a ContextChain
// This enables hierarchical and hybrid processing using the chain system
func (cp *ContextProcessor) buildChainFromNode(node *ContextNode, depth int, builder *pkgctx.ChainBuilder) *pkgctx.ContextChain {
	if node == nil {
		return nil
	}

	// Convert node to chainable context
	chainable := ToChainableContextNode(node, depth)
	if chainable == nil {
		return nil
	}

	// Add to chain
	builder.AddContext(chainable)
	chain := builder.GetChain()

	// Find the chain node for this context
	var currentChain *pkgctx.ContextChain
	queue := []*pkgctx.ContextChain{chain}
	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]
		if curr.Context == chainable {
			currentChain = curr
			break
		}
		if curr.Next != nil {
			queue = append(queue, curr.Next)
		}
		queue = append(queue, curr.Children...)
	}

	if currentChain == nil {
		currentChain = chain
	}

	// Add children as nested contexts
	for _, child := range node.Children {
		childChainable := ToChainableContextNode(child, depth+1)
		if childChainable != nil {
			currentChain.AddChild(childChainable)
			// Recursively build chain for child's children
			cp.buildChainFromNode(child, depth+1, builder)
		}
	}

	return chain
}

// mergeContext merges source context into target context
// Values from source override values in target (for sequential processing)
// This implements the precedence logic: later contexts override earlier ones
func (cp *ContextProcessor) mergeContext(target, source *Context) {
	// Merge output preferences (override if set)
	if source.Format != emptyValue {
		target.Format = source.Format
	}
	// Verbose and quiet are boolean flags - if source is true, set target to true
	// This allows later contexts to enable verbose/quiet even if earlier ones didn't
	if source.Verbose {
		target.Verbose = true
	}
	if source.Quiet {
		target.Quiet = true
	}

	// Merge profile (may override format via loaded profile spec)
	if source.Profile != emptyValue {
		mergeProfileIntoTarget(target, source)
	}

	// Merge project context (override if set)
	if source.ProjectRoot != emptyValue {
		target.ProjectRoot = source.ProjectRoot
	}
	if source.PriorityPlan != emptyValue {
		target.PriorityPlan = source.PriorityPlan
	}
	if source.Workstream != emptyValue {
		target.Workstream = source.Workstream
	}
	if source.Milestone != emptyValue {
		target.Milestone = source.Milestone
	}

	// Merge storage context parameters (override if set and > 0 for ints, true for bools)
	if source.StorageMaxPageSize > 0 {
		target.StorageMaxPageSize = source.StorageMaxPageSize
	}
	if source.StorageDefaultPageSize > 0 {
		target.StorageDefaultPageSize = source.StorageDefaultPageSize
	}
	if source.StorageEnableGrouping {
		target.StorageEnableGrouping = true
	}
	if source.StorageMaxGroupSize > 0 {
		target.StorageMaxGroupSize = source.StorageMaxGroupSize
	}

	// Merge cache freshness policy (override if set)
	if source.CacheFreshnessEnabled {
		target.CacheFreshnessEnabled = true
	}
	if len(source.CacheFreshnessTriggers) > 0 {
		target.CacheFreshnessTriggers = source.CacheFreshnessTriggers
	}
	if source.CacheFreshnessMinInterval > 0 {
		target.CacheFreshnessMinInterval = source.CacheFreshnessMinInterval
	}

	if source.ErrorLogOutput != emptyValue {
		target.ErrorLogOutput = source.ErrorLogOutput
	}
	if source.pathResolver != nil {
		target.pathResolver = source.pathResolver
	}

	// Merge layers map (preserve all layers for inspection)
	if target.layers == nil {
		target.layers = make(map[ContextLayer]map[string]any)
	}
	for layer, values := range source.layers {
		if target.layers[layer] == nil {
			target.layers[layer] = make(map[string]any)
		}
		maps.Copy(target.layers[layer], values)
	}
}

// cloneContext creates a deep copy of a context
func (cp *ContextProcessor) cloneContext(ctx *Context) *Context {
	if ctx == nil {
		return &Context{
			layers: make(map[ContextLayer]map[string]any),
		}
	}

	clone := &Context{
		Format:                    ctx.Format,
		Verbose:                   ctx.Verbose,
		Quiet:                     ctx.Quiet,
		Profile:                   ctx.Profile,
		ProjectRoot:               ctx.ProjectRoot,
		PriorityPlan:              ctx.PriorityPlan,
		Workstream:                ctx.Workstream,
		Milestone:                 ctx.Milestone,
		StorageMaxPageSize:        ctx.StorageMaxPageSize,
		StorageDefaultPageSize:    ctx.StorageDefaultPageSize,
		StorageEnableGrouping:     ctx.StorageEnableGrouping,
		StorageMaxGroupSize:       ctx.StorageMaxGroupSize,
		CacheFreshnessEnabled:     ctx.CacheFreshnessEnabled,
		CacheFreshnessTriggers:    make([]string, len(ctx.CacheFreshnessTriggers)),
		CacheFreshnessMinInterval: ctx.CacheFreshnessMinInterval,
		ErrorLogOutput:            ctx.ErrorLogOutput,
		pathResolver:              ctx.pathResolver,
		layers:                    make(map[ContextLayer]map[string]any),
	}
	copy(clone.CacheFreshnessTriggers, ctx.CacheFreshnessTriggers)

	// Deep copy layers
	for layer, values := range ctx.layers {
		clone.layers[layer] = make(map[string]any)
		maps.Copy(clone.layers[layer], values)
	}

	return clone
}

// NewContextNode creates a new context node
func NewContextNode(ctx *Context, mode ProcessingMode) *ContextNode {
	return &ContextNode{
		Context:  ctx,
		Children: make([]*ContextNode, 0),
		Mode:     mode,
		Priority: 0,
	}
}

// AddChild adds a child context node
func (cn *ContextNode) AddChild(child *ContextNode) {
	cn.Children = append(cn.Children, child)
}

// AddChildren adds multiple child context nodes
func (cn *ContextNode) AddChildren(children ...*ContextNode) {
	cn.Children = append(cn.Children, children...)
}

// SortChildren sorts children by priority (lower priority = higher precedence)
func (cn *ContextNode) SortChildren() {
	// Simple insertion sort by priority
	for i := 1; i < len(cn.Children); i++ {
		key := cn.Children[i]
		j := i - 1
		for j >= 0 && cn.Children[j].Priority > key.Priority {
			cn.Children[j+1] = cn.Children[j]
			j--
		}
		cn.Children[j+1] = key
	}
}
