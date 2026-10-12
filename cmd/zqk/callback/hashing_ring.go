package callback

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"sync"
)

// Standard errors for consistent hashing ring operations.
var (
	ErrEmptyRing    = errors.New("consistent hash ring is empty")
	ErrNodeExists   = errors.New("node already exists in consistent hash ring")
	ErrInvalidCount = errors.New("count must be greater than zero")
)

// DefaultVirtualNodeCount defines the default number of virtual nodes per physical node.
const DefaultVirtualNodeCount = 128

// HashRingConfig defines the configuration for a ConsistentHashingRing.
type HashRingConfig struct {
	VnodeCount int
	HashFunc   func(data []byte) uint32
}

// Murmur332 implements the 32-bit MurmurHash3 algorithm for uniform distribution across the hash ring.
func Murmur332(data []byte) uint32 {
	const (
		c1 uint32 = 0xcc9e2d51
		c2 uint32 = 0x1b873593
	)

	h := uint32(0)
	nblocks := len(data) / 4
	for i := 0; i < nblocks; i++ {
		k := uint32(data[i*4]) |
			(uint32(data[i*4+1]) << 8) |
			(uint32(data[i*4+2]) << 16) |
			(uint32(data[i*4+3]) << 24)

		k *= c1
		k = (k << 15) | (k >> 17)
		k *= c2

		h ^= k
		h = (h << 13) | (h >> 19)
		h = h*5 + 0xe6546b64
	}

	tail := data[nblocks*4:]
	k1 := uint32(0)
	switch len(tail) {
	case 3:
		k1 ^= uint32(tail[2]) << 16
		fallthrough
	case 2:
		k1 ^= uint32(tail[1]) << 8
		fallthrough
	case 1:
		k1 ^= uint32(tail[0])
		k1 *= c1
		k1 = (k1 << 15) | (k1 >> 17)
		k1 *= c2
		h ^= k1
	}

	h ^= uint32(len(data))
	h ^= h >> 16
	h *= 0x85ebca6b
	h ^= h >> 13
	h *= 0xc2b2ae35
	h ^= h >> 16
	return h
}

// defaultHashFunc implements Murmur3-32 hash.
func defaultHashFunc(data []byte) uint32 {
	return Murmur332(data)
}

// ConsistentHashingRing manages virtual node distribution across physical nodes on a hash ring.
type ConsistentHashingRing struct {
	mu         sync.RWMutex
	vnodeCount int
	hashFunc   func(data []byte) uint32
	ring       []uint32
	vnodes     map[uint32]string
	members    map[string]bool
}

// NewConsistentHashingRing instantiates a new ring with the supplied configuration.
func NewConsistentHashingRing(cfg HashRingConfig) *ConsistentHashingRing {
	vnodes := cfg.VnodeCount
	if vnodes <= 0 {
		vnodes = DefaultVirtualNodeCount
	}
	hFunc := cfg.HashFunc
	if hFunc == nil {
		hFunc = defaultHashFunc
	}

	return &ConsistentHashingRing{
		vnodeCount: vnodes,
		hashFunc:   hFunc,
		ring:       make([]uint32, 0),
		vnodes:     make(map[uint32]string),
		members:    make(map[string]bool),
	}
}

func validateNodeID(nodeID string) error {
	if nodeID == "" {
		return ErrInvalidNodeID
	}
	return nil
}

func (r *ConsistentHashingRing) withMutationLock(nodeID string, fn func() error) error {
	if err := validateNodeID(nodeID); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return fn()
}

// AddNode registers a physical node and populates its virtual nodes onto the ring.
func (r *ConsistentHashingRing) AddNode(nodeID string) error {
	return r.withMutationLock(nodeID, func() error {
		if r.members[nodeID] {
			return ErrNodeExists
		}

		r.members[nodeID] = true
		newHashes := make([]uint32, 0, r.vnodeCount)
		for i := 0; i < r.vnodeCount; i++ {
			vnodeKey := nodeID + "#vn" + strconv.Itoa(i)
			hash := r.hashFunc([]byte(vnodeKey))
			if _, exists := r.vnodes[hash]; !exists {
				r.vnodes[hash] = nodeID
				newHashes = append(newHashes, hash)
			}
		}

		r.ring = append(r.ring, newHashes...)
		sort.Slice(r.ring, func(i, j int) bool {
			return r.ring[i] < r.ring[j]
		})

		return nil
	})
}

// RemoveNode removes a physical node and all associated virtual nodes from the ring.
func (r *ConsistentHashingRing) RemoveNode(nodeID string) error {
	return r.withMutationLock(nodeID, func() error {
		if !r.members[nodeID] {
			return ErrNodeNotFound
		}

		delete(r.members, nodeID)
		filteredRing := make([]uint32, 0, len(r.ring))
		for _, hash := range r.ring {
			if owner := r.vnodes[hash]; owner == nodeID {
				delete(r.vnodes, hash)
			} else {
				filteredRing = append(filteredRing, hash)
			}
		}
		r.ring = filteredRing

		return nil
	})
}

// GetNode routes a key to its primary node on the ring using clockwise binary search.
func (r *ConsistentHashingRing) GetNode(key string) (string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if len(r.ring) == 0 {
		return "", ErrEmptyRing
	}

	hash := r.hashFunc([]byte(key))
	idx := sort.Search(len(r.ring), func(i int) bool {
		return r.ring[i] >= hash
	})

	if idx == len(r.ring) {
		idx = 0
	}

	targetHash := r.ring[idx]
	node, exists := r.vnodes[targetHash]
	if !exists {
		return "", fmt.Errorf("vnode hash %d missing owner", targetHash)
	}

	return node, nil
}

// GetNodes routes a key to count distinct physical replica nodes in clockwise order.
func (r *ConsistentHashingRing) GetNodes(key string, count int) ([]string, error) {
	if count <= 0 {
		return nil, ErrInvalidCount
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	if len(r.ring) == 0 {
		return nil, ErrEmptyRing
	}

	memberCount := len(r.members)
	if count > memberCount {
		count = memberCount
	}

	hash := r.hashFunc([]byte(key))
	startIdx := sort.Search(len(r.ring), func(i int) bool {
		return r.ring[i] >= hash
	})

	results := make([]string, 0, count)
	seen := make(map[string]bool, count)
	ringLen := len(r.ring)

	for i := 0; i < ringLen && len(results) < count; i++ {
		currIdx := (startIdx + i) % ringLen
		node := r.vnodes[r.ring[currIdx]]
		if !seen[node] {
			seen[node] = true
			results = append(results, node)
		}
	}

	return results, nil
}

// Members returns a slice of all physical member node IDs currently registered.
func (r *ConsistentHashingRing) Members() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	members := make([]string, 0, len(r.members))
	for member := range r.members {
		members = append(members, member)
	}
	sort.Strings(members)
	return members
}

// Size returns the count of registered physical member nodes.
func (r *ConsistentHashingRing) Size() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.members)
}
