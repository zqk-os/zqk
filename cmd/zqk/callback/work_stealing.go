package callback

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
)

const (
	// DefaultVirtualNodesPerNode defines the default virtual node replication factor for consistent hashing.
	DefaultVirtualNodesPerNode = 100

	// DefaultWorkerQueueCapacity sets the initial slice capacity for partition work deques.
	DefaultWorkerQueueCapacity = 1024

	// DefaultWorkStealingPartitions defines the default partition count for topology mapping.
	DefaultWorkStealingPartitions = 64

	// DefaultWorkStealingBackoff is the default pause when all sibling deques are empty.
	DefaultWorkStealingBackoff = 50 * time.Microsecond

	// SubscriberNamePartitionedWorkStealing identifies the partitioned work-stealing subscriber pool.
	SubscriberNamePartitionedWorkStealing = "partitioned_work_stealing"

	// SubscriberNameWorkStealingPool identifies the work stealing subscriber pool.
	SubscriberNameWorkStealingPool = "work_stealing_pool"

	// Constants for payload key extraction.
	keyPartitionKey = "partition_key"
	keyTopic        = "topic"
	keyObjectID     = "object_id"
	keyJobID        = "job_id"
	keyID           = "id"
	keyDefaultRoute = "default"
	keyEmptyRoute   = "empty"
)

var (
	// ErrNoNodesInRing signals that consistent hash ring operations were attempted on an empty topology.
	ErrNoNodesInRing = errfmt.Errorf("consistent hash ring has no nodes")

	// ErrEmptyNodeName indicates that an empty string was passed as a node or partition identifier.
	ErrEmptyNodeName = errfmt.Errorf("node name cannot be empty")

	// ErrNodeNotFound indicates that the requested node does not exist in the topology ring.
	ErrNodeNotFound = errfmt.Errorf("node not found in ring")

	// ErrSubscriberPoolClosed indicates operations attempted on a terminated subscriber pool.
	ErrSubscriberPoolClosed = errfmt.Errorf("partitioned subscriber pool is closed")

	// ErrCannotRemoveLastPartition prevents removal of the sole remaining partition.
	ErrCannotRemoveLastPartition = errfmt.Errorf("cannot remove last remaining partition")
)

var (
	_ CallbackSubscriber = (*PartitionedSubscriberPool)(nil)
	_ Subscriber         = (*PartitionedSubscriberPool)(nil)
	_ CallbackSubscriber = (*WorkStealingPool)(nil)
	_ Subscriber         = (*WorkStealingPool)(nil)
)

// ProcessingOrder defines the queue consumption discipline for local partition workers.
type ProcessingOrder int

const (
	// ProcessingOrderLIFO prioritizes the most recently pushed items for the owning worker (Chase-Lev stack discipline).
	ProcessingOrderLIFO ProcessingOrder = iota

	// ProcessingOrderFIFO prioritizes the oldest pushed items for the owning worker.
	ProcessingOrderFIFO
)

func validateNodeName(node string) error {
	if node == emptyValue {
		return ErrEmptyNodeName
	}
	return nil
}

// ExtractRoutingKey derives a consistent partitioning key from callback entry attributes.
func ExtractRoutingKey(entry *CallbackEntry) string {
	if entry == nil {
		return keyEmptyRoute
	}
	if entry.Payload != nil {
		if partitionKey, ok := entry.Payload[keyPartitionKey].(string); ok && partitionKey != emptyValue {
			return partitionKey
		}
		if topic, ok := entry.Payload[keyTopic].(string); ok && topic != emptyValue {
			return topic
		}
		if objID, ok := entry.Payload[keyObjectID].(string); ok && objID != emptyValue {
			return objID
		}
		if id, ok := entry.Payload[keyID].(string); ok && id != emptyValue {
			return id
		}
		if jobID, ok := entry.Payload[keyJobID].(string); ok && jobID != emptyValue {
			return jobID
		}
	}
	if entry.JobID != emptyValue {
		return entry.JobID
	}
	return keyDefaultRoute
}

// ConsistentHashRing manages a deterministic ring of virtual nodes to map keys to physical nodes.
type ConsistentHashRing struct {
	mu          sync.RWMutex
	vnodesCount int
	ring        []uint64
	vnodeToNode map[uint64]string
	nodes       map[string]struct{}
}

// NewConsistentHashRing constructs a thread-safe ConsistentHashRing with the specified virtual nodes factor.
func NewConsistentHashRing(vnodes int) *ConsistentHashRing {
	if vnodes <= 0 {
		vnodes = DefaultVirtualNodesPerNode
	}
	return &ConsistentHashRing{
		vnodesCount: vnodes,
		ring:        make([]uint64, 0),
		vnodeToNode: make(map[uint64]string),
		nodes:       make(map[string]struct{}),
	}
}

