package testing

import (
	"github.com/lanceman/zqk/pkg/mcp"
	"github.com/lanceman/zqk/pkg/objects"
)

// ResponseProcessor transforms or filters tool call responses/errors before validation
// This allows test scenarios to customize how responses are handled
type ResponseProcessor interface {
	// ProcessResponse transforms a response/error before validation
	// Returns: (transformed result, transformed error, should continue)
	// If shouldContinue is false, validation is skipped
	ProcessResponse(result any, err error) (any, error, bool)
}

// ResponseProcessorFunc is a function type that implements ResponseProcessor
type ResponseProcessorFunc func(result any, err error) (any, error, bool)

// ProcessResponse implements ResponseProcessor
func (f ResponseProcessorFunc) ProcessResponse(result any, err error) (any, error, bool) {
	return f(result, err)
}

// ElicitationToSuccessProcessor converts ElicitationErrors to success responses
// This allows tests to treat elicitation as expected behavior, not errors
type ElicitationToSuccessProcessor struct{}

// ProcessResponse converts ElicitationError to a success response with elicitation data
func (p *ElicitationToSuccessProcessor) ProcessResponse(result any, err error) (any, error, bool) {
	if err == nil {
		return result, nil, true
	}

	// Check if it's an ElicitationError
	if elicitationErr, ok := err.(*mcp.ElicitationError); ok {
		// Convert to success response with elicitation data
		successResult := map[string]any{
			"elicitation":              true,
			"message":                  elicitationErr.Message,
			objects.FieldKeyParameters: elicitationErr.Parameters,
			"elicitation_data":         elicitationErr.Data,
		}
		return successResult, nil, true
	}

	// Other errors pass through unchanged
	return result, err, true
}

// IgnoreErrorsProcessor ignores all errors, treating them as success
type IgnoreErrorsProcessor struct{}

// ProcessResponse ignores errors and returns success
func (p *IgnoreErrorsProcessor) ProcessResponse(result any, err error) (any, error, bool) {
	if err != nil {
		// Return error info as result data, but no error
		errorResult := map[string]any{
			"error_ignored":   true,
			"error_message":   err.Error(),
			"original_result": result,
		}
		return errorResult, nil, true
	}
	return result, nil, true
}

// ErrorTranslatorProcessor translates errors to different types or formats
type ErrorTranslatorProcessor struct {
	TranslateFunc func(error) (any, error)
}

// ProcessResponse translates errors using the provided function
func (p *ErrorTranslatorProcessor) ProcessResponse(result any, err error) (any, error, bool) {
	if err == nil {
		return result, nil, true
	}

	translatedResult, translatedErr := p.TranslateFunc(err)
	return translatedResult, translatedErr, true
}

// ChainedResponseProcessor chains multiple processors together
type ChainedResponseProcessor struct {
	Processors []ResponseProcessor
}

// ProcessResponse processes the response through all processors in sequence
func (p *ChainedResponseProcessor) ProcessResponse(result any, err error) (any, error, bool) {
	currentResult := result
	currentErr := err
	shouldContinue := true

	for _, processor := range p.Processors {
		if !shouldContinue {
			break
		}
		currentResult, currentErr, shouldContinue = processor.ProcessResponse(currentResult, currentErr)
	}

	return currentResult, currentErr, shouldContinue
}

// ConditionalProcessor applies a processor based on a condition
type ConditionalProcessor struct {
	Condition func(result any, err error) bool
	Processor ResponseProcessor
	Otherwise ResponseProcessor // Optional: processor to use if condition is false
}

// ProcessResponse applies processor based on condition
func (p *ConditionalProcessor) ProcessResponse(result any, err error) (any, error, bool) {
	if p.Condition(result, err) {
		return p.Processor.ProcessResponse(result, err)
	}
	if p.Otherwise != nil {
		return p.Otherwise.ProcessResponse(result, err)
	}
	return result, err, true
}

// ResponseProcessorConfig allows configuring response processing per scenario or step
type ResponseProcessorConfig struct {
	// Processor to use for all steps in the scenario
	GlobalProcessor ResponseProcessor

	// Step-specific processors (keyed by step name)
	StepProcessors map[string]ResponseProcessor

	// Default processor to use if no step-specific processor is found
	DefaultProcessor ResponseProcessor
}

// GetProcessorForStep returns the appropriate processor for a step
func (c *ResponseProcessorConfig) GetProcessorForStep(stepName string) ResponseProcessor {
	// Check for step-specific processor
	if processor, ok := c.StepProcessors[stepName]; ok {
		return processor
	}

	// Check for global processor
	if c.GlobalProcessor != nil {
		return c.GlobalProcessor
	}

	// Use default processor (may be nil)
	return c.DefaultProcessor
}

// NewDefaultResponseProcessorConfig creates a default configuration
// that treats ElicitationErrors as success (common case for interactive tools)
func NewDefaultResponseProcessorConfig() *ResponseProcessorConfig {
	return &ResponseProcessorConfig{
		DefaultProcessor: &ElicitationToSuccessProcessor{},
	}
}

// NewNoOpResponseProcessorConfig creates a config that passes responses through unchanged
func NewNoOpResponseProcessorConfig() *ResponseProcessorConfig {
	return &ResponseProcessorConfig{
		DefaultProcessor: ResponseProcessorFunc(func(result any, err error) (any, error, bool) {
			return result, err, true
		}),
	}
}
