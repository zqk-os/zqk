package mcp

import (
	"github.com/zqk-os/zqk/pkg/storage"
)

func init() {
	// Register the graph connection manager with storage so NewStorageFactory can use the graph
	// backend when enabled, without pkg/storage importing pkg/mcp (which would create an import cycle).
	storage.SetGraphConnectionProvider(GetGraphConnectionManager())
}
