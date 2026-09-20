package filecas

// ============================================================================
// Extracted Modules
// ============================================================================
//
// This file has been split into focused modules for better maintainability:
//
// - Types, constructors, and callbacks -> content_addressable_storage_types.go
//   - OrphanCleanupCallback, PostSyncCallback types
//   - ContentAddressableStorage, IDIndex structs
//   - NewContentAddressableStorage constructor
//   - SetOrphanCleanupCallback, SetPostSyncCallback, SetOperationCallback
//   - IDIndex.ListIDs, IDIndex.SnapshotMappings, IDIndex.loadLocked
//
// - CRUD operations -> content_addressable_storage_crud.go
//   - Create, Read, Update, UpdateWithIDChange, Delete
//
// - File operations and helpers -> content_addressable_storage_file.go
//   - findHashFile, WriteFileWithSync
//   - GetHashForID, GetFilePathForID, ListIDs, GetAllMappings
//
// - Index operations -> content_addressable_storage_index.go
//   - IDIndex.Save, saveMappingsLocked, saveMappings, saveMappingsNoLock
//   - IDIndex.GetHash, SetMapping, RemoveMapping
//
// Original file: 1,174 lines
// After split: This file now serves as documentation and module index

// Types, constructors, and callback setters have been moved to content_addressable_storage_types.go
// CRUD operations (Create, Read, Update, UpdateWithIDChange, Delete) have been moved to content_addressable_storage_crud.go
// File operations (findHashFile, WriteFileWithSync) and helper methods (GetHashForID, GetFilePathForID, ListIDs, GetAllMappings) have been moved to content_addressable_storage_file.go
// Index operations (Save, saveMappingsLocked, saveMappings, saveMappingsNoLock, GetHash, SetMapping, RemoveMapping) have been moved to content_addressable_storage_index.go
