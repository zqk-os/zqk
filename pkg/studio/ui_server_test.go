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

func getSuccessfulTestEndpoint(t *testing.T, baseURL, path string) *http.Response {
	t.Helper()
	resp, err := http.Get(baseURL + path)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Cache-Control"), "no-cache")
	return resp
}

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
		Attributes: map[string]any{
			"target_date":      "2026-10-15",
			"estimated_effort": "2w",
		},
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
		require.NoError(t, server.Shutdown(ctx))
	})

	return server, indexer, fmt.Sprintf("http://%s", addr)
}

func TestServer_HealthEndpoint(t *testing.T) {
	_, indexer, baseURL := setupTestStudioServer(t)

	resp := getSuccessfulTestEndpoint(t, baseURL, "/api/health")
	defer resp.Body.Close()

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

	resp := getSuccessfulTestEndpoint(t, baseURL, "/api/graph")
	defer resp.Body.Close()

	var payload studio.GraphPayload
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&payload))

	assert.NotEmpty(t, payload.Nodes)
	assert.NotEmpty(t, payload.Edges)

	// Validate nodes contain seeded objects
	nodeMap := make(map[string]studio.GraphNode)
	for _, n := range payload.Nodes {
		nodeMap[n.ID] = n
	}
	assert.Contains(t, nodeMap, "GOAL-TEST-001")
	assert.Contains(t, nodeMap, "MIL-TEST-001")
	assert.Contains(t, nodeMap, "BLI-TEST-001")

	// Validate attributes mapped to GraphNode fields
	assert.Equal(t, "2026-10-15", nodeMap["MIL-TEST-001"].TargetDate)
	assert.Equal(t, "2w", nodeMap["MIL-TEST-001"].EstimatedEffort)

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

	resp := getSuccessfulTestEndpoint(t, baseURL, "/")
	defer resp.Body.Close()

	assert.Contains(t, resp.Header.Get("Content-Type"), "text/html")

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	bodyStr := string(body)
	assert.Contains(t, bodyStr, "ZQK Knowledge Kernel Visual Studio")
	assert.Contains(t, bodyStr, "Interactive Knowledge Graph DAG")
	assert.Contains(t, bodyStr, "gantt-legend-bar")
	assert.Contains(t, bodyStr, "Milestone")
	assert.Contains(t, bodyStr, "Today Line")
}

func TestServer_ObjectsEndpoint(t *testing.T) {
	_, _, baseURL := setupTestStudioServer(t)

	// List objects
	resp := getSuccessfulTestEndpoint(t, baseURL, "/api/objects?kind=backlog_item")
	defer resp.Body.Close()

	var objects []*storage.IndexedNode
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&objects))
	require.Len(t, objects, 1)
	assert.Equal(t, "BLI-TEST-001", objects[0].ID)

	// Get single object
	respObj := getSuccessfulTestEndpoint(t, baseURL, "/api/objects/BLI-TEST-001")
	defer respObj.Body.Close()

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

func TestServer_AssetsEndpoints(t *testing.T) {
	_, _, baseURL := setupTestStudioServer(t)

	// Test CSS asset endpoint
	respCSS, err := http.Get(baseURL + "/assets/style.css")
	require.NoError(t, err)
	defer respCSS.Body.Close()
	assert.Equal(t, http.StatusOK, respCSS.StatusCode)
	assert.Contains(t, respCSS.Header.Get("Content-Type"), "text/css")
	cssBytes, err := io.ReadAll(respCSS.Body)
	require.NoError(t, err)
	assert.NotEmpty(t, cssBytes)
	assert.Contains(t, string(cssBytes), ":root")

	// Test JavaScript asset endpoint
	respJS, err := http.Get(baseURL + "/assets/app.js")
	require.NoError(t, err)
	defer respJS.Body.Close()
	assert.Equal(t, http.StatusOK, respJS.StatusCode)
	assert.Contains(t, respJS.Header.Get("Content-Type"), "application/javascript")
	jsBytes, err := io.ReadAll(respJS.Body)
	require.NoError(t, err)
	assert.NotEmpty(t, jsBytes)

	// Test 404 for unknown asset
	respNotFound, err := http.Get(baseURL + "/assets/unknown.xyz")
	require.NoError(t, err)
	defer respNotFound.Body.Close()
	assert.Equal(t, http.StatusNotFound, respNotFound.StatusCode)
}