func (c *ConsistentHashRing) lockForNode(node string) error {
	if node == emptyValue {
		return ErrEmptyNodeName
	}
	c.mu.Lock()
	return nil
}

// AddNode adds a physical node and its virtual replicas to the consistent hash ring.
func (c *ConsistentHashRing) AddNode(node string) error {
	if err := c.lockForNode(node); err != nil {
		return err
	}
	defer c.mu.Unlock()

	if _, exists := c.nodes[node]; exists {
		return nil
	}

	for i := 0; i < c.vnodesCount; i++ {
		vnodeKey := node + "#vn" + strconv.Itoa(i)
		h := hashFNV1a64(vnodeKey)
		if _, exists := c.vnodeToNode[h]; !exists {
			c.ring = append(c.ring, h)
		}
		c.vnodeToNode[h] = node
	}

	sort.Slice(c.ring, func(i, j int) bool {
		return c.ring[i] < c.ring[j]
	})
	c.nodes[node] = struct{}{}
	return nil
}

// RemoveNode removes a physical node and all associated virtual nodes from the ring.
func (c *ConsistentHashRing) RemoveNode(node string) error {
	if err := c.lockForNode(node); err != nil {
		return err
	}
	defer c.mu.Unlock()

	if _, exists := c.nodes[node]; !exists {
		return ErrNodeNotFound
	}

	delete(c.nodes, node)
	newRing := make([]uint64, 0, len(c.ring))
	for _, h := range c.ring {
		if c.vnodeToNode[h] == node {
			delete(c.vnodeToNode, h)
		} else {
			newRing = append(newRing, h)
		}
	}
	c.ring = newRing
	return nil
}

// GetNode resolves a routing key to its designated physical node on the ring with zero heap allocations.
func (c *ConsistentHashRing) GetNode(key string) (string, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	n := len(c.ring)
	if n == 0 {
		return emptyValue, ErrNoNodesInRing
	}

	h := hashFNV1a64(key)

	low, high := 0, n
	for low < high {
		mid := int(uint(low+high) >> 1)
		if c.ring[mid] < h {
			low = mid + 1
		} else {
			high = mid
		}
	}
	if low == n {
		low = 0
	}

	targetNode, ok := c.vnodeToNode[c.ring[low]]
	if !ok {
		return emptyValue, ErrNoNodesInRing
	}
	return targetNode, nil
}

// Nodes returns a sorted slice of all active physical nodes in the ring.
func (c *ConsistentHashRing) Nodes() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	res := make([]string, 0, len(c.nodes))
	for n := range c.nodes {
		res = append(res, n)
	}
	sort.Strings(res)
	return res
}

// NodeCount returns the number of active physical nodes in the ring.
func (c *ConsistentHashRing) NodeCount() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.nodes)
}

// HasNode returns whether the specified physical node exists in the ring.
func (c *ConsistentHashRing) HasNode(node string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	_, exists := c.nodes[node]
	return exists
}

// WorkDeque implements a thread-safe double-ended queue supporting Chase-Lev work-stealing semantics.
type WorkDeque struct {
	mu    sync.Mutex
	items []*CallbackEntry
}

// NewWorkDeque instantiates a new empty WorkDeque.
func NewWorkDeque() *WorkDeque {
	return &WorkDeque{
		items: make([]*CallbackEntry, 0, DefaultWorkerQueueCapacity),
	}
}

// PushBottom appends an entry to the bottom (tail) of the worker's deque.
func (d *WorkDeque) PushBottom(entry *CallbackEntry) {
	if entry == nil {
		return
	}
	d.mu.Lock()
	d.items = append(d.items, entry)
	d.mu.Unlock()
}

// PopBottom extracts and returns the entry from the bottom of the deque (LIFO discipline for local worker).
func (d *WorkDeque) PopBottom() (*CallbackEntry, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	n := len(d.items)
	if n == 0 {
		return nil, false
	}
	lastIdx := n - 1
	item := d.items[lastIdx]
	d.items[lastIdx] = nil
	d.items = d.items[:lastIdx]
	return item, true
}

// StealTop steals and returns the entry from the top of the deque (FIFO discipline for remote thieves).
func (d *WorkDeque) StealTop() (*CallbackEntry, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.items) == 0 {
		return nil, false
	}
	item := d.items[0]
	d.items[0] = nil
	d.items = d.items[1:]
	return item, true
}

// Steal is an alias for StealTop to satisfy work-stealing criteria contracts.
func (d *WorkDeque) Steal() (*CallbackEntry, bool) {
	return d.StealTop()
}

// PushHead delegates to PushBottom.
func (d *WorkDeque) PushHead(entry *CallbackEntry) {
	d.PushBottom(entry)
}

// PushTail prepends an entry to the top of the deque.
func (d *WorkDeque) PushTail(entry *CallbackEntry) {
	if entry == nil {
		return
	}
	d.mu.Lock()
	d.items = append([]*CallbackEntry{entry}, d.items...)
	d.mu.Unlock()
}

