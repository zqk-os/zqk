package studio_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/studio"
)

// Satisfies TST-VISUAL-STUDIO-UI-001 and CRIT-VISUAL-STUDIO-UI-001:
// Verify Embedded Local Web UI Server and API Endpoints

func setupTestStudioServer(t *testing.T) (*studio.Server, *storage.PureGoIndexer, string) {
	indexer := storage.NewPureGoIndexer()

	// Seed test hierarchy
	goal := &storage.IndexedNode{
		ID:     "GOAL-TEST-001",
		Kind:   "goal",
		Status: "originated",
		Title:  "Launch Visual Studio Web UI",
	}
	milestone := &storage.IndexedNode{
		ID:     "MIL-TEST-001",
		Kind:   "milestone",
		Status: "in_progress",
		Title:  "Phase 1 Visual Studio UI",
		References: map[string][]string{
			"goal_refs": {"GOAL-TEST-001"},
		},
	}
	bli := &storage.IndexedNode{
		ID:     "BLI-TEST-001",
		Kind:   "backlog_item",
		Status: "planned",
		Title:  "Implement Embedded HTTP Server",
		References: map[string][]string{
			"milestone_refs": {"MIL-TEST-001"},
		},
	}

	require.NoError(t, indexer.IndexNode(goal))
	require.NoError(t, indexer.IndexNode(milestone))
	require.NoError(t, indexer.IndexNode(bli))

	server := studio.NewServer(t.TempDir(), studio.WithIndexer(indexer))
	require.NoError(t, server.Start("127.0.0.1:0"))

	addr := server.Addr()
	require.NotEmpty(t, addr)

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	})

	return server, indexer, fmt.Sprintf("http://%s", addr)
}

func TestServer_HealthEndpoint(t *testing.T) {
	_, indexer, baseURL := setupTestStudioServer(t)

	resp, err := http.Get(baseURL + "/api/health")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))

	var data struct {
		Status    string `json:"status"`
		NodeCount int    `json:"nodeCount"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&data))
	assert.Equal(t, "ok", data.Status)
	assert.Equal(t, indexer.NodeCount(), data.NodeCount)
}

func TestServer_GraphEndpoint(t *testing.T) {
	_, _, baseURL := setupTestStudioServer(t)

	resp, err := http.Get(baseURL + "/api/graph")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var payload studio.GraphPayload
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&payload))

	assert.NotEmpty(t, payload.Nodes)
	assert.NotEmpty(t, payload.Edges)

	// Validate nodes contain seeded objects
	nodeIDs := make(map[string]bool)
	for _, n := range payload.Nodes {
		nodeIDs[n.ID] = true
	}
	assert.True(t, nodeIDs["GOAL-TEST-001"])
	assert.True(t, nodeIDs["MIL-TEST-001"])
	assert.True(t, nodeIDs["BLI-TEST-001"])

	// Validate directed edge
	var foundEdge bool
	for _, e := range payload.Edges {
		if e.Source == "BLI-TEST-001" && e.Target == "MIL-TEST-001" && e.Relation == "milestone_refs" {
			foundEdge = true
			break
		}
	}
	assert.True(t, foundEdge, "expected directed edge from BLI-TEST-001 to MIL-TEST-001")
}

func TestServer_DashboardHTML(t *testing.T) {
	_, _, baseURL := setupTestStudioServer(t)

	resp, err := http.Get(baseURL + "/")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Content-Type"), "text/html")

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Contains(t, string(body), "ZQK Knowledge Kernel Visual Studio")
	assert.Contains(t, string(body), "Interactive Knowledge Graph DAG")
}

func TestServer_ObjectsEndpoint(t *testing.T) {
	_, _, baseURL := setupTestStudioServer(t)

	// List objects
	resp, err := http.Get(baseURL + "/api/objects?kind=backlog_item")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	var objects []*storage.IndexedNode
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&objects))
	require.Len(t, objects, 1)
	assert.Equal(t, "BLI-TEST-001", objects[0].ID)

	// Get single object
	respObj, err := http.Get(baseURL + "/api/objects/BLI-TEST-001")
	require.NoError(t, err)
	defer respObj.Body.Close()

	assert.Equal(t, http.StatusOK, respObj.StatusCode)
	var single storage.IndexedNode
	require.NoError(t, json.NewDecoder(respObj.Body).Decode(&single))
	assert.Equal(t, "BLI-TEST-001", single.ID)
	assert.Equal(t, "planned", single.Status)

	// Non-existent object
	respMissing, err := http.Get(baseURL + "/api/objects/NONEXISTENT-999")
	require.NoError(t, err)
	defer respMissing.Body.Close()
	assert.Equal(t, http.StatusNotFound, respMissing.StatusCode)
}
