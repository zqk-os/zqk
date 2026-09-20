package mcp

// ErrorCodeCategory represents the category of an error code
type ErrorCodeCategory string

const (
	// ErrorCategoryStandard represents standard JSON-RPC 2.0 error codes
	ErrorCategoryStandard ErrorCodeCategory = "standard"
	// ErrorCategoryClient represents client-side errors (4xx-like)
	ErrorCategoryClient ErrorCodeCategory = "client"
	// ErrorCategoryServer represents server-side errors (5xx-like)
	ErrorCategoryServer ErrorCodeCategory = "server"
)

// ErrorCodeRange defines the ranges for different error code categories
const (
	// Standard JSON-RPC 2.0 error codes (-32768 to -32603)
	// These are defined by the JSON-RPC 2.0 specification
	// The actual standard codes are -32700 to -32603, but we use the full reserved range
	ErrorCodeRangeStandardStart = -32768
	ErrorCodeRangeStandardEnd   = -32603

	// MCP Client Error Codes (-32000 to -32049)
	// These represent client-side errors (similar to HTTP 4xx)
	// Client should fix the request or provide different parameters
	// Note: JSON-RPC 2.0 reserves -32000 to -32099 for implementation-defined server errors,
	// but we use -32000 to -32049 for client errors and -32050 to -32099 for server errors
	ErrorCodeRangeClientStart = -32000
	ErrorCodeRangeClientEnd   = -32049

	// MCP Server Error Codes (-32050 to -32099)
	// These represent server-side errors (similar to HTTP 5xx)
	// Server should handle or log these issues
	ErrorCodeRangeServerStart = -32050
	ErrorCodeRangeServerEnd   = -32099
)

// Standard JSON-RPC 2.0 error codes (as per specification)
const (
	// ParseError (-32700): Invalid JSON was received by the server
	// An error occurred on the server while parsing the JSON text
	ParseError = -32700

	// InvalidRequest (-32600): The JSON sent is not a valid Request object
	// The request doesn't follow the JSON-RPC 2.0 specification
	InvalidRequest = -32600

	// MethodNotFound (-32601): The method does not exist / is not available
	// The method requested doesn't exist or isn't available
	MethodNotFound = -32601

	// InvalidParams (-32602): Invalid method parameter(s)
	// The parameters provided don't match the method's expected parameters
	InvalidParams = -32602

	// InternalError (-32603): Internal JSON-RPC error
	// An internal error occurred in the JSON-RPC implementation
	InternalError = -32603
)

// Client Error Codes (-32000 to -32049)
// These errors indicate problems with the client's request
// The client should modify the request or provide different parameters

// Authentication & Authorization Errors (-32000 to -32009)
const (
	// Unauthenticated (-32000): Authentication required
	// The request requires authentication but none was provided
	Unauthenticated = -32000

	// AuthenticationFailed (-32001): Authentication failed
	// The provided credentials are invalid or expired
	AuthenticationFailed = -32001

	// PermissionDenied (-32002): Permission denied for operation
	// The authenticated user doesn't have permission for this operation
	PermissionDenied = -32002

	// AccessDenied (-32003): Access denied to resource
	// The user doesn't have access to the requested resource
	AccessDenied = -32003

	// Forbidden (-32004): Operation is forbidden
	// The operation is explicitly forbidden (e.g., by policy)
	Forbidden = -32004

	// AccountNotFound (-32005): Account not found
	// The specified account doesn't exist
	AccountNotFound = -32005

	// AccountInactive (-32006): Account is inactive
	// The account exists but is not active
	AccountInactive = -32006

	// RoleMismatch (-32007): Role mismatch
	// The provided role doesn't match the account's assigned roles
	RoleMismatch = -32007
)

// Validation & Request Errors (-32010 to -32019)
const (
	// ValidationError (-32010): Validation error
	// The request data failed validation
	ValidationError = -32010

	// MissingParameter (-32011): Required parameter missing
	// A required parameter was not provided
	MissingParameter = -32011

	// InvalidParameter (-32012): Invalid parameter value
	// A parameter value is invalid (wrong type, out of range, etc.)
	InvalidParameter = -32012

	// ParameterConflict (-32013): Parameter conflict
	// Parameters conflict with each other
	ParameterConflict = -32013

	// InvalidFormat (-32014): Invalid format
	// The request format is invalid (e.g., invalid output format)
	InvalidFormat = -32014

	// UnsupportedOperation (-32015): Unsupported operation
	// The requested operation is not supported
	UnsupportedOperation = -32015
)

// Resource Errors (-32020 to -32029)
const (
	// NotFound (-32020): Resource not found
	// The requested resource doesn't exist
	NotFound = -32020

	// AlreadyExists (-32021): Resource already exists
	// The resource already exists (e.g., duplicate creation)
	AlreadyExists = -32021

	// Conflict (-32022): Resource conflict
	// The operation conflicts with the current state of the resource
	Conflict = -32022

	// Gone (-32023): Resource gone
	// The resource existed but has been permanently removed
	Gone = -32023
)

