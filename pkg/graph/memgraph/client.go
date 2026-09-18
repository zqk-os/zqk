package memgraph

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/httpheaders"
)

// memgraphClient handles HTTP communication with MemGraph REST API
//
//nolint:unused // Reserved for future MemGraph HTTP client implementation
type memgraphClient struct {
	baseURL    string
	httpClient *http.Client
}

// newMemgraphClient creates a new MemGraph HTTP client
//
//nolint:unused // Reserved for future MemGraph HTTP client implementation
func newMemgraphClient(config *MemGraphConfig) *memgraphClient {
	port := config.Port
	if port == 0 {
		port = 7444 // Default HTTP port
	}

	baseURL := fmt.Sprintf("http://%s:%d", config.Host, port)

	return &memgraphClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// executeQuery executes a Cypher query via REST API
//
//nolint:unused // Reserved for future MemGraph HTTP client implementation
func (c *memgraphClient) executeQuery(ctx context.Context, query string, params map[string]any) (*queryResponse, error) {
	// MemGraph REST API endpoint for Cypher queries
	url := fmt.Sprintf("%s/cypher", c.baseURL)

	// Prepare request body
	requestBody := map[string]any{
		"query":  query,
		"params": params,
	}

	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return nil, errfmt.Newf("failed to marshal request").Wrap(err)
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, errfmt.Newf("failed to create request").Wrap(err)
	}

	req.Header.Set(httpheaders.ContentType, "application/json")

	// Execute request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, errfmt.Newf("failed to execute request").Wrap(err)
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errfmt.Newf("failed to read response").Wrap(err)
	}

	// Check status code
	if resp.StatusCode != http.StatusOK {
		return nil, errfmt.Errorf("query failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var queryResp queryResponse
	if err := json.Unmarshal(body, &queryResp); err != nil {
		return nil, errfmt.Newf("failed to parse response").Wrap(err)
	}

	// Check for errors in response
	if len(queryResp.Errors) > 0 {
		return nil, errfmt.Errorf("query error: %v", queryResp.Errors)
	}

	return &queryResp, nil
}

// queryResponse represents a MemGraph REST API query response
//
//nolint:unused // Reserved for future MemGraph HTTP client implementation
type queryResponse struct {
	Results []map[string]any `json:"results"`
	Errors  []map[string]any `json:"errors,omitempty"`
}

// healthCheck performs a simple health check query
//
//nolint:unused // Reserved for future MemGraph HTTP client implementation
func (c *memgraphClient) healthCheck(ctx context.Context) error {
	_, err := c.executeQuery(ctx, "RETURN 1 AS health", nil)
	return err
}
