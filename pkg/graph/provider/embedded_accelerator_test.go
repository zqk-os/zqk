package provider_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/graph/provider"
)

// TestCRIT_1789333118970585000_a2d88dd2 verifies CRIT-REDACTED:
// Vertex-label index lookup and 1-hop traverse stay bound without a full graph scan.
// Covering BLI-REDACTED (embedded vertex-label index cache) and
// BLI-REDACTED (bound 1-hop edge traverser / Cypher-subset accelerator).
func TestCRIT_1789333118970585000_a2d88dd2(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	acc := provider.NewEmbeddedGraphAccelerator(100, 2*time.Second)

	// Step 1: Populate 500 nodes across multiple vertex labels
	const totalNodes = 500
	for i := 0; i < totalNodes; i++ {
		label := "UnrelatedKind"
		if i%5 == 0 {
			label = "BacklogItem"
		} else if i%5 == 1 {
			label = "Requirement"
		} else if i%5 == 2 {
			label = "Criteria"
		}
		node := &provider.Node{
			ID:     fmt.Sprintf("NODE-%04d", i),
			Labels: []string{label},
			Properties: map[string]any{
				"index": i,
				"name":  fmt.Sprintf("Item %d", i),
			},
		}
		acc.IndexNode(node)
	}

	// Step 2: Populate edges forming a multi-hop graph structure
	for i := 0; i < totalNodes-1; i++ {
		edge := &provider.Edge{
			FromID: fmt.Sprintf("NODE-%04d", i),
			ToID:   fmt.Sprintf("NODE-%04d", i+1),
			Type:   "COVERS",
			Properties: map[string]any{
				"weight": 1.0,
			},
		}
		acc.IndexEdge(edge)
	}

	// Step 3: Vertex-label index lookup stays bound without full graph scan (BLI-REDACTED)
	startTime := time.Now()
	nodes, err := acc.LookupByLabel(ctx, "BacklogItem", 50)
	duration := time.Since(startTime)

	if err != nil {
		t.Fatalf("LookupByLabel failed: %v", err)
	}
	if len(nodes) != 50 {
		t.Fatalf("expected 50 nodes (bounded limit), got %d", len(nodes))
	}
	// Sub-millisecond guarantee on in-memory index
	if duration > 100*time.Millisecond {
		t.Errorf("LookupByLabel took %v, expected sub-millisecond", duration)
	}
	for _, n := range nodes {
		if len(n.Labels) == 0 || n.Labels[0] != "BacklogItem" {
			t.Fatalf("node %s has unexpected labels %v", n.ID, n.Labels)
		}
	}

	// Step 4: 1-hop edge traverse stays bound without full graph scan (BLI-REDACTED)
	startNodeID := "NODE-0000"
	tq := provider.TraversalQuery{
		StartNodeID:  startNodeID,
		Relationship: "COVERS",
		Direction:    provider.DirectionOutgoing,
		MaxDepth:     1,
	}

	startTime = time.Now()
	res, err := acc.Traverse1Hop(ctx, tq)
	duration = time.Since(startTime)

	if err != nil {
		t.Fatalf("Traverse1Hop failed: %v", err)
	}
	if len(res.Edges) != 1 {
		t.Fatalf("expected exactly 1 1-hop edge from %s, got %d", startNodeID, len(res.Edges))
	}
	if res.Edges[0].ToID != "NODE-0001" {
		t.Fatalf("expected edge to NODE-0001, got %s", res.Edges[0].ToID)
	}
	if len(res.Nodes) != 1 || res.Nodes[0].ID != "NODE-0001" {
		t.Fatalf("expected target node NODE-0001, got %v", res.Nodes)
	}
	if duration > 100*time.Millisecond {
		t.Errorf("Traverse1Hop took %v, expected sub-millisecond", duration)
	}

	// Step 5: Verify zero full-graph scans occurred across all operations
	if fullScans := acc.FullScanCount(); fullScans != 0 {
		t.Fatalf("expected exactly 0 full graph scans, found %d", fullScans)
	}
}

