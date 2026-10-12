package callback

import (
	"errors"
	"fmt"
	"sort"
	"time"
)

// ErrNilRing indicates that a provided consistent hash ring pointer is nil.
var ErrNilRing = errors.New("consistent hash ring is nil")

// PartitionMigration represents the reassignment of a virtual node partition from a source node to a target node.
type PartitionMigration struct {
	VnodeHash  uint32
	SourceNode string
	TargetNode string
	Timestamp  time.Time
}

// PartitionRebalancer computes partition migrations across ring topology shifts and plans replica handoffs.
type PartitionRebalancer struct{}

// NewPartitionRebalancer instantiates a new PartitionRebalancer.
func NewPartitionRebalancer() *PartitionRebalancer {
	return &PartitionRebalancer{}
}

type ringSnapshot struct {
	ring    []uint32
	vnodes  map[uint32]string
	members map[string]bool
}

func (pr *PartitionRebalancer) snapshotRing(ring *ConsistentHashingRing) (ringSnapshot, error) {
	ring.mu.RLock()
	defer ring.mu.RUnlock()

	ringCopy := make([]uint32, len(ring.ring))
	copy(ringCopy, ring.ring)

	vnodesCopy := make(map[uint32]string, len(ring.vnodes))
	for hash, node := range ring.vnodes {
		vnodesCopy[hash] = node
	}

	membersCopy := make(map[string]bool, len(ring.members))
	for node, active := range ring.members {
		membersCopy[node] = active
	}

	return ringSnapshot{
		ring:    ringCopy,
		vnodes:  vnodesCopy,
		members: membersCopy,
	}, nil
}

func (pr *PartitionRebalancer) ringsEquivalent(oldS, newS ringSnapshot) bool {
	if len(oldS.vnodes) != len(newS.vnodes) {
		return false
	}
	for hash, oldOwner := range oldS.vnodes {
		newOwner, exists := newS.vnodes[hash]
		if !exists || newOwner != oldOwner {
			return false
		}
	}
	return true
}

func resolveHashOwner(ring []uint32, vnodes map[uint32]string, hash uint32) string {
	if len(ring) == 0 {
		return ""
	}
	idx := sort.Search(len(ring), func(i int) bool {
		return ring[i] >= hash
	})
	if idx == len(ring) {
		idx = 0
	}
	return vnodes[ring[idx]]
}

func hasVnode(vnodes map[uint32]string, hash uint32) bool {
	owner, exists := vnodes[hash]
	return exists && len(owner) > 0
}

func (pr *PartitionRebalancer) collectAddedVnodeMigrations(oldS, newS ringSnapshot, now time.Time) []PartitionMigration {
	migrations := make([]PartitionMigration, 0)
	for hash, targetNode := range newS.vnodes {
		if !hasVnode(oldS.vnodes, hash) {
			sourceNode := resolveHashOwner(oldS.ring, oldS.vnodes, hash)
			if sourceNode != "" && sourceNode != targetNode {
				migrations = append(migrations, PartitionMigration{
					VnodeHash:  hash,
					SourceNode: sourceNode,
					TargetNode: targetNode,
					Timestamp:  now,
				})
			}
		}
	}
	return migrations
}

func (pr *PartitionRebalancer) collectRemovedVnodeMigrations(oldS, newS ringSnapshot, now time.Time) []PartitionMigration {
	migrations := make([]PartitionMigration, 0)
	for hash, sourceNode := range oldS.vnodes {
		if !hasVnode(newS.vnodes, hash) {
			targetNode := resolveHashOwner(newS.ring, newS.vnodes, hash)
			if targetNode != "" && sourceNode != targetNode {
				migrations = append(migrations, PartitionMigration{
					VnodeHash:  hash,
					SourceNode: sourceNode,
					TargetNode: targetNode,
					Timestamp:  now,
				})
			}
		}
	}
	return migrations
}

