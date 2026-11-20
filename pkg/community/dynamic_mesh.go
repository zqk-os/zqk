// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package community

import (
	"container/heap"
	"errors"
	"math"
	"sort"
	"sync"

	"github.com/zqk-os/zqk/pkg/strutil"
)

const (
	errMeshBlankNodeIDMsg     = "node ID cannot be blank"
	defaultLatencyWeight      = 1.0
	defaultBandwidthWeight    = 100.0
	defaultEpistemicWeight    = 10.0
	defaultBlackholeLatencyMs = 0.0
)

var (
	// ErrInvalidNodeID indicates a blank or invalid node identifier.
	ErrInvalidNodeID = errors.New(errMeshBlankNodeIDMsg)

	// ErrInvalidMaxNeighbors indicates a non-positive neighbor limit.
	ErrInvalidMaxNeighbors = errors.New("max neighbors must be positive")

	// ErrInvalidLinkWeight indicates non-finite or out-of-bounds link metrics.
	ErrInvalidLinkWeight = errors.New("link latency and bandwidth must be finite and positive, and epistemic proximity must be finite and non-negative")

	// ErrMeshNodeNotFound indicates the referenced node is not registered in the mesh.
	ErrMeshNodeNotFound = errors.New("node not found in mesh topology")

	// ErrNodeAlreadyExists indicates a duplicate node registration attempt.
	ErrNodeAlreadyExists = errors.New("node already registered in mesh topology")

	// ErrMaxNeighborsExceeded indicates a node has reached its configured neighbor limit.
	ErrMaxNeighborsExceeded = errors.New("node neighbor limit exceeded")

	// ErrSelfRoute indicates source and destination nodes are identical.
	ErrSelfRoute = errors.New("source and destination nodes must be distinct")

	// ErrDestinationUnreachable indicates no valid path exists to the destination node.
	ErrDestinationUnreachable = errors.New("destination node is unreachable")

	// ErrPartitionBlackhole indicates routing was blocked by a partition blackhole.
	ErrPartitionBlackhole = errors.New("route blocked by partition blackhole")
)

// MeshConfig configures composite routing weights and neighbor limits.
type MeshConfig struct {
	MaxNeighbors                int
	LatencyWeight               float64
	BandwidthWeight             float64
	EpistemicWeight             float64
	BlackholeLatencyThresholdMs float64
}

// MeshNode represents a peer vertex in the dynamic P2P swarm mesh.
type MeshNode struct {
	ID              string
	MaxNeighbors    int
	EpistemicDomain string
}

// LinkMetrics describes directed P2P link performance and epistemic proximity.
type LinkMetrics struct {
	LatencyMs          float64
	BandwidthMbps      float64
	EpistemicProximity float64
	Blackholed         bool
}

// MeshRoute represents a computed multi-hop P2P path and its aggregate metrics.
type MeshRoute struct {
	Source           string
	Destination      string
	Hops             []string
	TotalCost        float64
	TotalLatencyMs   float64
	MinBandwidthMbps float64
	FallbackUsed     bool
}

// MeshTelemetry tracks routing computations, failures, and fallback reroutes.
type MeshTelemetry struct {
	RouteComputations  uint64
	RouteFailures      uint64
	FallbackReroutes   uint64
	BlackholesDetected uint64
}

// TopologySnapshot provides an immutable point-in-time view of the mesh graph.
type TopologySnapshot struct {
	NodeCount      int
	LinkCount      int
	Nodes          []string
	DistanceMatrix map[string]map[string]float64
	Telemetry      MeshTelemetry
}

// DynamicMesh maintains a thread-safe P2P topology graph and adaptive epistemic router.
type DynamicMesh struct {
	mu             sync.RWMutex
	cfg            MeshConfig
	nodes          map[string]MeshNode
	adj            map[string]map[string]LinkMetrics
	distanceMatrix map[string]map[string]float64
	telemetry      MeshTelemetry
}

