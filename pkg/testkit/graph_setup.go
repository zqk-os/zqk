package testkit

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/config"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/graph/memgraph"
	"github.com/zqk-os/zqk/pkg/graph/provider"
	"github.com/zqk-os/zqk/pkg/testservices"
)

// PrepareGraphConnectionForTest sets up test services and returns a raw graph connection pool.
// It skips the test if ZQK_GRAPH_ENABLED is not set.
func PrepareGraphConnectionForTest(t *testing.T) provider.ConnectionPool {
	t.Helper()
	if !config.StorageGraphEnabled().OrDefault(false) {
		t.Skip("Graph backend not enabled (set ZQK_GRAPH_ENABLED=true)")
	}

	ctx, cancel := context.WithTimeout(pkgctx.NewSystemContext(), 2*time.Minute)
	t.Cleanup(cancel)

	services, err := testservices.SetupTestServices(ctx)
	if err != nil {
		if strings.Contains(err.Error(), "docker is not available") {
			t.Skipf("skipping test: %v", err)
		}
		t.Fatalf("failed to setup test services: %v", err)
	}
	t.Cleanup(func() { _ = services.Cleanup() })

	config := memgraph.MemGraphConfig{
		Host:     "localhost",
		Port:     7687,
		Username: "admin",
		Password: "password",
		Database: "test",
		PoolSize: 10,
	}
	mgProvider := memgraph.NewMemGraphProvider(&config)
	pool, err := mgProvider.CreatePool(ctx, provider.ConnectionConfig{MaxConns: 10})
	if err != nil {
		t.Fatalf("failed to get graph connection pool: %v", err)
	}

	return pool
}