// State & Lifecycle Errors (-32030 to -32039)
const (
	// InvalidState (-32030): Invalid state
	// The resource is in an invalid state for this operation
	InvalidState = -32030

	// StateTransitionError (-32031): State transition error
	// The requested state transition is not allowed
	StateTransitionError = -32031

	// LifecycleViolation (-32032): Lifecycle violation
	// The operation violates the resource's lifecycle rules
	LifecycleViolation = -32032
)

// Rate Limiting & Quota Errors (-32040 to -32049)
const (
	// RateLimitExceeded (-32040): Rate limit exceeded
	// The client has exceeded the rate limit
	RateLimitExceeded = -32040

	// QuotaExceeded (-32041): Quota exceeded
	// The client has exceeded their quota
	QuotaExceeded = -32041

	// TooManyRequests (-32042): Too many requests
	// The client has made too many requests in a short time
	TooManyRequests = -32042
)

// Server Error Codes (-32050 to -32099)
// These errors indicate problems on the server side
// The server should handle, log, or recover from these

// Server Initialization & Configuration Errors (-32050 to -32059)
const (
	// ServerError (-32050): Generic server error
	// A generic server error occurred
	ServerError = -32050

	// NotInitialized (-32051): Server not initialized
	// The server hasn't been initialized yet
	NotInitialized = -32051

	// AlreadyInitialized (-32052): Server already initialized
	// The server has already been initialized
	AlreadyInitialized = -32052

	// ConfigurationError (-32053): Configuration error
	// There's an error in the server configuration
	ConfigurationError = -32053
)

// Operation & Execution Errors (-32060 to -32079)
const (
	// OperationFailed (-32060): Operation failed
	// The operation failed for an unspecified reason
	OperationFailed = -32060

	// OperationTimeout (-32061): Operation timeout
	// The operation timed out
	OperationTimeout = -32061

	// OperationCancelled (-32062): Operation cancelled
	// The operation was cancelled
	OperationCancelled = -32062

	// ConcurrentModification (-32063): Concurrent modification
	// The resource was modified concurrently
	ConcurrentModification = -32063

	// Deadlock (-32064): Deadlock detected
	// A deadlock was detected during the operation
	Deadlock = -32064
)

// Storage & Data Errors (-32080 to -32089)
const (
	// StorageError (-32080): Storage error
	// An error occurred accessing storage
	StorageError = -32080

	// DataCorruption (-32081): Data corruption
	// Data corruption was detected
	DataCorruption = -32081

	// StorageUnavailable (-32082): Storage unavailable
	// The storage backend is unavailable
	StorageUnavailable = -32082
)

// System & Infrastructure Errors (-32090 to -32099)
const (
	// SystemUnavailable (-32090): System unavailable
	// The system is temporarily unavailable
	SystemUnavailable = -32090

	// ServiceUnavailable (-32091): Service unavailable
	// The service is temporarily unavailable
	ServiceUnavailable = -32091

	// OutOfMemory (-32092): Out of memory
	// The server is out of memory
	OutOfMemory = -32092

	// ResourceExhausted (-32093): Resource exhausted
	// Server resources are exhausted
	ResourceExhausted = -32093
)

// CategorizeErrorCode returns the category of an error code
func CategorizeErrorCode(code int) ErrorCodeCategory {
	// Check standard JSON-RPC 2.0 error codes first
	if code >= ErrorCodeRangeStandardStart && code <= ErrorCodeRangeStandardEnd {
		return ErrorCategoryStandard
	}
	// Check client error codes
	if code >= ErrorCodeRangeClientStart && code <= ErrorCodeRangeClientEnd {
		return ErrorCategoryClient
	}
	// Check server error codes
	if code >= ErrorCodeRangeServerStart && code <= ErrorCodeRangeServerEnd {
		return ErrorCategoryServer
	}
	// Codes between -32604 and -31999 are in the gap between standard and client ranges
	// These are treated as server errors for safety (unknown codes default to server)
	return ErrorCategoryServer
}

// IsClientError returns true if the error code represents a client error
func IsClientError(code int) bool {
	return CategorizeErrorCode(code) == ErrorCategoryClient
}

// IsServerError returns true if the error code represents a server error
func IsServerError(code int) bool {
	return CategorizeErrorCode(code) == ErrorCategoryServer
}

// IsStandardError returns true if the error code is a standard JSON-RPC 2.0 error
func IsStandardError(code int) bool {
	return CategorizeErrorCode(code) == ErrorCategoryStandard
}

// IsCriticalErrorCode checks if an error code should be considered critical
// Critical errors typically warrant notifications or special handling
// By default, server errors and certain client errors are considered critical
func IsCriticalErrorCode(code int) bool {
	category := CategorizeErrorCode(code)

	// All server errors are critical
	if category == ErrorCategoryServer {
		return true
	}

	// Certain client errors are critical
	if category == ErrorCategoryClient {
		switch code {
		case Unauthenticated, AuthenticationFailed, PermissionDenied, AccessDenied,
			Forbidden, AccountNotFound, AccountInactive, RateLimitExceeded,
			QuotaExceeded, TooManyRequests:
			return true
		}
	}

	// Standard JSON-RPC errors are generally not critical (expected validation errors)
	return false
}