// PopHead delegates to PopBottom (newest item, LIFO).
func (d *WorkDeque) PopHead() (*CallbackEntry, bool) {
	return d.PopBottom()
}

// PopTail delegates to StealTop (oldest item, FIFO).
func (d *WorkDeque) PopTail() (*CallbackEntry, bool) {
	return d.StealTop()
}

// StealTail delegates to StealTop.
func (d *WorkDeque) StealTail() (*CallbackEntry, bool) {
	return d.StealTop()
}

// StealHead delegates to PopBottom.
func (d *WorkDeque) StealHead() (*CallbackEntry, bool) {
	return d.PopBottom()
}

// Len returns the current count of pending entries in the deque.
func (d *WorkDeque) Len() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.items)
}

// IsEmpty reports whether the deque contains no entries.
func (d *WorkDeque) IsEmpty() bool {
	return d.Len() == 0
}

// Drain clears all items from the deque and returns them as a slice.
func (d *WorkDeque) Drain() []*CallbackEntry {
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.items) == 0 {
		return nil
	}
	drained := make([]*CallbackEntry, len(d.items))
	copy(drained, d.items)
	clear(d.items)
	d.items = d.items[:0]
	return drained
}

// TopologyPartitionMap manages dynamic subscriber partition assignments across cluster nodes/workers.
type TopologyPartitionMap struct {
	mu              sync.RWMutex
	numPartitions   int
	workers         []int
	partAssignments map[int]int
}

// NewTopologyPartitionMap creates an initialized partition map with evenly distributed worker assignments.
func NewTopologyPartitionMap(numPartitions int, initialWorkers ...int) *TopologyPartitionMap {
	if numPartitions <= 0 {
		numPartitions = DefaultWorkStealingPartitions
	}
	workers := initialWorkers
	if len(workers) == 0 {
		workers = []int{0}
	}

	m := &TopologyPartitionMap{
		numPartitions:   numPartitions,
		workers:         make([]int, len(workers)),
		partAssignments: make(map[int]int, numPartitions),
	}
	copy(m.workers, workers)
	sort.Ints(m.workers)
	m.rebalanceLocked()
	return m
}

func (m *TopologyPartitionMap) rebalanceLocked() {
	nw := len(m.workers)
	if nw == 0 {
		return
	}
	for p := 0; p < m.numPartitions; p++ {
		m.partAssignments[p] = m.workers[p%nw]
	}
}

// AddWorker adds a new worker node and redistributes partitions.
func (m *TopologyPartitionMap) AddWorker(workerID int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, w := range m.workers {
		if w == workerID {
			return
		}
	}
	m.workers = append(m.workers, workerID)
	sort.Ints(m.workers)
	m.rebalanceLocked()
}

// RemoveWorker removes a worker node and rebalances its partitions across surviving workers.
func (m *TopologyPartitionMap) RemoveWorker(workerID int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.workers) <= 1 {
		return ErrCannotRemoveLastPartition
	}

	idx := -1
	for i, w := range m.workers {
		if w == workerID {
			idx = i
			break
		}
	}
	if idx == -1 {
		return ErrNodeNotFound
	}

	m.workers = append(m.workers[:idx], m.workers[idx+1:]...)
	m.rebalanceLocked()
	return nil
}

// SetWorkerCount reconfigures the active workers to IDs [0..count-1] and rebalances assignments.
func (m *TopologyPartitionMap) SetWorkerCount(count int) {
	if count <= 0 {
		count = 1
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.workers = make([]int, count)
	for i := 0; i < count; i++ {
		m.workers[i] = i
	}
	m.rebalanceLocked()
}

// WorkerForPartition resolves the designated worker node ID for the given partition ID.
func (m *TopologyPartitionMap) WorkerForPartition(partitionID int) int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if len(m.workers) == 0 {
		return 0
	}
	p := partitionID % m.numPartitions
	if p < 0 {
		p += m.numPartitions
	}
	return m.partAssignments[p]
}

// WorkerForKey maps an arbitrary string routing key to its designated worker node ID.
func (m *TopologyPartitionMap) WorkerForKey(key string) int {
	h := hashFNV1a64(key)
	partitionID := int(h % uint64(m.numPartitions))
	return m.WorkerForPartition(partitionID)
}

// PartitionsForWorker returns all partition IDs currently assigned to the given worker ID.
func (m *TopologyPartitionMap) PartitionsForWorker(workerID int) []int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var assigned []int
	for p := 0; p < m.numPartitions; p++ {
		if m.partAssignments[p] == workerID {
			assigned = append(assigned, p)
		}
	}
	return assigned
}

// AssignPartition manually pins a specific partition to a target worker node ID.
func (m *TopologyPartitionMap) AssignPartition(partitionID int, workerID int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p := partitionID % m.numPartitions
	if p < 0 {
		p += m.numPartitions
	}
	m.partAssignments[p] = workerID
}

