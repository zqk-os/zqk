package storage

import (
	"fmt"
	"path/filepath"

	"github.com/lanceman/zqk/pkg/paths"
)

// ShardCount is the number of registry shards.
const ShardCount = 16

// GetRegistryPathForID returns the registry path for a given ID based on sharding.
func GetRegistryPathForID(projectRoot, kind, id string) string {
	shard := 0
	for i := 0; i < len(id); i++ {
		shard += int(id[i])
	}
	shard %= ShardCount

	filename := fmt.Sprintf("%s%s_%02d%s", streamRegistryPrefix, kind, shard, streamRegistrySuffix)
	return filepath.Join(projectRoot, paths.ProjectDataDir, paths.StateDir, filename)
}