// NewDynamicMesh constructs a validated DynamicMesh instance.
func NewDynamicMesh(cfg MeshConfig) (*DynamicMesh, error) {
	if cfg.MaxNeighbors <= 0 {
		return nil, ErrInvalidMaxNeighbors
	}
	if err := validateConfigWeights(&cfg); err != nil {
		return nil, err
	}
	return &DynamicMesh{
		cfg:            cfg,
		nodes:          make(map[string]MeshNode),
		adj:            make(map[string]map[string]LinkMetrics),
		distanceMatrix: make(map[string]map[string]float64),
	}, nil
}

func validateConfigWeights(cfg *MeshConfig) error {
	weights := []float64{cfg.LatencyWeight, cfg.BandwidthWeight, cfg.EpistemicWeight, cfg.BlackholeLatencyThresholdMs}
	for _, w := range weights {
		if math.IsNaN(w) || math.IsInf(w, 0) || w < 0 {
			return ErrInvalidLinkWeight
		}
	}
	if cfg.LatencyWeight == 0 && cfg.BandwidthWeight == 0 && cfg.EpistemicWeight == 0 {
		cfg.LatencyWeight = defaultLatencyWeight
		cfg.BandwidthWeight = defaultBandwidthWeight
		cfg.EpistemicWeight = defaultEpistemicWeight
	}
	return nil
}

func validateLinkMetrics(m LinkMetrics) error {
	if math.IsNaN(m.LatencyMs) || math.IsInf(m.LatencyMs, 0) || m.LatencyMs <= 0 {
		return ErrInvalidLinkWeight
	}
	if math.IsNaN(m.BandwidthMbps) || math.IsInf(m.BandwidthMbps, 0) || m.BandwidthMbps <= 0 {
		return ErrInvalidLinkWeight
	}
	if math.IsNaN(m.EpistemicProximity) || math.IsInf(m.EpistemicProximity, 0) || m.EpistemicProximity < 0 {
		return ErrInvalidLinkWeight
	}
	return nil
}