// NumPartitions returns the total number of partitions in the topology map.
func (m *TopologyPartitionMap) NumPartitions() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.numPartitions
}

// NumWorkers returns the count of active worker nodes.
func (m *TopologyPartitionMap) NumWorkers() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.workers)
}

// ActiveWorkers returns a sorted slice of active worker node identifiers.
func (m *TopologyPartitionMap) ActiveWorkers() []int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	res := make([]int, len(m.workers))
	copy(res, m.workers)
	return res
}

// WorkStealingConfig defines runtime options for WorkStealingPool.
type WorkStealingConfig struct {
	NumWorkers    int
	NumPartitions int
	Backoff       time.Duration
	Logger        logging.Logger
}

// WorkStealingStats captures operational load balancing telemetry.
type WorkStealingStats struct {
	TotalSubmitted int64
	LocalPops      int64
	Steals         int64
	StealAttempts  int64
	StealFailures  int64
	ActiveWorkers  int
	TotalPending   int
	WorkerDepths   []int
}

// WorkStealingPool maintains N worker deques and coordinates cross-worker work stealing.
type WorkStealingPool struct {
	mu             sync.RWMutex
	topology       *TopologyPartitionMap
	deques         []*WorkDeque
	numWorkers     int
	backoff        time.Duration
	closed         atomic.Bool
	totalSubmitted atomic.Int64
	localPops      atomic.Int64
	steals         atomic.Int64
	stealAttempts  atomic.Int64
	stealFailures  atomic.Int64
}

// NewWorkStealingPool allocates a WorkStealingPool with N worker deques and balanced topology.
func NewWorkStealingPool(numWorkers int) *WorkStealingPool {
	return NewWorkStealingPoolWithConfig(WorkStealingConfig{
		NumWorkers: numWorkers,
	})
}

// NewWorkStealingPoolWithConfig instantiates a WorkStealingPool with customized configuration.
func NewWorkStealingPoolWithConfig(cfg WorkStealingConfig) *WorkStealingPool {
	nw := cfg.NumWorkers
	if nw <= 0 {
		nw = 4
	}
	np := cfg.NumPartitions
	if np <= 0 {
		np = nw * 16
		if np < DefaultWorkStealingPartitions {
			np = DefaultWorkStealingPartitions
		}
	}
	bo := cfg.Backoff
	if bo <= 0 {
		bo = DefaultWorkStealingBackoff
	}

	deques := make([]*WorkDeque, nw)
	workerIDs := make([]int, nw)
	for i := 0; i < nw; i++ {
		deques[i] = NewWorkDeque()
		workerIDs[i] = i
	}

	topology := NewTopologyPartitionMap(np, workerIDs...)

	return &WorkStealingPool{
		topology:   topology,
		deques:     deques,
		numWorkers: nw,
		backoff:    bo,
	}
}

// Submit routes a callback entry to its assigned worker queue via partition hint or consistent hash.
func (p *WorkStealingPool) Submit(entry *CallbackEntry, shardHint int) bool {
	if entry == nil || p.closed.Load() {
		return false
	}

	var workerID int
	if shardHint >= 0 {
		workerID = p.topology.WorkerForPartition(shardHint)
	} else {
		key := ExtractRoutingKey(entry)
		workerID = p.topology.WorkerForKey(key)
	}

	p.mu.RLock()
	nw := p.numWorkers
	if nw == 0 {
		p.mu.RUnlock()
		return false
	}
	if workerID < 0 || workerID >= nw {
		workerID = workerID % nw
	}
	dq := p.deques[workerID]
	p.mu.RUnlock()

	dq.PushBottom(entry)
	p.totalSubmitted.Add(1)
	return true
}

// WorkerPopOrSteal retrieves work for a worker: locally first, stealing from siblings on idle.
func (p *WorkStealingPool) WorkerPopOrSteal(workerID int) (*CallbackEntry, bool) {
	if dq := p.WorkerDeque(workerID); dq != nil {
		if entry, ok := dq.PopBottom(); ok {
			p.localPops.Add(1)
			return entry, true
		}
	}

	if stolen, ok := p.Steal(workerID); ok {
		return stolen, true
	}

	if p.backoff > 0 {
		time.Sleep(p.backoff)
	}
	return nil, false
}

// Steal allows workerID to steal from overloaded sibling worker queues.
func (p *WorkStealingPool) Steal(workerID int) (*CallbackEntry, bool) {
	if p.closed.Load() {
		return nil, false
	}
	p.mu.RLock()
	nw := p.numWorkers
	p.mu.RUnlock()
	if nw <= 1 {
		return nil, false
	}

	p.stealAttempts.Add(1)
	for i := 1; i < nw; i++ {
		victimID := (workerID + i) % nw
		p.mu.RLock()
		var victimDq *WorkDeque
		if victimID < len(p.deques) {
			victimDq = p.deques[victimID]
		}
		p.mu.RUnlock()

		if victimDq != nil {
			if stolen, ok := victimDq.StealTop(); ok {
				p.steals.Add(1)
				return stolen, true
			}
		}
	}
	p.stealFailures.Add(1)
	return nil, false
}

