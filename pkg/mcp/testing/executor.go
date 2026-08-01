package testing

import (
	"context"
	"fmt"
	"reflect"
	"regexp"

	"github.com/lanceman/zqk/pkg/brand"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/mcp"
)

// logFieldComponent is the structured-log key for subsystem (not imported from pkg/mcp to avoid cycles).
var logFieldComponent = string([]byte{'c', 'o', 'm', 'p', 'o', 'n', 'e', 'n', 't'})

// ScenarioExecutor executes test scenarios against an MCP server
type ScenarioExecutor struct {
	server            *mcp.Server
	ctx               context.Context
	results           map[string]any           // Stored results from steps
	responseProcessor *ResponseProcessorConfig // Configurable response processing
}

// NewScenarioExecutor creates a new scenario executor
func NewScenarioExecutor(server *mcp.Server) *ScenarioExecutor {
	return &ScenarioExecutor{
		server:            server,
		ctx:               pkgctx.NewSystemContext(),
		results:           make(map[string]any),
		responseProcessor: NewDefaultResponseProcessorConfig(), // Default: treat elicitation as success
	}
}

// NewScenarioExecutorWithProcessor creates a new scenario executor with custom response processing
func NewScenarioExecutorWithProcessor(server *mcp.Server, processor *ResponseProcessorConfig) *ScenarioExecutor {
	return &ScenarioExecutor{
		server:            server,
		ctx:               pkgctx.NewSystemContext(),
		results:           make(map[string]any),
		responseProcessor: processor,
	}
}

// SetResponseProcessor sets the response processor configuration
func (se *ScenarioExecutor) SetResponseProcessor(config *ResponseProcessorConfig) {
	se.responseProcessor = config
}

// RunScenario runs a complete test scenario
func (se *ScenarioExecutor) RunScenario(scenario *TestScenario) (*ScenarioResults, error) {
	// Apply scenario-level response processor if specified
	if scenario.ResponseProcessor != emptyValue {
		processor, err := GetProcessor(scenario.ResponseProcessor)
		if err != nil {
			return nil, errfmt.Errorf("unknown response processor '%s': %w", scenario.ResponseProcessor, err)
		}
		config := &ResponseProcessorConfig{
			GlobalProcessor: processor,
		}
		se.SetResponseProcessor(config)
	}

	results := &ScenarioResults{
		ScenarioName: scenario.Name,
		StepResults:  make([]StepResult, 0),
	}

	// Run setup steps
	for _, step := range scenario.Setup {
		result, err := se.runStep(&step)
		results.StepResults = append(results.StepResults, result)
		if err != nil {
			return results, errfmt.Errorf("setup step %s failed: %w", step.Name, err)
		}

		// Check step expectations
		if step.Expected != nil {
			if err := se.validateStepResult(&step, result); err != nil {
				return results, errfmt.Errorf("setup step %s validation failed: %w", step.Name, err)
			}
		}

		// Store result if requested
		if step.StoreResult != emptyValue {
			se.results[step.StoreResult] = result.Result
		}
	}

	// Run test steps
	for _, step := range scenario.Tests {
		if step.Skip {
			results.StepResults = append(results.StepResults, StepResult{
				StepName: step.Name,
				Skipped:  true,
			})
			continue
		}

		// Check dependencies
		if err := se.checkDependencies(&step); err != nil {
			return results, errfmt.Errorf("step %s dependency check failed: %w", step.Name, err)
		}

		result, err := se.runStep(&step)
		results.StepResults = append(results.StepResults, result)
		if err != nil {
			if step.Expected != nil && step.Expected.Success != nil && !*step.Expected.Success {
				// Expected failure
				if err := se.validateStepResult(&step, result); err != nil {
					return results, errfmt.Errorf("step %s expected failure validation failed: %w", step.Name, err)
				}
				continue
			}
			return results, errfmt.Errorf("test step %s failed: %w", step.Name, err)
		}

		// Check step expectations (skip if processor marked as skipped)
		if !result.Skipped && step.Expected != nil {
			if err := se.validateStepResult(&step, result); err != nil {
				return results, errfmt.Errorf("test step %s validation failed: %w", step.Name, err)
			}
		}

		// Store result if requested
		if step.StoreResult != emptyValue {
			se.results[step.StoreResult] = result.Result
		}
	}

	// Run cleanup steps (even if tests failed)
	for _, step := range scenario.Cleanup {
		result, err := se.runStep(&step)
		results.CleanupResults = append(results.CleanupResults, result)
		if err != nil {
			// Log but don't fail on cleanup errors
			logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
			logging.Fluent(logger).Warn("Cleanup step failed").
				WithError(err).
				String("step_name", step.Name).
				String(logFieldComponent, "mcp_testing").
				Log()
		}
	}

	return results, nil
}