// RegisterNode adds a validated peer node to the mesh topology.
func (m *DynamicMesh) RegisterNode(node MeshNode) error {
	if err := strutil.ErrorOnBlank(node.ID, errMeshBlankNodeIDMsg); err != nil {
		return ErrInvalidNodeID
	}
	if node.MaxNeighbors <= 0 {
		return ErrInvalidMaxNeighbors
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.nodes[node.ID]; exists {
		return ErrNodeAlreadyExists
	}
	m.nodes[node.ID] = node
	m.adj[node.ID] = make(map[string]LinkMetrics)
	m.recomputeMatrixLocked()
	return nil
}

// RemoveNode deletes a node and all incident links from the mesh topology.
func (m *DynamicMesh) RemoveNode(nodeID string) error {
	if err := strutil.ErrorOnBlank(nodeID, errMeshBlankNodeIDMsg); err != nil {
		return ErrInvalidNodeID
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.nodes[nodeID]; !exists {
		return ErrMeshNodeNotFound
	}
	delete(m.nodes, nodeID)
	delete(m.adj, nodeID)
	for peer := range m.adj {
		delete(m.adj[peer], nodeID)
	}
	m.recomputeMatrixLocked()
	return nil
}

// UpsertLink creates or updates a directed link between two registered nodes.
func (m *DynamicMesh) UpsertLink(src, dst string, metrics LinkMetrics) error {
	if err := validateEndpoints(src, dst); err != nil {
		return err
	}
	if err := validateLinkMetrics(metrics); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	srcNode, srcOK := m.nodes[src]
	if !srcOK {
		return ErrMeshNodeNotFound
	}
	if _, dstOK := m.nodes[dst]; !dstOK {
		return ErrMeshNodeNotFound
	}

	_, updating := m.adj[src][dst]
	if !updating && len(m.adj[src]) >= srcNode.MaxNeighbors {
		return ErrMaxNeighborsExceeded
	}
	m.adj[src][dst] = metrics
	m.recomputeMatrixLocked()
	return nil
}

// RemoveLink removes a directed link between two nodes.
func (m *DynamicMesh) RemoveLink(src, dst string) error {
	if err := validateEndpoints(src, dst); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.nodes[src]; !ok {
		return ErrMeshNodeNotFound
	}
	if _, ok := m.nodes[dst]; !ok {
		return ErrMeshNodeNotFound
	}
	delete(m.adj[src], dst)
	m.recomputeMatrixLocked()
	return nil
}

// MarkLinkBlackholed toggles partition blackhole state on an existing link.
func (m *DynamicMesh) MarkLinkBlackholed(src, dst string, blackholed bool) error {
	if err := validateEndpoints(src, dst); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	metrics, ok := m.adj[src][dst]
	if !ok {
		return ErrMeshNodeNotFound
	}
	metrics.Blackholed = blackholed
	m.adj[src][dst] = metrics
	m.recomputeMatrixLocked()
	return nil
}

func validateEndpoints(src, dst string) error {
	if err := strutil.ErrorOnBlank(src, errMeshBlankNodeIDMsg); err != nil {
		return ErrInvalidNodeID
	}
	if err := strutil.ErrorOnBlank(dst, errMeshBlankNodeIDMsg); err != nil {
		return ErrInvalidNodeID
	}
	if src == dst {
		return ErrSelfRoute
	}
	return nil
}

func (m *DynamicMesh) isLinkBlocked(l LinkMetrics) bool {
	if l.Blackholed {
		return true
	}
	return m.cfg.BlackholeLatencyThresholdMs > 0 && l.LatencyMs >= m.cfg.BlackholeLatencyThresholdMs
}

func (m *DynamicMesh) compositeCost(l LinkMetrics) float64 {
	latencyTerm := m.cfg.LatencyWeight * l.LatencyMs
	bandwidthTerm := m.cfg.BandwidthWeight / l.BandwidthMbps
	epistemicTerm := m.cfg.EpistemicWeight / (1.0 + l.EpistemicProximity)
	return latencyTerm + bandwidthTerm + epistemicTerm
}

// ComputeRoute calculates the optimal adaptive route from src to dst.
func (m *DynamicMesh) ComputeRoute(src, dst string) (MeshRoute, error) {
	if err := validateEndpoints(src, dst); err != nil {
		return MeshRoute{}, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.nodes[src]; !ok {
		m.telemetry.RouteFailures++
		return MeshRoute{}, ErrMeshNodeNotFound
	}
	if _, ok := m.nodes[dst]; !ok {
		m.telemetry.RouteFailures++
		return MeshRoute{}, ErrMeshNodeNotFound
	}

	route, sawBlackhole, err := m.dijkstraLocked(src, dst)
	if err != nil {
		m.telemetry.RouteFailures++
		if sawBlackhole {
			m.telemetry.BlackholesDetected++
		}
		return MeshRoute{}, err
	}

	m.telemetry.RouteComputations++
	if sawBlackhole {
		route.FallbackUsed = true
		m.telemetry.FallbackReroutes++
	}
	return route, nil
}

// Snapshot returns a thread-safe point-in-time view of the mesh topology and telemetry.
func (m *DynamicMesh) Snapshot() TopologySnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()

	nodes := make([]string, 0, len(m.nodes))
	for id := range m.nodes {
		nodes = append(nodes, id)
	}
	sort.Strings(nodes)

	linkCount := 0
	for _, edges := range m.adj {
		linkCount += len(edges)
	}

	return TopologySnapshot{
		NodeCount:      len(nodes),
		LinkCount:      linkCount,
		Nodes:          nodes,
		DistanceMatrix: copyMatrix(m.distanceMatrix),
		Telemetry:      m.telemetry,
	}
}

// DistanceMatrix returns a deep copy of the current all-pairs shortest-path cost matrix.
func (m *DynamicMesh) DistanceMatrix() map[string]map[string]float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return copyMatrix(m.distanceMatrix)
}

func copyMatrix(src map[string]map[string]float64) map[string]map[string]float64 {
	out := make(map[string]map[string]float64, len(src))
	for rowKey, cols := range src {
		rowCopy := make(map[string]float64, len(cols))
		for colKey, val := range cols {
			rowCopy[colKey] = val
		}
		out[rowKey] = rowCopy
	}
	return out
}

func (m *DynamicMesh) recomputeMatrixLocked() {
	matrix := make(map[string]map[string]float64, len(m.nodes))
	for src := range m.nodes {
		row := make(map[string]float64, len(m.nodes))
		for dst := range m.nodes {
			if src == dst {
				row[dst] = 0
				continue
			}
			route, _, err := m.dijkstraLocked(src, dst)
			if err != nil {
				row[dst] = math.Inf(1)
				continue
			}
			row[dst] = route.TotalCost
		}
		matrix[src] = row
	}
	m.distanceMatrix = matrix
}

type routeItem struct {
	node string
	cost float64
	idx  int
}

type routeHeap []*routeItem

func (h routeHeap) Len() int           { return len(h) }
func (h routeHeap) Less(i, j int) bool { return h[i].cost < h[j].cost }
func (h routeHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].idx = i
	h[j].idx = j
}
func (h *routeHeap) Push(x any) {
	item := x.(*routeItem)
	item.idx = len(*h)
	*h = append(*h, item)
}
func (h *routeHeap) Pop() any {
	old := *h
	n := len(old)
	item := old[n-1]
	item.idx = -1
	*h = old[:n-1]
	return item
}