// Rebalance resizes the worker deque pool and reallocates partition topology dynamically.
func (p *WorkStealingPool) Rebalance(numWorkers int) {
	if numWorkers <= 0 {
		numWorkers = 1
	}
	p.mu.Lock()
	if numWorkers == p.numWorkers {
		p.mu.Unlock()
		return
	}

	var orphaned []*CallbackEntry
	if numWorkers > p.numWorkers {
		for i := p.numWorkers; i < numWorkers; i++ {
			p.deques = append(p.deques, NewWorkDeque())
		}
	} else {
		for i := numWorkers; i < p.numWorkers; i++ {
			if i < len(p.deques) {
				orphaned = append(orphaned, p.deques[i].Drain()...)
			}
		}
		p.deques = p.deques[:numWorkers]
	}

	p.numWorkers = numWorkers
	p.topology.SetWorkerCount(numWorkers)
	p.mu.Unlock()

	for _, entry := range orphaned {
		p.Submit(entry, -1)
	}
}

// Drain clears and returns all pending callback entries from all worker deques.
func (p *WorkStealingPool) Drain() []*CallbackEntry {
	p.mu.RLock()
	deques := make([]*WorkDeque, len(p.deques))
	copy(deques, p.deques)
	p.mu.RUnlock()

	var all []*CallbackEntry
	for _, dq := range deques {
		if dq != nil {
			drained := dq.Drain()
			if len(drained) > 0 {
				all = append(all, drained...)
			}
		}
	}
	return all
}

// Stats returns a point-in-time telemetry snapshot of pool metrics.
func (p *WorkStealingPool) Stats() WorkStealingStats {
	p.mu.RLock()
	nw := p.numWorkers
	depths := make([]int, nw)
	totalPending := 0
	for i := 0; i < nw && i < len(p.deques); i++ {
		l := p.deques[i].Len()
		depths[i] = l
		totalPending += l
	}
	p.mu.RUnlock()

	return WorkStealingStats{
		TotalSubmitted: p.totalSubmitted.Load(),
		LocalPops:      p.localPops.Load(),
		Steals:         p.steals.Load(),
		StealAttempts:  p.stealAttempts.Load(),
		StealFailures:  p.stealFailures.Load(),
		ActiveWorkers:  nw,
		TotalPending:   totalPending,
		WorkerDepths:   depths,
	}
}

// Close gracefully closes the work stealing pool.
func (p *WorkStealingPool) Close() error {
	p.closed.Store(true)
	return nil
}

// IsClosed reports whether the pool has been shut down.
func (p *WorkStealingPool) IsClosed() bool {
	return p.closed.Load()
}

// Name returns the canonical identifier for the subscriber interface.
func (p *WorkStealingPool) Name() string {
	return SubscriberNameWorkStealingPool
}

// Notify delivers a callback entry to the work stealing pool as a Subscriber.
func (p *WorkStealingPool) Notify(ctx context.Context, entry *CallbackEntry) error {
	if p.closed.Load() {
		return ErrSubscriberPoolClosed
	}
	if entry == nil {
		return nil
	}
	p.Submit(entry, -1)
	return nil
}

// Topology returns the active TopologyPartitionMap.
func (p *WorkStealingPool) Topology() *TopologyPartitionMap {
	return p.topology
}

// WorkerDeque returns the WorkDeque for the specified worker ID.
func (p *WorkStealingPool) WorkerDeque(workerID int) *WorkDeque {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if workerID >= 0 && workerID < len(p.deques) {
		return p.deques[workerID]
	}
	return nil
}

// WorkStealingBalancer orchestrates consistent hash partitioning and cross-partition work-stealing.
type WorkStealingBalancer struct {
	mu     sync.RWMutex
	ring   *ConsistentHashRing
	deques map[string]*WorkDeque
	nodes  []string
	logger logging.Logger
}

// NewWorkStealingBalancer initializes a balancer with the provided initial nodes and virtual node factor.
func NewWorkStealingBalancer(nodes []string, vnodes int, logger logging.Logger) (*WorkStealingBalancer, error) {
	ring := NewConsistentHashRing(vnodes)
	balancer := &WorkStealingBalancer{
		ring:   ring,
		deques: make(map[string]*WorkDeque),
		nodes:  make([]string, 0, len(nodes)),
		logger: logger,
	}

	for _, n := range nodes {
		if err := balancer.AddNode(n); err != nil {
			return nil, err
		}
	}
	return balancer, nil
}

