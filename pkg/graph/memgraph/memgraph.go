package memgraph

import (
	"context"
	"fmt"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// Client provides access to a Memgraph database using the Neo4j driver.
type Client struct {
	driver neo4j.DriverWithContext
}

// NewClient creates a new Memgraph client.
func NewClient(uri, username, password string) (*Client, error) {
	driver, err := neo4j.NewDriverWithContext(uri, neo4j.BasicAuth(username, password, ""))
	if err != nil {
		return nil, fmt.Errorf("failed to create memgraph driver: %w", err)
	}

	return &Client{
		driver: driver,
	}, nil
}

// Close closes the underlying driver connection.
func (c *Client) Close(ctx context.Context) error {
	if c.driver != nil {
		return c.driver.Close(ctx)
	}
	return nil
}

// Ping verifies connectivity to the Memgraph database.
func (c *Client) Ping(ctx context.Context) error {
	if c.driver == nil {
		return fmt.Errorf("driver is not initialized")
	}
	return c.driver.VerifyConnectivity(ctx)
}
