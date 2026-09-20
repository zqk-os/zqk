package testdiscovery

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"path/filepath"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

type CacheEntry struct {
	ModTimeUnix int64              `json:"mod_time_unix"`
	Hash        string             `json:"hash"`
	Targets     []DiscoveredTarget `json:"targets"`
}

type DiscoveryCache struct {
	mu      sync.RWMutex
	path    string
	entries map[string]CacheEntry
	dirty   bool
}

func LoadCache(path string) *DiscoveryCache {
	c := &DiscoveryCache{
		path:    path,
		entries: make(map[string]CacheEntry),
	}
	if path == "" {
		return c
	}

	data, err := fileutil.ReadFile(filepath.Clean(path)) // #nosec G304
	if err == nil {
		_ = json.Unmarshal(data, &c.entries)
	}
	return c
}

func (c *DiscoveryCache) Get(relPath string, modTime time.Time, content []byte) ([]DiscoveredTarget, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.entries[relPath]
	if !ok {
		return nil, false
	}

	// Check mtime fast path
	if entry.ModTimeUnix == modTime.Unix() {
		return entry.Targets, true
	}

	// Fallback to content hash check
	if len(content) > 0 {
		h := sha256.Sum256(content)
		hStr := hex.EncodeToString(h[:])
		if hStr == entry.Hash {
			return entry.Targets, true
		}
	}

	return nil, false
}

func (c *DiscoveryCache) Set(relPath string, modTime time.Time, content []byte, targets []DiscoveredTarget) {
	c.mu.Lock()
	defer c.mu.Unlock()

	h := sha256.Sum256(content)
	c.entries[relPath] = CacheEntry{
		ModTimeUnix: modTime.Unix(),
		Hash:        hex.EncodeToString(h[:]),
		Targets:     targets,
	}
	c.dirty = true
}

func (c *DiscoveryCache) Save() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.dirty || c.path == "" {
		return nil
	}

	if err := fileutil.MkdirAll(filepath.Dir(c.path), 0750); err != nil {
		return err
	}

	data, err := json.MarshalIndent(c.entries, "", "  ")
	if err != nil {
		return err
	}

	return fileutil.WriteSecureFile(c.path, data)
}
