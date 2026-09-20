package mcp

import "fmt"

// ElicitationParam represents a parameter that needs to be elicited from the client
type ElicitationParam struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Type        string `json:"type"` // "string", "number", "boolean", "array", "object"
	Required    bool   `json:"required,omitempty"`
	// Optional: provide choices or examples
	Choices []any `json:"choices,omitempty"`
	Example any   `json:"example,omitempty"`
}

// ElicitationError represents an error that requires elicitation
// This is returned when a tool call is missing required parameters
type ElicitationError struct {
	Message    string             `json:"message"`
	Parameters []ElicitationParam `json:"parameters"`
	Data       map[string]any     `json:"data,omitempty"` // Additional context data (e.g., session_id)
}

// Error implements the error interface
func (e *ElicitationError) Error() string {
	return e.Message
}

// NewElicitationError creates a new elicitation error
func NewElicitationError(message string, params []ElicitationParam) *ElicitationError {
	return &ElicitationError{
		Message:    message,
		Parameters: params,
	}
}

// NewElicitationErrorWithData creates a new elicitation error with additional context data
func NewElicitationErrorWithData(message string, params []ElicitationParam, data map[string]any) *ElicitationError {
	return &ElicitationError{
		Message:    message,
		Parameters: params,
		Data:       data,
	}
}

// NewElicitationErrorForMissingParams creates an elicitation error for missing parameters
func NewElicitationErrorForMissingParams(missingParams []string, toolName string) *ElicitationError {
	params := make([]ElicitationParam, 0, len(missingParams))
	for _, name := range missingParams {
		params = append(params, ElicitationParam{
			Name:        name,
			Description: fmt.Sprintf("Missing required parameter: %s", name),
			Type:        "string", // Default type, can be refined
			Required:    true,
		})
	}
	return &ElicitationError{
		Message:    fmt.Sprintf("Missing required parameters for %s. Please provide: %v", toolName, missingParams),
		Parameters: params,
	}
}

// ValidateToolParams validates tool parameters and returns an elicitation error if required params are missing
func ValidateToolParams(params map[string]any, requiredParams []string, toolName string) error {
	missing := make([]string, 0)
	for _, required := range requiredParams {
		if _, exists := params[required]; !exists {
			missing = append(missing, required)
		}
	}
	if len(missing) > 0 {
		return NewElicitationErrorForMissingParams(missing, toolName)
	}
	return nil
}

// ElicitParam creates an elicitation parameter with optional choices or example
func ElicitParam(name, description, paramType string, required bool) ElicitationParam {
	return ElicitationParam{
		Name:        name,
		Description: description,
		Type:        paramType,
		Required:    required,
	}
}

// ElicitParamWithChoices creates an elicitation parameter with choices
func ElicitParamWithChoices(name, description, paramType string, required bool, choices []any) ElicitationParam {
	return ElicitationParam{
		Name:        name,
		Description: description,
		Type:        paramType,
		Required:    required,
		Choices:     choices,
	}
}

// ElicitParamWithExample creates an elicitation parameter with an example
func ElicitParamWithExample(name, description, paramType string, required bool, example any) ElicitationParam {
	return ElicitationParam{
		Name:        name,
		Description: description,
		Type:        paramType,
		Required:    required,
		Example:     example,
	}
}
