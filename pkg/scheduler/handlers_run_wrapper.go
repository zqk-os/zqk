package scheduler

// ============================================================================
// Extracted Modules
// ============================================================================
//
// This file has been split into focused modules for better maintainability:
//
// - Types and constructors -> handlers_run_wrapper_types.go
//   - RunWrapperHandler struct
//   - NewRunWrapperHandler, NewRunWrapperHandlerWithProjectRoot
//   - StopNotificationContext, routeAsync
//
// - Main execution logic -> handlers_run_wrapper_execution.go
//   - Execute method with command execution, retry logic, process group management
//   - Command preparation, timeout calculation, progress monitoring
//   - Success/failure handling, log file operations
//
// - Callback operations -> handlers_run_wrapper_callbacks.go
//   - executeCallback, executeCallbackDirect
//   - executeWebhookCallback, executeCommandCallback, executeEventCallback
//
// - Helper functions -> handlers_run_wrapper_helpers.go
//   - isTestCommand, sanitizeTestOutput, extractTestInfo
//
// Original file: 1,155 lines
// After split: This file now serves as documentation and module index
