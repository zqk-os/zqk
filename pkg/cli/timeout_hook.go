package cli

// This file has been split into focused modules for better maintainability:
//
// - timeout_hook_types.go: Types, interfaces, and configuration structs
// - timeout_hook_lifecycle.go: Constructors, setters, and lifecycle methods
// - timeout_hook_execution.go: Command wrapping and execution logic
// - timeout_hook_timeout_calculation.go: Timeout calculation and MCP config reading
// - timeout_hook_resettable_timeout.go: Resettable timeout logic for server commands
// - timeout_hook_normalization.go: Command normalization and sanitization
//
// All public APIs remain unchanged - this split is purely organizational.
