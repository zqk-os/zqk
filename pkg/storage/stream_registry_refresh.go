package storage

import (
	"bufio"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// without Stat-under-write-lock. 2026-08-29 system-check profiles: ~350 Creates blocked
// here. Same-process Append/Delete already merge into the cache; this interval only
// delays noticing another process's registry write.
const streamRegistryRefreshMinInterval = 1 * time.Second

func (c *streamRegistrySnapshot) refresh(projectRoot, kind string) {
	c.refreshMu.Lock()
	defer c.refreshMu.Unlock()
	if !c.lastRefresh.IsZero() && time.Since(c.lastRefresh) < streamRegistryRefreshMinInterval {
		return
	}
	c.refreshFromDisk(projectRoot, kind)
	c.lastRefresh = time.Now()
}

func (c *streamRegistrySnapshot) refreshFromDisk(projectRoot, kind string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	stateDir := filepath.Join(projectRoot, paths.ProjectDataDir, paths.StateDir)

	// Check deleted
	delPath := streamDeletedPath(projectRoot, kind)
	if info, err := fileutil.Stat(delPath); err == nil {
		if info.Size() < c.delSize {
			c.deleted = make(map[string]bool)
			c.delSize = 0
		}
		if info.Size() > c.delSize {
			f, err := fileutil.Open(delPath)
			if err == nil {
				_, _ = f.Seek(c.delSize, 0)
				sc := bufio.NewScanner(f)
				for sc.Scan() {
					id := strings.TrimSpace(sc.Text())
					if id != "" {
						c.deleted[id] = true
					}
				}
				c.delSize = info.Size()
				_ = f.Close()
			}
		}
	} else {
		c.deleted = make(map[string]bool)
		c.delSize = 0
	}

	// Check shards
	for i := 0; i < ShardCount; i++ {
		filename := fmt.Sprintf("%s%s_%02d%s", streamRegistryPrefix, kind, i, streamRegistrySuffix)
		registryPath := filepath.Join(stateDir, filename)
		info, err := fileutil.Stat(registryPath)
		if err != nil {
			c.sizes[i] = 0
			continue
		}
		if info.Size() < c.sizes[i] {
			c.locations = make(map[string]string)
			for j := 0; j < ShardCount; j++ {
				c.sizes[j] = 0
			}
			c.sizes[i] = 0
			i = -1 // Restart the loop from shard 0 since we wiped the map
			continue
		}
		if info.Size() > c.sizes[i] {
			f, err := fileutil.Open(registryPath)
			if err == nil {
				_, _ = f.Seek(c.sizes[i], 0)
				sc := bufio.NewScanner(f)
				for sc.Scan() {
					id, loc := parseStreamRegistryLineFast(sc.Bytes())
					if id != "" && loc != "" {
						c.locations[id] = loc
					}
				}
				c.sizes[i] = info.Size()
				_ = f.Close()
			}
		}
	}
}

func (s *streamRegistrySnapshot) getLoc(id string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.deleted[id] {
		return "", true
	}
	loc, ok := s.locations[id]
	return loc, ok
}

func (s *streamRegistrySnapshot) isDeleted(id string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.deleted[id]
}

func (s *streamRegistrySnapshot) iterateLocs(fn func(id, loc string)) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for id, loc := range s.locations {
		fn(id, loc)
	}
}

func (s *streamRegistrySnapshot) iterateLiveLocs(fn func(id, loc string)) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for id, loc := range s.locations {
		if !s.deleted[id] {
			fn(id, loc)
		}
	}
}

func (s *streamRegistrySnapshot) iterateDeleted(fn func(id string)) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for id := range s.deleted {
		fn(id)
	}
}

func readStreamRegistryJSONLIntoMaps(projectRoot, kind string) (locations map[string]string, deleted map[string]bool) {
	snap := loadStreamRegistrySnapshot(projectRoot, kind)
	locations = make(map[string]string)
	deleted = make(map[string]bool)
	if snap != nil {
		snap.iterateLocs(func(id, loc string) {
			locations[id] = loc
		})
		snap.iterateDeleted(func(id string) {
			deleted[id] = true
		})
	}
	return locations, deleted
}
