package storage

import (
	"container/list"
	"sync"

	"github.com/lanceman/zqk/pkg/concurrency"
	"github.com/lanceman/zqk/pkg/objects"
)

const maxParseCacheEntries = 8192

type parseCacheEntry struct {
	hash   string
	parsed *objects.ParsedObject
}

type ParseCache struct {
	mu      sync.RWMutex
	max     int
	ll      *list.List
	entries map[string]*list.Element
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
	return parsed, ok
}

func (c *ParseCache) Put(hash string, parsed *objects.ParsedObject) {
	if c == nil || hash == "" || parsed == nil {
		return
	}
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
		}
		return nil
	})
}

func (c *ParseCache) evictLRULocked() {
	if c.ll.Len() > 0 {
		el := c.ll.Back()
		e := el.Value.(*parseCacheEntry)
		c.ll.Remove(el)
		delete(c.entries, e.hash)
	}
}

func GetGlobalParseCache() *ParseCache {
	return globalParseCache
}
