package storage

import (
	"time"
)

// timeRange represents a time range for optimized bucketed storage queries
type timeRange struct {
	start time.Time
	end   time.Time
}

// This file previously contained List operations that have been split into separate files:
// - Main List function -> object_storage_file_list_main.go
// - Filtering functions -> object_storage_file_list_filtering.go
// - Sorting/Grouping functions -> object_storage_file_list_sorting.go
// - Query/Exists/Count -> object_storage_file_list_query.go
// - File collection functions -> object_storage_file_list_collection.go
//
// This file is kept for the shared timeRange type definition and package structure.