// TestVertexLabelIndex_CacheOperationsAndBound verifies inverted index operations,
// node updates, and bound enforcement.
func TestVertexLabelIndex_CacheOperationsAndBound(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	idx := provider.NewVertexLabelIndex()

	nodeA := &provider.Node{ID: "N-A", Labels: []string{"Task", "Priority"}}
	nodeB := &provider.Node{ID: "N-B", Labels: []string{"Task"}}
	nodeC := &provider.Node{ID: "N-C", Labels: []string{"Epic"}}

	idx.IndexNode(nodeA)
	idx.IndexNode(nodeB)
	idx.IndexNode(nodeC)

	if idx.TotalIndexedNodes() != 3 {
		t.Fatalf("expected 3 total indexed nodes, got %d", idx.TotalIndexedNodes())
	}
	if idx.NodeCountForLabel("Task") != 2 {
		t.Fatalf("expected 2 nodes with label Task, got %d", idx.NodeCountForLabel("Task"))
	}
	if idx.NodeCountForLabel("Epic") != 1 {
		t.Fatalf("expected 1 node with label Epic, got %d", idx.NodeCountForLabel("Epic"))
	}

	// Lookup bounded to 1 item
	tasks, err := idx.LookupByLabel(ctx, "Task", 1)
	if err != nil {
		t.Fatalf("LookupByLabel failed: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected bounded result count 1, got %d", len(tasks))
	}

	// Update node A to drop Task label
	nodeAUpdated := &provider.Node{ID: "N-A", Labels: []string{"Archived"}}
	idx.IndexNode(nodeAUpdated)

	if idx.NodeCountForLabel("Task") != 1 {
		t.Fatalf("expected 1 node with label Task after update, got %d", idx.NodeCountForLabel("Task"))
	}
	if idx.NodeCountForLabel("Archived") != 1 {
		t.Fatalf("expected 1 node with label Archived, got %d", idx.NodeCountForLabel("Archived"))
	}

	// Remove node B
	idx.RemoveNode("N-B")
	if idx.NodeCountForLabel("Task") != 0 {
		t.Fatalf("expected 0 nodes with label Task after removal, got %d", idx.NodeCountForLabel("Task"))
	}

	// Fail closed on empty label
	_, err = idx.LookupByLabel(ctx, "", 10)
	if err == nil {
		t.Fatal("expected error on empty label lookup, got nil")
	}

	// Verify 0 full graph scans
	if idx.FullScanCount() != 0 {
		t.Fatalf("expected 0 full scans, got %d", idx.FullScanCount())
	}
}

// TestBoundEdgeTraverser_1HopAdjacencyAndFiltering verifies 1-hop traversal directions,
// label filtering, and bound caps without full graph scans.
func TestBoundEdgeTraverser_1HopAdjacencyAndFiltering(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	acc := provider.NewEmbeddedGraphAccelerator(10, 2*time.Second)

	hub := &provider.Node{ID: "HUB", Labels: []string{"Hub"}}
	child1 := &provider.Node{ID: "CHILD-1", Labels: []string{"Worker", "Active"}}
	child2 := &provider.Node{ID: "CHILD-2", Labels: []string{"Worker", "Idle"}}
	child3 := &provider.Node{ID: "CHILD-3", Labels: []string{"Manager"}}
	parent := &provider.Node{ID: "PARENT", Labels: []string{"Root"}}

	acc.IndexNode(hub)
	acc.IndexNode(child1)
	acc.IndexNode(child2)
	acc.IndexNode(child3)
	acc.IndexNode(parent)

	// Outgoing edges from HUB
	acc.IndexEdge(&provider.Edge{FromID: "HUB", ToID: "CHILD-1", Type: "OWNS"})
	acc.IndexEdge(&provider.Edge{FromID: "HUB", ToID: "CHILD-2", Type: "OWNS"})
	acc.IndexEdge(&provider.Edge{FromID: "HUB", ToID: "CHILD-3", Type: "MANAGES"})

	// Incoming edge to HUB
	acc.IndexEdge(&provider.Edge{FromID: "PARENT", ToID: "HUB", Type: "CONTAINS"})

	// Traverse outgoing with relationship filter
	resOwns, err := acc.Traverse1Hop(ctx, provider.TraversalQuery{
		StartNodeID:  "HUB",
		Relationship: "OWNS",
		Direction:    provider.DirectionOutgoing,
	})
	if err != nil {
		t.Fatalf("traverse OWNS failed: %v", err)
	}
	if len(resOwns.Nodes) != 2 {
		t.Fatalf("expected 2 nodes for OWNS, got %d", len(resOwns.Nodes))
	}

	// Traverse outgoing with label filter
	var filter provider.NodeFilter
	filter.Labels = []string{"Active"}
	resActive, err := acc.Traverse1Hop(ctx, provider.TraversalQuery{
		StartNodeID:  "HUB",
		Relationship: "OWNS",
		Direction:    provider.DirectionOutgoing,
		Filter:       filter,
	})
	if err != nil {
		t.Fatalf("traverse with label filter failed: %v", err)
	}
	if len(resActive.Nodes) != 1 || resActive.Nodes[0].ID != "CHILD-1" {
		t.Fatalf("expected only CHILD-1 for Active filter, got %v", resActive.Nodes)
	}

	// Traverse incoming
	resIncoming, err := acc.Traverse1Hop(ctx, provider.TraversalQuery{
		StartNodeID: "HUB",
		Direction:   provider.DirectionIncoming,
	})
	if err != nil {
		t.Fatalf("traverse incoming failed: %v", err)
	}
	if len(resIncoming.Nodes) != 1 || resIncoming.Nodes[0].ID != "PARENT" {
		t.Fatalf("expected PARENT for incoming, got %v", resIncoming.Nodes)
	}

	// Remove an edge and verify traversal updates
	acc.RemoveEdge("HUB", "CHILD-2", "OWNS")
	resAfterRemoval, err := acc.Traverse1Hop(ctx, provider.TraversalQuery{
		StartNodeID:  "HUB",
		Relationship: "OWNS",
		Direction:    provider.DirectionOutgoing,
	})
	if err != nil {
		t.Fatalf("traverse after removal failed: %v", err)
	}
	if len(resAfterRemoval.Nodes) != 1 || resAfterRemoval.Nodes[0].ID != "CHILD-1" {
		t.Fatalf("expected 1 node (CHILD-1) after edge removal, got %v", resAfterRemoval.Nodes)
	}

	// Verify 0 full graph scans
	if acc.FullScanCount() != 0 {
		t.Fatalf("expected 0 full scans, got %d", acc.FullScanCount())
	}
}

