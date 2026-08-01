package storage

// ============================================================================
// Extracted Modules
// ============================================================================
//
// This file has been split into focused modules for better maintainability:
//
// - Types, constants, and callbacks -> cas_index_write_queue_types.go
//   - ListingIndexBatchEventCallback type
//   - indexUpdateRequest, indexQueue, ListingIndexWriteQueue structs
//   - GetGlobalListingIndexWriteQueue, SetProjectRoot, GetProjectRoot, SetStorage, GetStorage
//   - indexQueue.getProjectRoot, indexQueue.getStorage
//
// - Queue management -> cas_index_write_queue_management.go
//   - getOrCreateQueue, EnqueueUpdate, EnqueueUpdateWithCallback, EnqueueUpdateWithOperationCallback, enqueue
//
// - Worker lifecycle -> cas_index_write_queue_worker.go
//   - wakeWorkerIfNeeded, startWorker, processBatch, signalIndexUpdateCompletion
//
// - Lifecycle operations -> cas_index_write_queue_lifecycle.go
//   - Shutdown, InitiateShutdown, Drain, IsDrained, FlushKind, FlushAll
//
// - Stats and helpers -> cas_index_write_queue_stats.go
//   - GetQueueStats, GetPendingCount, GetName, IsCritical
//
// Original file: 1,102 lines
// After split: This file now serves as documentation and module index