// Ring exposes the underlying ConsistentHashRing.
func (b *WorkStealingBalancer) Ring() *ConsistentHashRing {
	return b.ring
}

func (b *WorkStealingBalancer) getDeque(node string) (*WorkDeque, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	dq, ok := b.deques[node]
	return dq, ok && dq != nil
}

// AddNode dynamically registers a new partition node and initializes its work deque.
func (b *WorkStealingBalancer) AddNode(node string) error {
	if err := validateNodeName(node); err != nil {
		return err
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	if _, exists := b.deques[node]; exists {
		return nil
	}

	if err := b.ring.AddNode(node); err != nil {
		return err
	}

	b.deques[node] = NewWorkDeque()
	b.nodes = append(b.nodes, node)
	sort.Strings(b.nodes)
	return nil
}

// RemoveNode safely detaches a partition node, draining and rebalancing its pending entries across remaining nodes.
func (b *WorkStealingBalancer) RemoveNode(node string) ([]*CallbackEntry, error) {
	if err := validateNodeName(node); err != nil {
		return nil, err
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	if len(b.nodes) <= 1 {
		return nil, ErrCannotRemoveLastPartition
	}

	deque, exists := b.deques[node]
	if !exists {
		return nil, ErrNodeNotFound
	}

	drained := deque.Drain()

	if err := b.ring.RemoveNode(node); err != nil {
		return nil, err
	}
	delete(b.deques, node)

	newNodes := make([]string, 0, len(b.nodes)-1)
	for _, n := range b.nodes {
		if n != node {
			newNodes = append(newNodes, n)
		}
	}
	b.nodes = newNodes

	for _, entry := range drained {
		key := ExtractRoutingKey(entry)
		targetNode, err := b.ring.GetNode(key)
		if err == nil {
			if targetDq, ok := b.deques[targetNode]; ok && targetDq != nil {
				targetDq.PushHead(entry)
			}
		}
	}

	return drained, nil
}

// HasNode checks if a physical node is actively registered in the balancer.
func (b *WorkStealingBalancer) HasNode(node string) bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	_, exists := b.deques[node]
	return exists
}

// Nodes returns a snapshot of active node names.
func (b *WorkStealingBalancer) Nodes() []string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	res := make([]string, len(b.nodes))
	copy(res, b.nodes)
	return res
}

// Enqueue routes an entry to its partition deque determined by the consistent hash ring.
func (b *WorkStealingBalancer) Enqueue(key string, entry *CallbackEntry) (string, error) {
	if entry == nil {
		return emptyValue, nil
	}
	b.mu.RLock()
	targetNode, err := b.ring.GetNode(key)
	if err != nil {
		b.mu.RUnlock()
		return emptyValue, err
	}
	dq, ok := b.deques[targetNode]
	b.mu.RUnlock()

	if !ok || dq == nil {
		return emptyValue, ErrNodeNotFound
	}
	dq.PushHead(entry)
	return targetNode, nil
}

// PopLocal pops an entry from the node's local deque according to the chosen processing discipline.
func (b *WorkStealingBalancer) PopLocal(node string, order ProcessingOrder) (*CallbackEntry, bool) {
	dq, ok := b.getDeque(node)
	if !ok {
		return nil, false
	}
	if order == ProcessingOrderFIFO {
		return dq.PopTail()
	}
	return dq.PopHead()
}

// Steal allows an idle worker to steal work from other partition deques using Chase-Lev semantics.
func (b *WorkStealingBalancer) Steal(thiefNode string, order ProcessingOrder) (*CallbackEntry, string, bool) {
	b.mu.RLock()
	nodes := make([]string, len(b.nodes))
	copy(nodes, b.nodes)
	b.mu.RUnlock()

	var bestVictim string
	var maxLen int

	for _, n := range nodes {
		if n == thiefNode {
			continue
		}
		dq, ok := b.getDeque(n)
		if !ok {
			continue
		}
		l := dq.Len()
		if l > maxLen {
			maxLen = l
			bestVictim = n
		}
	}

	if bestVictim != emptyValue {
		dq, ok := b.getDeque(bestVictim)
		if ok {
			var entry *CallbackEntry
			var stole bool
			if order == ProcessingOrderFIFO {
				entry, stole = dq.StealHead()
			} else {
				entry, stole = dq.StealTail()
			}
			if stole {
				return entry, bestVictim, true
			}
		}
	}

	for _, n := range nodes {
		if n == thiefNode || n == bestVictim {
			continue
		}
		dq, ok := b.getDeque(n)
		if !ok {
			continue
		}
		var entry *CallbackEntry
		var stole bool
		if order == ProcessingOrderFIFO {
			entry, stole = dq.StealHead()
		} else {
			entry, stole = dq.StealTail()
		}
		if stole {
			return entry, n, true
		}
	}

	return nil, emptyValue, false
}