// TestEmbeddedGraphAccelerator_Cypher1HopAccelerator verifies accelerated Cypher queries.
func TestEmbeddedGraphAccelerator_Cypher1HopAccelerator(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	acc := provider.NewEmbeddedGraphAccelerator(100, 2*time.Second)

	acc.IndexNode(&provider.Node{ID: "REQ-001", Labels: []string{"Requirement"}})
	acc.IndexNode(&provider.Node{ID: "REQ-002", Labels: []string{"Requirement"}})
	acc.IndexNode(&provider.Node{ID: "CRIT-001", Labels: []string{"Criteria"}})
	acc.IndexEdge(&provider.Edge{FromID: "REQ-001", ToID: "CRIT-001", Type: "COVERS"})

	// Query 1: MATCH (n:Requirement) RETURN n
	res1, err := acc.ExecuteCypher(ctx, "MATCH (n:Requirement) RETURN n", nil)
	if err != nil {
		t.Fatalf("Cypher query 1 failed: %v", err)
	}
	if len(res1.Nodes) != 2 {
		t.Fatalf("expected 2 requirement nodes, got %d", len(res1.Nodes))
	}

	// Query 2: MATCH (a:Requirement)-[r:COVERS]->(b:Criteria) WHERE a.id = $id RETURN b
	res2, err := acc.ExecuteCypher(ctx, "MATCH (a:Requirement)-[r:COVERS]->(b:Criteria) WHERE a.id = $id RETURN b", map[string]any{
		"id": "REQ-001",
	})
	if err != nil {
		t.Fatalf("Cypher query 2 failed: %v", err)
	}
	if len(res2.Nodes) != 1 || res2.Nodes[0].ID != "CRIT-001" {
		t.Fatalf("expected CRIT-001 target node, got %v", res2.Nodes)
	}

	// Query 3: Fail closed on unsupported or malformed Cypher
	_, err = acc.ExecuteCypher(ctx, "DROP DATABASE production;", nil)
	if err == nil {
		t.Fatal("expected error on unsupported Cypher, got nil")
	}

	// Verify 0 full graph scans
	if acc.FullScanCount() != 0 {
		t.Fatalf("expected 0 full scans, got %d", acc.FullScanCount())
	}
}

