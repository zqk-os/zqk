package storage

// ============================================================================
// Extracted Modules
// ============================================================================
//
// This file has been split into focused modules for better maintainability:
//
// - Types, callbacks, and rules -> audit_event_buffer_types.go
//   - AuditBufferFlushEventCallback, FlushProgress, FlushErrorCallback types
//   - AuditEventBuffer, AggregationGroup, AggregationRule structs
//   - DefaultAggregationRules function
//   - Global callback management (SetAuditBufferFlushEventCallback, getAuditBufferFlushEventCallback)
//
// - Initialization and configuration -> audit_event_buffer_initialization.go
//   - Global buffer management (GetGlobalAuditEventBuffer, InitializeGlobalBufferWithConfig)
//   - Constructor (NewAuditEventBuffer)
//   - Configuration setters (SetEnabled, IsEnabled, SetFileStorage, SetFlushErrorCallback, SetFlushProgressChannel, SetProjectRoot, SetSecurityContext, getContext)
//
// - Event handling -> audit_event_buffer_events.go
//   - ShouldAggregate, AddEvent, generateAggregationKey
//
// - Flush operations -> audit_event_buffer_flush.go
//   - periodicFlush, Flush, flushGroup, emitFlushEvent, flushGroupLocked
//
// - Lifecycle and stats -> audit_event_buffer_lifecycle.go
//   - Shutdown, InitiateShutdown, Drain, IsDrained, GetPendingCount, GetName, IsCritical, GetBufferStats
//
// Original file: 1,091 lines
// After split: This file now serves as documentation and module index