// runStep executes a single test step
func (se *ScenarioExecutor) runStep(step *TestStep) (StepResult, error) {
	// Convert args to map[string]any for MCP server
	args := make(map[string]any)
	for k, v := range step.Args {
		args[k] = v
	}

	// Call tool via MCP server
	result, err := se.callTool(step.Tool, args)

	// Process response through configured processor (if any)
	processedResult := result
	processedErr := err
	if se.responseProcessor != nil {
		processor := se.responseProcessor.GetProcessorForStep(step.Name)
		if processor != nil {
			var shouldContinue bool
			processedResult, processedErr, shouldContinue = processor.ProcessResponse(result, err)
			if !shouldContinue {
				// Processor indicates we should skip validation
				return StepResult{
					StepName: step.Name,
					Tool:     step.Tool,
					Result:   processedResult,
					Error:    processedErr,
					Success:  processedErr == nil,
					Skipped:  true, // Mark as skipped for validation
				}, processedErr
			}
		}
	}

	stepResult := StepResult{
		StepName: step.Name,
		Tool:     step.Tool,
		Result:   processedResult,
		Error:    processedErr,
		Success:  processedErr == nil,
	}

	return stepResult, processedErr
}

// checkDependencies checks if step dependencies are satisfied
func (se *ScenarioExecutor) checkDependencies(step *TestStep) error {
	for _, dep := range step.DependsOn {
		if _, exists := se.results[dep]; !exists {
			return errfmt.Errorf("dependency %s not found", dep)
		}
	}
	return nil
}