// DequeLen returns the count of items in the specified node's deque.
func (b *WorkStealingBalancer) DequeLen(node string) int {
	dq, ok := b.getDeque(node)
	if !ok {
		return 0
	}
	return dq.Len()
}

// TotalPending returns the total number of unconsumed entries across all partition deques.
func (b *WorkStealingBalancer) TotalPending() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	var total int
	for _, dq := range b.deques {
		if dq != nil {
			total += dq.Len()
		}
	}
	return total
}

// PartitionedSubscriberConfig defines the construction parameters for PartitionedSubscriberPool.
type PartitionedSubscriberConfig struct {
	Name            string
	Partitions      []string
	VirtualNodes    int
	WorkersPerShard int
	ProcessingOrder ProcessingOrder
	Handler         func(ctx context.Context, entry *CallbackEntry) error
	Subscriber      Subscriber
	Logger          logging.Logger
}

// PartitionedPoolMetrics provides operational telemetry for partitioned work-stealing execution.
type PartitionedPoolMetrics struct {
	EnqueuedTotal    int64
	ProcessedTotal   int64
	StolenTotal      int64
	DroppedTotal     int64
	FailedTotal      int64
	InFlight         int64
	ActivePartitions int
	Partitions       map[string]int
}

// PartitionedSubscriberPool routes incoming callback entries across consistent hash partitions and dispatches to work-stealing workers.
type PartitionedSubscriberPool struct {
	cfg            PartitionedSubscriberConfig
	balancer       *WorkStealingBalancer
	ctx            context.Context
	cancel         context.CancelFunc
	workerWg       sync.WaitGroup
	wakeCh         chan struct{}
	stopCh         chan struct{}
	stopOnce       sync.Once
	closed         atomic.Bool
	inFlight       atomic.Int64
	enqueuedTotal  atomic.Int64
	processedTotal atomic.Int64
	stolenTotal    atomic.Int64
	droppedTotal   atomic.Int64
	failedTotal    atomic.Int64
}

// NewPartitionedSubscriberPool initializes and starts background partition workers.
func NewPartitionedSubscriberPool(cfg PartitionedSubscriberConfig) (*PartitionedSubscriberPool, error) {
	if cfg.Name == emptyValue {
		cfg.Name = SubscriberNamePartitionedWorkStealing
	}
	if len(cfg.Partitions) == 0 {
		cfg.Partitions = []string{"partition-0", "partition-1", "partition-2", "partition-3"}
	}
	if cfg.VirtualNodes <= 0 {
		cfg.VirtualNodes = DefaultVirtualNodesPerNode
	}
	if cfg.WorkersPerShard <= 0 {
		cfg.WorkersPerShard = 1
	}

	balancer, err := NewWorkStealingBalancer(cfg.Partitions, cfg.VirtualNodes, cfg.Logger)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())
	pool := &PartitionedSubscriberPool{
		cfg:      cfg,
		balancer: balancer,
		ctx:      ctx,
		cancel:   cancel,
		wakeCh:   make(chan struct{}, 4096),
		stopCh:   make(chan struct{}),
	}

	for _, node := range cfg.Partitions {
		for w := 0; w < cfg.WorkersPerShard; w++ {
			workerID := w
			primaryNode := node
			pool.workerWg.Add(1)
			workerName := fmt.Sprintf("partition_worker_%s_%d", primaryNode, workerID)
			workerDesc := fmt.Sprintf("work-stealing partition worker for %s shard %d", primaryNode, workerID)
			goroutinelabels.NewGoroutine(workerName, workerDesc).StartSimple(func() {
				defer pool.workerWg.Done()
				pool.runWorker(pool.ctx, primaryNode, workerID)
			})
		}
	}

	return pool, nil
}

// Name returns the canonical identifier of the subscriber pool.
func (p *PartitionedSubscriberPool) Name() string {
	return p.cfg.Name
}

// Balancer returns the underlying WorkStealingBalancer.
func (p *PartitionedSubscriberPool) Balancer() *WorkStealingBalancer {
	return p.balancer
}

// Notify routes the callback entry into the appropriate partition queue via consistent hashing.
func (p *PartitionedSubscriberPool) Notify(ctx context.Context, entry *CallbackEntry) error {
	if p.closed.Load() {
		return ErrSubscriberPoolClosed
	}
	if entry == nil {
		return nil
	}

	p.inFlight.Add(1)
	key := ExtractRoutingKey(entry)
	_, err := p.balancer.Enqueue(key, entry)
	if err != nil {
		p.inFlight.Add(-1)
		p.droppedTotal.Add(1)
		return err
	}

	p.enqueuedTotal.Add(1)
	p.signalWake()
	return nil
}

func (p *PartitionedSubscriberPool) signalWake() {
	select {
	case p.wakeCh <- struct{}{}:
	default:
	}
}

