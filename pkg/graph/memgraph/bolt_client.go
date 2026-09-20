package memgraph

import (
	"context"
	"fmt"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// boltClient handles Bolt protocol communication with MemGraph
// MemGraph is compatible with Neo4j Bolt protocol
type boltClient struct {
	driver neo4j.DriverWithContext
	config MemGraphConfig
}

// newBoltClient creates a new MemGraph Bolt client
func newBoltClient(config *MemGraphConfig) (*boltClient, error) {
	// MemGraph uses Bolt protocol on port 7687 (same as Neo4j)
	uri := fmt.Sprintf("bolt://%s:%d", config.Host, config.Port)
	if config.Port == 0 {
		uri = fmt.Sprintf("bolt://%s:7687", config.Host)
	}

	auth := neo4j.BasicAuth(config.Username, config.Password, "")

	driver, err := neo4j.NewDriverWithContext(uri, auth)
	if err != nil {
		return nil, errfmt.Newf("failed to create Bolt driver").Wrap(err)
	}

	return &boltClient{
		driver: driver,
		config: *config,
	}, nil
}

// verifyConnectivity verifies the connection is working
func (c *boltClient) verifyConnectivity(ctx context.Context) error {
	return c.driver.VerifyConnectivity(ctx)
}

// close closes the driver
//
//nolint:unused // Reserved for future use or interface implementation
func (c *boltClient) close(ctx context.Context) error {
	return c.driver.Close(ctx)
}

// newSession creates a new Bolt session
//
//nolint:gocritic // SessionConfig is defined by the driver; value pass is required
func (c *boltClient) newSession(ctx context.Context, config neo4j.SessionConfig) neo4j.SessionWithContext {
	return c.driver.NewSession(ctx, config)
}
