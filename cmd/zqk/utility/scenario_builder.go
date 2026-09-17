package utility

// ============================================================================
// Extracted Modules
// ============================================================================
//
// This file has been split into focused modules for better maintainability:
//
// - Types and constructor -> scenario_builder_types.go
//   - ScenarioBuilderConfig, ScenarioBuilder types
//   - NewScenarioBuilder constructor
//
// - Configuration methods -> scenario_builder_config.go
//   - WithKinds, WithCount, WithDefaultCount, WithDiversity, WithDefaultDiversity
//   - WithLinkProbability, WithTimeRange, WithStartTime
//
// - Build operations -> scenario_builder_build.go
//   - Build, BuildFromObjects, generateObjectsForKind, generateObject
//   - addKindSpecificFieldsToBuilder, createLinks
//
// - Infrastructure setup -> scenario_builder_infrastructure.go
//   - setupInfrastructure, copyFile, copyDirectory
//   - copyBootstrapConfigFiles, copyScenarioBuilderProfile, copyConfigFile
//
// - Scenario object management -> scenario_builder_scenario.go
//   - createOrUpdateScenarioObject, buildDataGenerationConfig
//   - LoadFromScenarioObject, findScenarioByName
//
// - CLI commands -> scenario_builder_commands.go
//   - NewScenarioBuilderCmd, runScenarioBuilder
//
// Note: Additional helper functions are in separate files:
//   - scenario_builder_data_loader.go - BuildFromDataFile, emitCoordinatorEvent
//   - scenario_builder_copy.go - CopyObjectsFromProject, ScenarioCopyConfig
//   - scenario_builder_helpers.go - Flag parsing, builder creation, handlers
//   - scenario_builder_create_helpers.go - Scenario creation/update helpers
//   - scenario_builder_load_helpers.go - Scenario loading helpers
//   - scenario_builder_copy_helpers.go - Copy operation helpers
//   - scenario_builder_copy_directory_helpers.go - Directory copy helpers
//
// Original file: 1,295 lines
// After split: This file now serves as documentation and module index
