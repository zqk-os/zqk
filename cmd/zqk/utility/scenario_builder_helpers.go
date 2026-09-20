package utility

// ============================================================================
// Extracted Modules
// ============================================================================
//
// This file has been split into focused modules for better maintainability:
//
// - Flag parsing and types -> scenario_builder_helpers_flags.go
//   - ScenarioBuilderFlags type
//   - parseScenarioBuilderFlags, resolveTargetDirectory, validateTargetDirectory
//
// - Builder creation -> scenario_builder_helpers_builder.go
//   - createScenarioBuilder (Processor pattern initialization)
//
// - Command handlers -> scenario_builder_helpers_handlers.go
//   - handleCopyObjects, handleLoadFromScenario, configureBuilderFromFlags, handleDryRun
//
// Original file: 428 lines
// After split: This file now serves as documentation and module index