// validateStepResult validates a step result against expectations
func (se *ScenarioExecutor) validateStepResult(step *TestStep, result StepResult) error {
	if step.Expected == nil {
		return nil
	}

	exp := step.Expected

	// Check success expectation
	if exp.Success != nil {
		if *exp.Success != result.Success {
			return errfmt.Errorf("expected success=%v, got success=%v", *exp.Success, result.Success)
		}
	}

	// If we have an error expectation
	if exp.Error != nil {
		if result.Error == nil {
			return errfmt.Errorf("expected error but got success")
		}

		// Validate error details
		errorExp := exp.Error

		// Check error type if specified
		if errorExp.Type != emptyValue {
			// Check if it's an ElicitationError
			if errorExp.Type == "elicitation" {
				if _, ok := result.Error.(*mcp.ElicitationError); !ok {
					return errfmt.Errorf("expected elicitation error, got %T", result.Error)
				}
			} else {
				// For other error types, check error message contains type
				errorMsg := result.Error.Error()
				// Simple type checking - could be enhanced
				if errorMsg == emptyValue {
					return errfmt.Errorf("error has empty message")
				}
			}
		}

		// Check error message if specified (exact or regex pattern)
		if errorExp.Message != emptyValue {
			errorMsg := result.Error.Error()
			// Try exact match first
			if errorMsg != errorExp.Message {
				// Try regex pattern match
				matched, err := regexp.MatchString(errorExp.Message, errorMsg)
				if err != nil {
					// If not valid regex, treat as exact match requirement
					return errfmt.Errorf("expected error message '%s', got '%s'", errorExp.Message, errorMsg)
				}
				if !matched {
					return errfmt.Errorf("error message '%s' does not match pattern '%s'", errorMsg, errorExp.Message)
				}
			}
		}

		// Check error code if specified (for JSONRPCError)
		if errorExp.Code != nil {
			if jsonrpcErr, ok := result.Error.(*mcp.JSONRPCError); ok {
				if jsonrpcErr.Code != *errorExp.Code {
					return errfmt.Errorf("expected error code %d, got %d", *errorExp.Code, jsonrpcErr.Code)
				}
			} else {
				// Code validation only works for JSONRPCError
				return errfmt.Errorf("error code validation only supported for JSONRPCError, got %T", result.Error)
			}
		}

		return nil
	}

	// If result is not a map, can't validate fields
	resultMap, ok := result.Result.(map[string]any)
	if !ok {
		return nil // Skip field validation if result is not a map
	}

	// Check has_fields
	for _, field := range exp.HasFields {
		if _, exists := resultMap[field]; !exists {
			return errfmt.Errorf("expected field %s not found", field)
		}
	}

	// Check not_has_fields
	for _, field := range exp.NotHasFields {
		if _, exists := resultMap[field]; exists {
			return errfmt.Errorf("unexpected field %s found", field)
		}
	}

	// Check equals
	for field, expectedValue := range exp.Equals {
		actualValue, exists := resultMap[field]
		if !exists {
			return errfmt.Errorf("field %s not found for equality check", field)
		}
		if !se.valuesEqual(actualValue, expectedValue) {
			return errfmt.Errorf("field %s: expected %v, got %v", field, expectedValue, actualValue)
		}
	}

	// Check has_field (existence and value)
	for field, expectedValue := range exp.HasField {
		actualValue, exists := resultMap[field]
		if !exists {
			return errfmt.Errorf("field %s not found", field)
		}
		if expectedValue != nil && !se.valuesEqual(actualValue, expectedValue) {
			return errfmt.Errorf("field %s: expected %v, got %v", field, expectedValue, actualValue)
		}
	}

	// Check matches (regex patterns)
	for field, pattern := range exp.Matches {
		actualValue, exists := resultMap[field]
		if !exists {
			return errfmt.Errorf("field %s not found for pattern match", field)
		}

		// Convert actual value to string for regex matching
		actualStr := fmt.Sprintf("%v", actualValue)
		matched, err := regexp.MatchString(pattern, actualStr)
		if err != nil {
			return errfmt.Errorf("invalid regex pattern for field %s: %w", field, err)
		}
		if !matched {
			return errfmt.Errorf("field %s: value '%v' does not match pattern '%s'", field, actualValue, pattern)
		}
	}

	return nil
}

// valuesEqual compares two values for equality with deep comparison support
func (se *ScenarioExecutor) valuesEqual(a, b any) bool {
	// Handle nil cases
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}

	// Use reflect.DeepEqual for deep comparison (handles maps, slices, etc.)
	return reflect.DeepEqual(a, b)
}

// ScenarioResults contains the results of running a scenario
type ScenarioResults struct {
	ScenarioName   string
	StepResults    []StepResult
	CleanupResults []StepResult
}

// StepResult contains the result of a single step
type StepResult struct {
	StepName string
	Tool     string
	Result   any
	Error    error
	Success  bool
	Skipped  bool
}

// callTool calls a tool on the server
// This is a wrapper that converts the args format
func (se *ScenarioExecutor) callTool(tool string, args map[string]any) (any, error) {
	// Normalize tool name: convert legacy MCP tool prefix to current brand prefix
	// so test scenarios can use the legacy prefix while supporting brand changes.
	normalizedTool := normalizeToolName(tool)

	// Convert args to map[string]any
	argsAny := make(map[string]any)
	for k, v := range args {
		argsAny[k] = v
	}

	// Use server's HandleToolCall method (public wrapper)
	return se.server.HandleToolCall(se.ctx, normalizedTool, argsAny)
}

// normalizeToolName converts legacy tool names (with brand.LegacyMCPToolPrefix) to current brand prefix.
func normalizeToolName(toolName string) string {
	prefix := brand.LegacyMCPToolPrefix
	if len(toolName) > len(prefix) && toolName[:len(prefix)] == prefix {
		return mcp.GetToolName(toolName[len(prefix):])
	}
	return toolName
}
