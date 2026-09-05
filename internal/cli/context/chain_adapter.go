package context

import (
	"slices"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/outputtypes"
)

const emptyValue = ""

var validOutputFormats = []string{
	string(outputtypes.IDTable),
	string(outputtypes.IDJSON),
	string(outputtypes.IDJSONL),
	string(outputtypes.IDYAML),
	"json-rpc",
	"stream",
	"ids",
}

// ContextChainAdapter adapts internal Context to ChainableContext interface
// This allows the internal context system to work with the chain-based system
type ContextChainAdapter struct {
	ctx        *Context
	precedence int
	depth      int
}

// NewContextChainAdapter creates a new adapter for an internal Context
func NewContextChainAdapter(ctx *Context, precedence, depth int) *ContextChainAdapter {
	return &ContextChainAdapter{
		ctx:        ctx,
		precedence: precedence,
		depth:      depth,
	}
}

// GetContext returns the underlying Context
func (a *ContextChainAdapter) GetContext() *Context {
	return a.ctx
}

// GetPrecedence returns the precedence level for this context
func (a *ContextChainAdapter) GetPrecedence() int {
	return a.precedence
}

// SetPrecedence sets the precedence level for this context
func (a *ContextChainAdapter) SetPrecedence(precedence int) {
	a.precedence = precedence
}

// GetDepth returns the depth of this context in the hierarchy
func (a *ContextChainAdapter) GetDepth() int {
	return a.depth
}

// SetDepth sets the depth of this context in the hierarchy
func (a *ContextChainAdapter) SetDepth(depth int) {
	a.depth = depth
}

// Validate performs validation on this context node
func (a *ContextChainAdapter) Validate() []pkgctx.ValidationError {
	var errors []pkgctx.ValidationError
	if a.ctx == nil {
		errors = append(errors, pkgctx.ValidationError{
			Field:   "Context",
			Message: "context cannot be nil",
			Context: "ContextChainAdapter",
		})
		return errors
	}

	// Validate format
	if a.ctx.Format != emptyValue {
		if !slices.Contains(validOutputFormats, a.ctx.Format) {
			errors = append(errors, pkgctx.ValidationError{
				Field:   "Format",
				Message: "invalid format: " + a.ctx.Format,
				Context: "ContextChainAdapter",
			})
		}
	}

	return errors
}

// Merge merges this context into the target context
// In chain processing, target is the accumulated result (lower precedence contexts)
// and this context (source) has higher precedence and should override target values
func (a *ContextChainAdapter) Merge(target pkgctx.ChainableContext) pkgctx.ChainableContext {
	if target == nil {
		return a
	}

	// If target is also a ContextChainAdapter, merge the underlying contexts
	if targetAdapter, ok := target.(*ContextChainAdapter); ok {
		// Use the existing ContextProcessor merge logic for consistency
		processor := NewContextProcessor(ModeSequential)

		// Start with target (accumulated result from lower precedence contexts)
		var merged *Context
		if targetAdapter.ctx != nil {
			merged = processor.cloneContext(targetAdapter.ctx)
		} else {
			merged = &Context{
				layers: make(map[ContextLayer]map[string]any),
			}
		}

		// Merge source (this context) into merged - source has higher precedence
		if a.ctx != nil {
			processor.mergeContext(merged, a.ctx)
		}

		// Use the higher precedence value for the adapter
		resultPrecedence := a.precedence
		if targetAdapter.precedence < a.precedence {
			resultPrecedence = targetAdapter.precedence
		}

		return NewContextChainAdapter(merged, resultPrecedence, a.depth)
	}

	// Different context types - return this one
	return a
}

// ToChainableContext converts an internal Context to ChainableContext
// Uses layer-based precedence: System=0, User=25, Project=50, Command=75
func ToChainableContext(ctx *Context, layer ContextLayer) pkgctx.ChainableContext {
	if ctx == nil {
		return nil
	}

	// Map layer to precedence
	precedence := 100 // Default
	switch layer {
	case LayerSystem:
		precedence = 0
	case LayerUser:
		precedence = 25
	case LayerProject:
		precedence = 50
	case LayerCommand:
		precedence = 75
	}

	return NewContextChainAdapter(ctx, precedence, 0)
}
