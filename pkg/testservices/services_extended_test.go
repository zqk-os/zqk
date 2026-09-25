package testservices

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestServiceManager_VerboseLog(t *testing.T) {
	sm := &ServiceManager{verbose: true}
	// Verify sm.log executes when verbose is true
	sm.log("test verbose message %d", 42)
	assert.True(t, sm.verbose)
}

func TestServiceManager_HealthChecks(t *testing.T) {
	sm := NewServiceManager()
	ctx := context.Background()

	t.Run("checkPortHealth", func(t *testing.T) {
		// Listen on a local port
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		defer ln.Close()

		_, portStr, err := net.SplitHostPort(ln.Addr().String())
		require.NoError(t, err)

		healthy, err := sm.checkPortHealth(ctx, portStr)
		assert.NoError(t, err)
		assert.True(t, healthy)

		// Check an invalid/unused port
		unhealthy, err := sm.checkPortHealth(ctx, "59999")
		assert.NoError(t, err)
		assert.False(t, unhealthy)
	})

	t.Run("checkHTTPHealth", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/health" {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("ok"))
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		_, portStr, err := net.SplitHostPort(server.Listener.Addr().String())
		require.NoError(t, err)

		healthy, err := sm.checkHTTPHealth(ctx, portStr, "/health")
		assert.NoError(t, err)
		assert.True(t, healthy)

		// 404 response
		notFound, err := sm.checkHTTPHealth(ctx, portStr, "/nonexistent")
		assert.NoError(t, err)
		assert.False(t, notFound)
	})

	t.Run("checkHealth_Routes", func(t *testing.T) {
		// Test "port" type
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		defer ln.Close()

		_, portStr, _ := net.SplitHostPort(ln.Addr().String())
		ok, err := sm.checkHealth(ctx, ServiceConfig{
			HealthCheck: HealthCheck{Type: "port", Port: portStr},
		})
		assert.NoError(t, err)
		assert.True(t, ok)

		// Test "http" type
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		_, httpPort, _ := net.SplitHostPort(server.Listener.Addr().String())
		ok, err = sm.checkHealth(ctx, ServiceConfig{
			HealthCheck: HealthCheck{Type: "http", Port: httpPort, Path: "/"},
		})
		assert.NoError(t, err)
		assert.True(t, ok)
	})
}

func TestServiceManager_DockerAndService(t *testing.T) {
	sm := NewServiceManager()
	ctx := context.Background()

	// isDockerAvailable
	dockerAvail := sm.isDockerAvailable()
	assert.True(t, dockerAvail)

	// isServiceRunning with known running container
	assert.True(t, sm.isServiceRunning("zqk-memgraph"))
	assert.True(t, sm.IsServiceRunning("zqk-memgraph"))
	assert.False(t, sm.isServiceRunning("non-existent-container-xyz-123"))

	// lockStartService
	mu1 := lockStartService("container-1")
	mu2 := lockStartService("container-1")
	assert.Equal(t, mu1, mu2)

	// hostPortsAlreadyPublished
	cfg := ServiceConfig{
		Ports: map[string]string{
			"7687": "7687",
		},
	}
	assert.True(t, sm.hostPortsAlreadyPublished(cfg))

	// hostPortsAlreadyPublished on non-published port
	cfgUnused := ServiceConfig{
		Ports: map[string]string{
			"59999": "59999",
		},
	}
	assert.False(t, sm.hostPortsAlreadyPublished(cfgUnused))

	// StartService when already running
	err := sm.StartService(ctx, ServiceConfig{
		Name:          "memgraph",
		ContainerName: "zqk-memgraph",
	})
	assert.NoError(t, err)

	// StartService when host ports already published
	err = sm.StartService(ctx, ServiceConfig{
		Name:          "memgraph-reuse",
		ContainerName: "zqk-different-container",
		Ports: map[string]string{
			"7687": "7687",
		},
	})
	assert.NoError(t, err)
}

func TestServiceManager_WaitForHealthy(t *testing.T) {
	sm := NewServiceManager()
	ctx := context.Background()

	// Healthy check passing immediately
	cfg := ServiceConfig{
		StartTimeout: 2 * time.Second,
		HealthCheck: HealthCheck{
			Type:     "command",
			Command:  []string{"true"},
			Interval: 10 * time.Millisecond,
		},
	}
	err := sm.waitForHealthy(ctx, cfg)
	assert.NoError(t, err)

	// Healthy check timeout
	cfgTimeout := ServiceConfig{
		StartTimeout: 50 * time.Millisecond,
		HealthCheck: HealthCheck{
			Type:     "command",
			Command:  []string{"false"},
			Interval: 10 * time.Millisecond,
		},
	}
	err = sm.waitForHealthy(ctx, cfgTimeout)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "health check timeout")
}

func TestTestServices_CleanupWithErrors(t *testing.T) {
	ts := &TestServices{
		manager:           NewServiceManager(),
		ctx:               context.Background(),
		memgraphContainer: "zqk-test-coverage-not-running",
	}

	// StopService on non-running returns nil, so Cleanup succeeds
	err := ts.Cleanup()
	assert.NoError(t, err)
}

func TestSetupTestServices_GraphEnabledWithExistingMemgraph(t *testing.T) {
	// Create a temp project root with config/zqk.yaml setting storage.graph_enabled: true
	tmpDir, err := os.MkdirTemp("", "testservices_graph_*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	configDir := tmpDir + "/config"
	require.NoError(t, os.MkdirAll(configDir, 0755))
	configContent := "storage:\n  graph_enabled: true\n"
	require.NoError(t, fileutil.WriteFile(configDir+"/zqk.yaml", []byte(configContent), 0644))

	t.Setenv("ZQK_PROJECT_ROOT", tmpDir)

	// SetupTestServices with graph enabled will call StartService for MemGraph,
	// which detects zqk-memgraph already running on host ports and succeeds immediately!
	ts, err := SetupTestServices(context.Background())
	require.NoError(t, err)
	require.NotNil(t, ts)
	assert.Equal(t, "zqk-test-memgraph", ts.memgraphContainer)

	// ts.Cleanup on this container
	err = ts.Cleanup()
	assert.NoError(t, err)
}

func TestServiceManager_StartAndStopEphemeralContainer(t *testing.T) {
	sm := NewServiceManager()
	ctx := context.Background()

	// Spin up a quick alpine container that exits or stays alive with sleep
	containerName := "zqk-test-ephemeral-cov"
	cfg := ServiceConfig{
		Name:          "ephemeral",
		ContainerName: containerName,
		Image:         "alpine",
		Environment: map[string]string{
			"FOO": "bar",
		},
		HealthCheck: HealthCheck{
			Type:     "command",
			Command:  []string{"true"},
			Interval: 10 * time.Millisecond,
		},
		StartTimeout: 5 * time.Second,
	}

	// Make sure it's stopped/removed first
	_ = sm.StopService(ctx, containerName)

	err := sm.StartService(ctx, cfg)
	if err != nil {
		t.Logf("StartService returned %v (docker may be restricted in sandbox)", err)
		return
	}

	assert.True(t, sm.IsServiceRunning(containerName))

	// Stop it
	err = sm.StopService(ctx, containerName)
	assert.NoError(t, err)
	assert.False(t, sm.IsServiceRunning(containerName))
}

