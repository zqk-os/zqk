package scheduler

// ============================================================================
// Extracted Modules
// ============================================================================
//
// This file has been split into focused modules for better maintainability:
//
// - Command creation -> scheduler_commands.go
//   - NewSchedulerCmd, NewStartCmd, NewStopCmd, NewStatusCmd, NewTriggerCmd, NewListCmd
//   - issues-bundle-health -> issues_bundle_health_cmd.go (builder from .zqk/cli/specs/scheduler/issues_bundle_health_command.yaml)
//
// - Configuration -> scheduler_config.go
//   - schedulerConfig type, loadSchedulerConfig
//
// - Core operations -> scheduler_core.go
//   - startScheduler, stopScheduler, showSchedulerStatus, triggerJob
//
// - List operations -> scheduler_list.go
//   - listJobs, outputTable, getString, getBool (json/yaml via internal/cli.FormatOutput)
//
// - History operations -> scheduler_history.go
//   - jobStats type, NewHistoryCmd, showJobHistory, outputHistoryTable
//   - Note: History helpers are in show_history_helpers.go
//
// - Activity operations -> scheduler_activity.go
//   - jobActivityEvent type, NewActivityCmd, showJobActivity, missedTrigger type, detectMissedTriggers, outputActivityStructured, outputActivityTable
//   - Note: Activity helpers are in show_activity_helpers.go and detect_missed_triggers_helpers.go
//
// - Health check operations -> scheduler_health.go
//   - NewHealthCheckCmd, recordExternalHealthMetric
//
// Original file: 1,220 lines
// After split: This file now serves as documentation and module index
