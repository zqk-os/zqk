package filecas

import (
	"container/list"
	"encoding/hex"
	"sync"

	"github.com/zqk-os/zqk/pkg/concurrency"
)

// Immutable CAS blob cache keyed by content hash. Hash-named YAML never changes
// for a given hash, so list/MCP can skip re-read/re-parse.
const maxCASBlobCacheEntries = 4096

// BlobCache is a bounded, thread-safe byte KV with O(1) LRU eviction.
// entries maps key → list element; the list is MRU at Front, LRU at Back.
// Peek does not move the element and takes only a read lock.
// A single oldestKey pointer is not enough: Get/Put-overwrite move recency, so
// the next victim would still require a scan. The list Back() is that victim.
type BlobCache struct {
	mu      sync.RWMutex
	max     int
	ll      *list.List
	entries map[string]*list.Element
}

type blobEntry struct {
	key  string
	data []byte
}

var globalCASBlobCache = NewBlobCache(maxCASBlobCacheEntries)

// NewBlobCache returns an empty cache that holds at most max entries (min 1).
func NewBlobCache(max int) *BlobCache {
	if max < 1 {
		max = 1
	}
	return &BlobCache{
		max:     max,
		ll:      list.New(),
		entries: make(map[string]*list.Element),
	}
}

func isCASContentHash(s string) bool {
	if len(s) != 64 {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}

func copyBlob(src []byte) []byte {
	if len(src) == 0 {
		return nil
	}
	out := make([]byte, len(src))
	copy(out, src)
	return out
}

func entryOf(el *list.Element) *blobEntry {
	if el == nil {
		return nil
	}
	e, _ := el.Value.(*blobEntry)
	return e
}

// LookupCASBlob returns a copy of a cached immutable CAS blob (Peek: no LRU bump).
func LookupCASBlob(hash string) ([]byte, bool) {
	if !isCASContentHash(hash) {
		return nil, false
	}
	return globalCASBlobCache.Peek(hash)
}

// StoreCASBlob records an immutable CAS blob for later list/MCP hits.
func StoreCASBlob(hash string, data []byte) {
	if !isCASContentHash(hash) || len(data) == 0 {
		return
	}
	globalCASBlobCache.Put(hash, data)
}

// EvictCASBlob removes a hash from the global CAS blob cache.
func EvictCASBlob(hash string) bool {
	if !isCASContentHash(hash) {
		return false
	}
	return globalCASBlobCache.Evict(hash)
}

// ClearCASBlobCache clears all entries from the global CAS blob cache.
func ClearCASBlobCache() {
	globalCASBlobCache.Clear()
}

func lookupCASBlob(hash string) ([]byte, bool) { return LookupCASBlob(hash) }

func storeCASBlob(hash string, data []byte) { StoreCASBlob(hash, data) }

func (c *BlobCache) accessEntry(key string, fn func() ([]byte, bool)) ([]byte, bool) {
	if c == nil || key == "" {
		return nil, false
	}
	src, ok := fn()
	if !ok {
		return nil, false
	}
	return copyBlob(src), true
}

// Peek copies the value for key without changing eviction order.
func (c *BlobCache) Peek(key string) ([]byte, bool) {
	return c.accessEntry(key, func() ([]byte, bool) {
		var src []byte
		ok := false
		_ = concurrency.RunInRLock(&c.mu, func() error {
			el, hit := c.entries[key]
			if e := entryOf(el); hit && e != nil {
				src = e.data
				ok = true
			}
			return nil
		})
		return src, ok
	})
}

// Get copies the value and marks the key most-recently used.
func (c *BlobCache) Get(key string) ([]byte, bool) {
	return c.accessEntry(key, func() ([]byte, bool) {
		var src []byte
		ok := false
		_ = concurrency.RunInLock(&c.mu, func() error {
			el, hit := c.entries[key]
			e := entryOf(el)
			if !hit || e == nil {
				return nil
			}
			c.ll.MoveToFront(el)
			src = e.data
			ok = true
			return nil
		})
		return src, ok
	})
}

// Put stores a copy of data. A new key at capacity evicts the LRU entry first.
// An existing key is overwritten and becomes most-recently used.
func (c *BlobCache) Put(key string, data []byte) {
	if c == nil || key == "" || len(data) == 0 {
		return
	}
	copied := copyBlob(data)
	_ = concurrency.RunInLock(&c.mu, func() error {
		if c.ll == nil {
			c.ll = list.New()
		}
		if c.entries == nil {
			c.entries = make(map[string]*list.Element)
		}
		if el, exists := c.entries[key]; exists {
			if e := entryOf(el); e != nil {
				e.data = copied
			}
			c.ll.MoveToFront(el)
			return nil
		}
		if len(c.entries) >= c.max {
			c.evictLRULocked()
		}
		el := c.ll.PushFront(&blobEntry{key: key, data: copied})
		c.entries[key] = el
		return nil
	})
}

// Evict removes key. Reports whether it was present.
func (c *BlobCache) Evict(key string) bool {
	if c == nil || key == "" {
		return false
	}
	removed := false
	_ = concurrency.RunInLock(&c.mu, func() error {
		el, ok := c.entries[key]
		if !ok {
			return nil
		}
		c.removeElementLocked(el)
		removed = true
		return nil
	})
	return removed
}

// Clear drops every entry. Capacity is unchanged.
func (c *BlobCache) Clear() {
	if c == nil {
		return
	}
	_ = concurrency.RunInLock(&c.mu, func() error {
		c.ll = list.New()
		c.entries = make(map[string]*list.Element)
		return nil
	})
}

// Len is the current entry count.
func (c *BlobCache) Len() int {
	if c == nil {
		return 0
	}
	n := 0
	_ = concurrency.RunInRLock(&c.mu, func() error {
		n = len(c.entries)
		return nil
	})
	return n
}

func (c *BlobCache) evictLRULocked() {
	c.removeElementLocked(c.ll.Back())
}

func (c *BlobCache) removeElementLocked(el *list.Element) {
	e := entryOf(el)
	if e == nil {
		return
	}
	c.ll.Remove(el)
	delete(c.entries, e.key)
}