func (p *PartitionedSubscriberPool) runWorker(ctx context.Context, primaryNode string, workerID int) {
	order := p.cfg.ProcessingOrder
	for {
		if p.closed.Load() {
			return
		}

		entry, ok := p.balancer.PopLocal(primaryNode, order)
		if ok {
			p.dispatch(ctx, entry)
			continue
		}

		stolenEntry, _, stolen := p.balancer.Steal(primaryNode, order)
		if stolen {
			p.stolenTotal.Add(1)
			p.signalWake()
			p.dispatch(ctx, stolenEntry)
			continue
		}

		if !p.balancer.HasNode(primaryNode) {
			return
		}

		if p.balancer.TotalPending() > 0 {
			time.Sleep(50 * time.Microsecond)
			continue
		}

		select {
		case <-p.stopCh:
			return
		case <-ctx.Done():
			return
		case <-p.wakeCh:
		case <-time.After(5 * time.Millisecond):
		}
	}
}

func (p *PartitionedSubscriberPool) dispatch(ctx context.Context, entry *CallbackEntry) {
	defer p.inFlight.Add(-1)
	if entry == nil {
		return
	}
	var err error
	if p.cfg.Handler != nil {
		err = p.cfg.Handler(ctx, entry)
	} else if p.cfg.Subscriber != nil {
		err = p.cfg.Subscriber.Notify(ctx, entry)
	}

	if err != nil {
		p.failedTotal.Add(1)
		if p.cfg.Logger != nil {
			p.cfg.Logger.Warn("partition worker failed to process entry", logging.Error(err))
		}
	} else {
		p.processedTotal.Add(1)
	}
}

// AddPartition adds a new partition shard and starts its corresponding background workers.
func (p *PartitionedSubscriberPool) AddPartition(node string) error {
	if p.closed.Load() {
		return ErrSubscriberPoolClosed
	}
	if err := p.balancer.AddNode(node); err != nil {
		return err
	}

	for w := 0; w < p.cfg.WorkersPerShard; w++ {
		workerID := w
		primaryNode := node
		p.workerWg.Add(1)
		workerName := fmt.Sprintf("partition_worker_%s_%d", primaryNode, workerID)
		workerDesc := fmt.Sprintf("work-stealing partition worker for %s shard %d", primaryNode, workerID)
		goroutinelabels.NewGoroutine(workerName, workerDesc).StartSimple(func() {
			defer p.workerWg.Done()
			p.runWorker(p.ctx, primaryNode, workerID)
		})
	}

	p.signalWake()
	return nil
}

// RemovePartition dynamically removes a partition shard, gracefully migrating pending items.
func (p *PartitionedSubscriberPool) RemovePartition(node string) error {
	if p.closed.Load() {
		return ErrSubscriberPoolClosed
	}
	_, err := p.balancer.RemoveNode(node)
	if err != nil {
		return err
	}
	p.signalWake()
	return nil
}

// Partitions returns the list of current partition names.
func (p *PartitionedSubscriberPool) Partitions() []string {
	return p.balancer.Nodes()
}

// TotalPending returns the total number of unconsumed entries across all partition queues.
func (p *PartitionedSubscriberPool) TotalPending() int {
	return p.balancer.TotalPending()
}

// InFlight returns the count of entries currently enqueued or being dispatched.
func (p *PartitionedSubscriberPool) InFlight() int64 {
	return p.inFlight.Load()
}

// Drain blocks until all pending partition entries and in-flight dispatches are processed or until the context expires.
func (p *PartitionedSubscriberPool) Drain(ctx context.Context) error {
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		if p.inFlight.Load() == 0 && p.TotalPending() == 0 {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// Close gracefully terminates all background workers and cleans up resources.
func (p *PartitionedSubscriberPool) Close() error {
	p.stopOnce.Do(func() {
		p.closed.Store(true)
		if p.cancel != nil {
			p.cancel()
		}
		close(p.stopCh)
	})
	p.workerWg.Wait()
	return nil
}

// IsClosed reports whether the subscriber pool has terminated.
func (p *PartitionedSubscriberPool) IsClosed() bool {
	return p.closed.Load()
}

// Metrics captures operational telemetry for the subscriber pool.
func (p *PartitionedSubscriberPool) Metrics() PartitionedPoolMetrics {
	nodes := p.balancer.Nodes()
	counts := make(map[string]int, len(nodes))
	for _, n := range nodes {
		counts[n] = p.balancer.DequeLen(n)
	}

	return PartitionedPoolMetrics{
		EnqueuedTotal:    p.enqueuedTotal.Load(),
		ProcessedTotal:   p.processedTotal.Load(),
		StolenTotal:      p.stolenTotal.Load(),
		DroppedTotal:     p.droppedTotal.Load(),
		FailedTotal:      p.failedTotal.Load(),
		InFlight:         p.inFlight.Load(),
		ActivePartitions: len(nodes),
		Partitions:       counts,
	}
}