func (m *DynamicMesh) dijkstraLocked(src, dst string) (MeshRoute, bool, error) {
	dist := make(map[string]float64, len(m.nodes))
	prev := make(map[string]string, len(m.nodes))
	for id := range m.nodes {
		dist[id] = math.Inf(1)
	}
	dist[src] = 0

	pq := &routeHeap{&routeItem{node: src, cost: 0}}
	heap.Init(pq)
	sawBlackhole := false

	for pq.Len() > 0 {
		cur := heap.Pop(pq).(*routeItem)
		if cur.cost > dist[cur.node] {
			continue
		}
		if cur.node == dst {
			break
		}
		for next, metrics := range m.adj[cur.node] {
			if m.isLinkBlocked(metrics) {
				sawBlackhole = true
				continue
			}
			alt := cur.cost + m.compositeCost(metrics)
			if alt < dist[next] {
				dist[next] = alt
				prev[next] = cur.node
				heap.Push(pq, &routeItem{node: next, cost: alt})
			}
		}
	}

	if math.IsInf(dist[dst], 1) {
		if sawBlackhole {
			return MeshRoute{}, true, ErrPartitionBlackhole
		}
		return MeshRoute{}, false, ErrDestinationUnreachable
	}
	return m.buildRouteLocked(src, dst, dist[dst], prev), sawBlackhole, nil
}

func (m *DynamicMesh) buildRouteLocked(src, dst string, totalCost float64, prev map[string]string) MeshRoute {
	var rev []string
	for cur := dst; cur != ""; cur = prev[cur] {
		rev = append(rev, cur)
		if cur == src {
			break
		}
	}
	hops := make([]string, len(rev))
	for i := range rev {
		hops[i] = rev[len(rev)-1-i]
	}

	totalLatency := 0.0
	minBandwidth := math.Inf(1)
	for i := 0; i < len(hops)-1; i++ {
		edge := m.adj[hops[i]][hops[i+1]]
		totalLatency += edge.LatencyMs
		if edge.BandwidthMbps < minBandwidth {
			minBandwidth = edge.BandwidthMbps
		}
	}

	return MeshRoute{
		Source:           src,
		Destination:      dst,
		Hops:             hops,
		TotalCost:        totalCost,
		TotalLatencyMs:   totalLatency,
		MinBandwidthMbps: minBandwidth,
	}
}
