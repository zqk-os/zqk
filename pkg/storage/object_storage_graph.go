package storage

// Types and constructors are now in object_storage_graph_types.go
// Helper functions are now in object_storage_graph_helpers.go
// Validation functions are now in object_storage_graph_validation.go
// CRUD operations are now in object_storage_graph_crud.go
// List/Query operations are now in object_storage_graph_list.go
// Transaction support is now in object_storage_graph_transaction.go
// Bulk operations are now in object_storage_graph_bulk.go
// Graph traversal operations are now in object_storage_graph_traversal.go

import "context"

func (g *GraphObjectStorage) Shutdown(ctx context.Context) error { return nil }
