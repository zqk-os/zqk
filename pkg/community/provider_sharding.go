package community

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"sort"
	"sync"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

var (
	ErrNoShardsAvailable = errors.New("no provider shards available in ring")
	ErrShardAlreadyAdded = errors.New("shard already registered in ring")
	ErrShardNotFound     = errors.New("shard not found in ring")
)

// ConsistentShardRing provides consistent hashing across provider shards.
type ConsistentShardRing struct {
	mu            sync.RWMutex
	vnodesPerNode int
	ring          []uint64
	vnodeToShard  map[uint64]string
	shards        map[string]int // shardID -> weight
}

// NewConsistentShardRing initializes an empty consistent shard ring.
func NewConsistentShardRing(vnodesPerNode int) *ConsistentShardRing {
	if vnodesPerNode <= 0 {
		vnodesPerNode = 50
	}
	return &ConsistentShardRing{
		vnodesPerNode: vnodesPerNode,
		vnodeToShard:  make(map[uint64]string),
		shards:        make(map[string]int),
	}
}

// AddShard adds a shard instance to the ring with a specified relative weight multiplier.
func (r *ConsistentShardRing) AddShard(shardID string, weight int) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.shards[shardID]; exists {
		return ErrShardAlreadyAdded
	}
	if weight <= 0 {
		weight = 1
	}
	r.shards[shardID] = weight

	totalVnodes := r.vnodesPerNode * weight
	for i := 0; i < totalVnodes; i++ {
		vnodeKey := fmt.Sprintf("%s#vnode%d", shardID, i)
		h := hashString(vnodeKey)
		r.ring = append(r.ring, h)
		r.vnodeToShard[h] = shardID
	}

	sort.Slice(r.ring, func(i, j int) bool {
		return r.ring[i] < r.ring[j]
	})
	return nil
}

// RemoveShard removes a shard instance from the ring and cleans up its virtual nodes.
func (r *ConsistentShardRing) RemoveShard(shardID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.shards[shardID]; !exists {
		return ErrShardNotFound
	}
	delete(r.shards, shardID)

	var newRing []uint64
	for _, h := range r.ring {
		if r.vnodeToShard[h] == shardID {
			delete(r.vnodeToShard, h)
		} else {
			newRing = append(newRing, h)
		}
	}
	r.ring = newRing
	return nil
}

// GetShard maps an arbitrary partition key to the nearest shard on the hash ring.
func (r *ConsistentShardRing) GetShard(key string) (string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if len(r.ring) == 0 {
		return "", ErrNoShardsAvailable
	}

	h := hashString(key)
	idx := sort.Search(len(r.ring), func(i int) bool {
		return r.ring[i] >= h
	})

	if idx == len(r.ring) {
		idx = 0
	}

	return r.vnodeToShard[r.ring[idx]], nil
}

// ShardCount returns the number of active physical shards.
func (r *ConsistentShardRing) ShardCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.shards)
}

func hashString(s string) uint64 {
	hasher := sha256.New()
	hasher.Write([]byte(s))
	sum := hasher.Sum(nil)
	return binary.BigEndian.Uint64(sum[:8])
}

// SafeRunGoroutine executes an asynchronous worker under POL-CONCURRENCY guidelines.
func (r *ConsistentShardRing) SafeRunGoroutine(ctx context.Context, label string, fn func()) {
	goroutinelabels.NewGoroutine("community.sharding."+label, "async sharding worker").StartSimple(fn)
}
