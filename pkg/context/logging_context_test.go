package context

import (
	"testing"
)

func TestLoggingContext_ChainableContext(t *testing.T) {
	t.Parallel()
	// Test that LoggingContext implements ChainableContext
	var _ ChainableContext = (*LoggingContext)(nil)
}

func TestLoggingContext_ChainIntegration(t *testing.T) {
	t.Parallel()
	// Create system logging context (high precedence)
	systemCtx := NewSystemLoggingContext()
	if systemCtx.GetPrecedence() != PrecedenceSystem {
		t.Errorf("Expected precedence %d, got %d", PrecedenceSystem, systemCtx.GetPrecedence())
	}

	// Create human logging context (default precedence)
	humanCtx := NewHumanLoggingContext()
	if humanCtx.GetPrecedence() != PrecedenceDefault {
		t.Errorf("Expected precedence %d, got %d", PrecedenceDefault, humanCtx.GetPrecedence())
	}

	// Build a chain with both contexts
	builder := NewChainBuilder()
	builder.AddContext(systemCtx) // Precedence 0 (higher)
	builder.AddContext(humanCtx)  // Precedence 100 (lower)

	// Build the chain
	result, errors := builder.Build()
	if len(errors) > 0 {
		t.Errorf("Unexpected validation errors: %v", errors)
	}

	// Result should be a LoggingContext
	loggingCtx, ok := result.(*LoggingContext)
	if !ok {
		t.Fatalf("Expected LoggingContext, got %T", result)
	}

	// System context has higher precedence, so profile should be system
	if loggingCtx.Profile != ProfileSystem {
		t.Errorf("Expected profile %s, got %s", ProfileSystem, loggingCtx.Profile)
	}

	// SuppressDebugToStdout should be true (OR'd from both)
	if !loggingCtx.ShouldSuppressDebugToStdout() {
		t.Error("Expected SuppressDebugToStdout to be true")
	}
}

func TestLoggingContext_Merge(t *testing.T) {
	t.Parallel()
	// Create two contexts with different precedence
	systemCtx := NewSystemLoggingContext() // Precedence 0
	humanCtx := NewHumanLoggingContext()   // Precedence 100

	// Merge: system has higher precedence
	merged := systemCtx.Merge(humanCtx).(*LoggingContext)
	if merged.Profile != ProfileSystem {
		t.Errorf("Expected profile %s (from higher precedence), got %s", ProfileSystem, merged.Profile)
	}
	if !merged.ShouldSuppressDebugToStdout() {
		t.Error("Expected SuppressDebugToStdout to be true (OR'd)")
	}

	// Merge: human has lower precedence, but when merged with system, system wins
	merged2 := humanCtx.Merge(systemCtx).(*LoggingContext)
	if merged2.Profile != ProfileSystem {
		t.Errorf("Expected profile %s (from higher precedence), got %s", ProfileSystem, merged2.Profile)
	}
}

func TestLoggingContext_Validate(t *testing.T) {
	t.Parallel()
	// Valid context
	ctx := NewLoggingContext(ProfileHuman)
	errors := ctx.Validate()
	if len(errors) > 0 {
		t.Errorf("Expected no validation errors, got %v", errors)
	}

	// Invalid context (empty profile)
	ctx2 := &LoggingContext{
		Profile:    "",
		precedence: PrecedenceDefault,
		depth:      DepthRoot,
	}
	errors2 := ctx2.Validate()
	if len(errors2) == 0 {
		t.Error("Expected validation error for empty profile")
	}
}
