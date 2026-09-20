package mcp

import "maps"

// ErrorResponseBuilder provides a fluent API for building JSONRPCError responses
// This standardizes error responses and makes common errors easily accessible
type ErrorResponseBuilder struct {
	code    int
	message string
	data    map[string]any
}

// NewErrorResponseBuilder creates a new error response builder
func NewErrorResponseBuilder(code int, message string) *ErrorResponseBuilder {
	return &ErrorResponseBuilder{
		code:    code,
		message: message,
		data:    make(map[string]any),
	}
}

// WithData adds data to the error response
func (b *ErrorResponseBuilder) WithData(key string, value any) *ErrorResponseBuilder {
	if b.data == nil {
		b.data = make(map[string]any)
	}
	b.data[key] = value
	return b
}

// WithDataMap adds multiple data fields to the error response
func (b *ErrorResponseBuilder) WithDataMap(data map[string]any) *ErrorResponseBuilder {
	if b.data == nil {
		b.data = make(map[string]any)
	}
	maps.Copy(b.data, data)
	return b
}

// Build creates the JSONRPCError
func (b *ErrorResponseBuilder) Build() *JSONRPCError {
	var data any
	if len(b.data) > 0 {
		data = b.data
	}
	return &JSONRPCError{
		Code:    b.code,
		Message: b.message,
		Data:    data,
	}
}

// Standard Error Response Builders
// These provide commonly used error responses with consistent structure

// NewMissingParameterError creates an error for missing required parameters
func NewMissingParameterError(parameter string, function string) *JSONRPCError {
	return NewErrorResponseBuilder(MissingParameter, "Required parameter missing").
		WithData("parameter", parameter).
		WithData("function", function).
		Build()
}

// NewInvalidParameterError creates an error for invalid parameter values
func NewInvalidParameterError(parameter string, reason string, function string) *JSONRPCError {
	return NewErrorResponseBuilder(InvalidParameter, "Invalid parameter value").
		WithData("parameter", parameter).
		WithData("reason", reason).
		WithData("function", function).
		Build()
}

// NewPermissionDeniedError creates an error for permission denied
func NewPermissionDeniedError(operation string, reason string) *JSONRPCError {
	return NewErrorResponseBuilder(PermissionDenied, "Permission denied for operation").
		WithData("operation", operation).
		WithData("reason", reason).
		Build()
}

// NewAccessDeniedError creates an error for access denied to a resource
func NewAccessDeniedError(resource string, reason string) *JSONRPCError {
	return NewErrorResponseBuilder(AccessDenied, "Access denied to resource").
		WithData("resource", resource).
		WithData("reason", reason).
		Build()
}

// NewNotFoundError creates an error for resource not found
func NewNotFoundError(resourceType string, resourceID string) *JSONRPCError {
	return NewErrorResponseBuilder(NotFound, "Resource not found").
		WithData("resource_type", resourceType).
		WithData("resource_id", resourceID).
		Build()
}

// NewConfigurationError creates an error for configuration issues
func NewConfigurationError(message string, details map[string]any) *JSONRPCError {
	builder := NewErrorResponseBuilder(ConfigurationError, message)
	if details != nil {
		builder = builder.WithDataMap(details)
	}
	return builder.Build()
}

// NewValidationError creates an error for validation failures
func NewValidationError(field string, reason string, function string) *JSONRPCError {
	return NewErrorResponseBuilder(ValidationError, "Validation error").
		WithData("field", field).
		WithData("reason", reason).
		WithData("function", function).
		Build()
}

// NewOperationFailedError creates an error for failed operations
func NewOperationFailedError(operation string, reason string) *JSONRPCError {
	return NewErrorResponseBuilder(OperationFailed, "Operation failed").
		WithData("operation", operation).
		WithData("reason", reason).
		Build()
}

// NewServerError creates a generic server error
func NewServerError(message string, details map[string]any) *JSONRPCError {
	builder := NewErrorResponseBuilder(ServerError, message)
	if details != nil {
		builder = builder.WithDataMap(details)
	}
	return builder.Build()
}

// NewNotInitializedError creates an error for server not initialized
func NewNotInitializedError(operation string) *JSONRPCError {
	return NewErrorResponseBuilder(NotInitialized, "Server not initialized").
		WithData("operation", operation).
		Build()
}

// NewAlreadyExistsError creates an error for resources that already exist
func NewAlreadyExistsError(resourceType string, resourceID string) *JSONRPCError {
	return NewErrorResponseBuilder(AlreadyExists, "Resource already exists").
		WithData("resource_type", resourceType).
		WithData("resource_id", resourceID).
		Build()
}

// NewInvalidStateError creates an error for invalid resource state
func NewInvalidStateError(resourceType string, currentState string, requiredState string) *JSONRPCError {
	return NewErrorResponseBuilder(InvalidState, "Resource is in invalid state").
		WithData("resource_type", resourceType).
		WithData("current_state", currentState).
		WithData("required_state", requiredState).
		Build()
}

// NewStateTransitionError creates an error for invalid state transitions
func NewStateTransitionError(resourceType string, fromState string, toState string, reason string) *JSONRPCError {
	return NewErrorResponseBuilder(StateTransitionError, "Invalid state transition").
		WithData("resource_type", resourceType).
		WithData("from_state", fromState).
		WithData("to_state", toState).
		WithData("reason", reason).
		Build()
}

// NewAuthenticationFailedError creates an error for authentication failures
func NewAuthenticationFailedError(reason string, details map[string]any) *JSONRPCError {
	builder := NewErrorResponseBuilder(AuthenticationFailed, "Authentication failed").
		WithData("reason", reason)
	if details != nil {
		builder = builder.WithDataMap(details)
	}
	return builder.Build()
}

// NewUnauthenticatedError creates an error for unauthenticated requests
func NewUnauthenticatedError(operation string) *JSONRPCError {
	return NewErrorResponseBuilder(Unauthenticated, "Authentication required").
		WithData("operation", operation).
		Build()
}

// NewInvalidFormatError creates an error for invalid format
func NewInvalidFormatError(format string, reason string, context string) *JSONRPCError {
	return NewErrorResponseBuilder(InvalidFormat, "Invalid format").
		WithData("format", format).
		WithData("reason", reason).
		WithData("context", context).
		Build()
}