func (pr *PartitionRebalancer) collectOwnerChangeMigrations(oldS, newS ringSnapshot, now time.Time) []PartitionMigration {
	migrations := make([]PartitionMigration, 0)
	for hash, oldOwner := range oldS.vnodes {
		if newOwner, exists := newS.vnodes[hash]; exists && oldOwner != newOwner {
			migrations = append(migrations, PartitionMigration{
				VnodeHash:  hash,
				SourceNode: oldOwner,
				TargetNode: newOwner,
				Timestamp:  now,
			})
		}
	}
	return migrations
}

// CalculateMigrations compares vnode distributions between old and new rings and returns the minimal delta of vnodes that changed ownership.
func (pr *PartitionRebalancer) CalculateMigrations(oldRing, newRing *ConsistentHashingRing) ([]PartitionMigration, error) {
	if oldRing == nil || newRing == nil {
		return nil, ErrNilRing
	}

	oldSnapshot, err := pr.snapshotRing(oldRing)
	if err != nil {
		return nil, err
	}

	newSnapshot, err := pr.snapshotRing(newRing)
	if err != nil {
		return nil, err
	}

	if len(oldSnapshot.ring) == 0 || len(newSnapshot.ring) == 0 {
		return nil, ErrEmptyRing
	}

	if pr.ringsEquivalent(oldSnapshot, newSnapshot) {
		return make([]PartitionMigration, 0), nil
	}

	now := time.Now().UTC()
	migrations := make([]PartitionMigration, 0)
	migrations = append(migrations, pr.collectAddedVnodeMigrations(oldSnapshot, newSnapshot, now)...)
	migrations = append(migrations, pr.collectRemovedVnodeMigrations(oldSnapshot, newSnapshot, now)...)
	migrations = append(migrations, pr.collectOwnerChangeMigrations(oldSnapshot, newSnapshot, now)...)

	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].VnodeHash < migrations[j].VnodeHash
	})

	return migrations, nil
}

func (pr *PartitionRebalancer) findHealthyReplicas(ring *ConsistentHashingRing, startIdx int, failedNode string) []string {
	memberCount := len(ring.members)
	maxReplicas := memberCount - 1
	if maxReplicas <= 0 {
		return make([]string, 0)
	}

	replicas := make([]string, 0, maxReplicas)
	seen := make(map[string]bool, memberCount)
	seen[failedNode] = true
	ringLen := len(ring.ring)

	for i := 0; i < ringLen && len(replicas) < maxReplicas; i++ {
		currIdx := (startIdx + i) % ringLen
		node := ring.vnodes[ring.ring[currIdx]]
		if !seen[node] {
			seen[node] = true
			replicas = append(replicas, node)
		}
	}

	return replicas
}

// PlanHandoff maps keys from a failed node to healthy replicas.
func (pr *PartitionRebalancer) PlanHandoff(failedNode string, ring *ConsistentHashingRing) (map[string][]string, error) {
	if ring == nil {
		return nil, ErrNilRing
	}
	if failedNode == "" {
		return nil, ErrNodeNotFound
	}

	ring.mu.RLock()
	defer ring.mu.RUnlock()

	if len(ring.ring) == 0 {
		return nil, ErrEmptyRing
	}
	if !ring.members[failedNode] {
		return nil, ErrNodeNotFound
	}

	handoffPlan := make(map[string][]string)
	for i := 0; i < ring.vnodeCount; i++ {
		vnodeKey := fmt.Sprintf("%s#vn%d", failedNode, i)
		hash := ring.hashFunc([]byte(vnodeKey))
		owner, exists := ring.vnodes[hash]
		if !exists || owner != failedNode {
			continue
		}

		startIdx := sort.Search(len(ring.ring), func(j int) bool {
			return ring.ring[j] >= hash
		})
		handoffPlan[vnodeKey] = pr.findHealthyReplicas(ring, startIdx, failedNode)
	}

	return handoffPlan, nil
}
