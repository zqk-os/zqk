package storage

import (
	"container/list"
	"sync"
	"sync/atomic"

	"github.com/lanceman/zqk/pkg/concurrency"
	"github.com/lanceman/zqk/pkg/objects"
)

const maxParseCacheEntries = 8192

type parseCacheEntry struct {
	hash   string
	parsed *objects.ParsedObject
}

type ParseCache struct {
	mu        sync.RWMutex
	max       int
	ll        *list.List
	entries   map[string]*list.Element
	hits      atomic.Uint64
	misses    atomic.Uint64
	puts      atomic.Uint64
	evictions atomic.Uint64
}

var globalParseCache = NewParseCache(maxParseCacheEntries)

func NewParseCache(max int) *ParseCache {
	if max < 1 {
		max = 1
	}
	return &ParseCache{
		max:     max,
		ll:      list.New(),
		entries: make(map[string]*list.Element),
	}
}

func (c *ParseCache) Get(hash string) (*objects.ParsedObject, bool) {
	if c == nil || hash == "" {
		return nil, false
	}
	var parsed *objects.ParsedObject
	ok := false
	_ = concurrency.RunInRLock(&c.mu, func() error {
		el, hit := c.entries[hash]
		if hit {
			e := el.Value.(*parseCacheEntry)
			parsed = e.parsed
			ok = true
		}
		return nil
	})
	if ok {
		c.hits.Add(1)
	} else {
		c.misses.Add(1)
	}
	return parsed, ok
}

func (c *ParseCache) Put(hash string, parsed *objects.ParsedObject) {
	if c == nil || hash == "" || parsed == nil {
		return
	}
	c.puts.Add(1)
	_ = concurrency.RunInLock(&c.mu, func() error {
		if el, hit := c.entries[hash]; hit {
			e := el.Value.(*parseCacheEntry)
			e.parsed = parsed
			c.ll.MoveToFront(el)
			return nil
		}
		if len(c.entries) >= c.max {
			c.evictLRULocked()
		}
		el := c.ll.PushFront(&parseCacheEntry{hash: hash, parsed: parsed})
		c.entries[hash] = el
		return nil
	})
}

func (c *ParseCache) Evict(hash string) {
	if c == nil || hash == "" {
		return
	}
	_ = concurrency.RunInLock(&c.mu, func() error {
		el, hit := c.entries[hash]
		if hit {
			c.ll.Remove(el)
			delete(c.entries, hash)
			c.evictions.Add(1)
		}
		return nil
	})
}

func (c *ParseCache) Clear() {
	if c == nil {
		return
	}
	_ = concurrency.RunInLock(&c.mu, func() error {
		c.ll.Init()
		c.entries = make(map[string]*list.Element)
		return nil
	})
}

func (c *ParseCache) Hits() uint64 {
	if c == nil {
		return 0
	}
	return c.hits.Load()
}

func (c *ParseCache) Misses() uint64 {
	if c == nil {
		return 0
	}
	return c.misses.Load()
}

func (c *ParseCache) Puts() uint64 {
	if c == nil {
		return 0
	}
	return c.puts.Load()
}

func (c *ParseCache) Evictions() uint64 {
	if c == nil {
		return 0
	}
	return c.evictions.Load()
}

func (c *ParseCache) ResetMetrics() {
	if c == nil {
		return
	}
	c.hits.Store(0)
	c.misses.Store(0)
	c.puts.Store(0)
	c.evictions.Store(0)
}

func (c *ParseCache) evictLRULocked() {
	if c.ll.Len() > 0 {
		el := c.ll.Back()
		e := el.Value.(*parseCacheEntry)
		c.ll.Remove(el)
		delete(c.entries, e.hash)
		c.evictions.Add(1)
	}
}

func GetGlobalParseCache() *ParseCache {
	return globalParseCache
}