// TestEmbeddedGraphAccelerator_FailClosedOnTimeout verifies that operations fail closed
// when context deadline expires or context is cancelled.
func TestEmbeddedGraphAccelerator_FailClosedOnTimeout(t *testing.T) {
	t.Parallel()
	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	acc := provider.NewEmbeddedGraphAccelerator(50, 1*time.Second)
	acc.IndexNode(&provider.Node{ID: "N-1", Labels: []string{"Label1"}})

	// LookupByLabel must fail closed
	_, err := acc.LookupByLabel(cancelledCtx, "Label1", 10)
	if err == nil {
		t.Fatal("expected error on cancelled context for LookupByLabel, got nil")
	}

	// Traverse1Hop must fail closed
	_, err = acc.Traverse1Hop(cancelledCtx, provider.TraversalQuery{StartNodeID: "N-1"})
	if err == nil {
		t.Fatal("expected error on cancelled context for Traverse1Hop, got nil")
	}

	// ExecuteCypher must fail closed
	_, err = acc.ExecuteCypher(cancelledCtx, "MATCH (n:Label1) RETURN n", nil)
	if err == nil {
		t.Fatal("expected error on cancelled context for ExecuteCypher, got nil")
	}
}

// TestMockConnection_IntegrationWithAccelerator verifies that MockGraphProvider
// operations transparently leverage the accelerator with 0 full scans.
func TestMockConnection_IntegrationWithAccelerator(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	p := provider.NewMockGraphProvider()
	conn, err := p.Connect(ctx, provider.ConnectionConfig{})
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	// Create nodes
	err = conn.CreateNode(ctx, provider.Node{
		ID:     "BLI-TEST-001",
		Labels: []string{"BacklogItem"},
		Properties: map[string]any{
			"title": "Test BLI",
		},
	})
	if err != nil {
		t.Fatalf("CreateNode failed: %v", err)
	}

	err = conn.CreateNode(ctx, provider.Node{
		ID:     "CRIT-TEST-001",
		Labels: []string{"Criteria"},
		Properties: map[string]any{
			"title": "Test CRIT",
		},
	})
	if err != nil {
		t.Fatalf("CreateNode failed: %v", err)
	}

	// Create edge
	err = conn.CreateEdge(ctx, provider.Edge{
		FromID: "BLI-TEST-001",
		ToID:   "CRIT-TEST-001",
		Type:   "SATISFIES",
	})
	if err != nil {
		t.Fatalf("CreateEdge failed: %v", err)
	}

	// ListNodes with label uses index
	nodes, err := conn.ListNodes(ctx, provider.NodeFilter{
		Labels: []string{"BacklogItem"},
	})
	if err != nil {
		t.Fatalf("ListNodes failed: %v", err)
	}
	if len(nodes) != 1 || nodes[0].ID != "BLI-TEST-001" {
		t.Fatalf("expected BLI-TEST-001, got %v", nodes)
	}

	// Traversal uses 1-hop traverser
	travRes, err := conn.ExecuteTraversal(ctx, provider.TraversalQuery{
		StartNodeID:  "BLI-TEST-001",
		Relationship: "SATISFIES",
		Direction:    provider.DirectionOutgoing,
	})
	if err != nil {
		t.Fatalf("ExecuteTraversal failed: %v", err)
	}
	if len(travRes.Nodes) != 1 || travRes.Nodes[0].ID != "CRIT-TEST-001" {
		t.Fatalf("expected CRIT-TEST-001 target, got %v", travRes.Nodes)
	}

	// ExecuteQuery uses accelerator
	queryRes, err := conn.ExecuteQuery(ctx, provider.Query{
		Query: "MATCH (n:BacklogItem) RETURN n",
	})
	if err != nil {
		t.Fatalf("ExecuteQuery failed: %v", err)
	}
	if len(queryRes.Nodes) != 1 || queryRes.Nodes[0].ID != "BLI-TEST-001" {
		t.Fatalf("expected BLI-TEST-001 from Cypher query, got %v", queryRes.Nodes)
	}

	// Check MockConnection.Accelerator().FullScanCount() == 0
	mockConn, ok := conn.(*provider.MockConnection)
	if !ok {
		t.Fatal("expected *MockConnection")
	}
	if mockConn.Accelerator().FullScanCount() != 0 {
		t.Fatalf("expected 0 full scans, got %d", mockConn.Accelerator().FullScanCount())
	}
}
