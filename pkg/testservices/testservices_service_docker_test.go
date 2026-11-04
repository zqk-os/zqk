// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package testservices

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// setupMockDockerEnvironment installs a script named docker into a temporary directory
// and prepends it to PATH so that exec calls to docker are safely intercepted.
func setupMockDockerEnvironment(t *testing.T) {
	t.Helper()
	binDir := t.TempDir()
	dockerPath := filepath.Join(binDir, "docker")
	script := `#!/bin/sh
case "$1" in
  info)
    if [ "$MOCK_DOCKER_INFO_FAIL" = "1" ]; then
      echo "Cannot connect to Docker daemon" >&2
      exit 1
    fi
    echo "Client: Docker Engine mock"
    exit 0
    ;;
  ps)
    if [ "$MOCK_DOCKER_PS_FAIL" = "1" ]; then
      echo "mock docker ps failure" >&2
      exit 1
    fi
    for arg in "$@"; do
      case "$arg" in
        name=*)
          target="${arg#name=}"
          case " $MOCK_DOCKER_RUNNING_CONTAINERS " in
            *" $target "*)
              echo "$target"
              exit 0
              ;;
          esac
          ;;
        publish=*)
          port="${arg#publish=}"
          case " $MOCK_DOCKER_PUBLISHED_PORTS " in
            *" $port "*)
              echo "mock-id-$port"
              exit 0
              ;;
          esac
          ;;
      esac
    done
    exit 0
    ;;
  run)
    if [ "$MOCK_DOCKER_RUN_FAIL" = "1" ]; then
      echo "mock docker run failed to bind port" >&2
      exit 1
    fi
    echo "mock-container-id-cycle230"
    exit 0
    ;;
  stop)
    if [ "$MOCK_DOCKER_STOP_FAIL" = "1" ]; then
      echo "mock docker stop error" >&2
      exit 1
    fi
    echo "$2"
    exit 0
    ;;
  rm)
    if [ "$MOCK_DOCKER_RM_FAIL" = "1" ]; then
      echo "mock docker rm error" >&2
      exit 1
    fi
    echo "$2"
    exit 0
    ;;
  *)
    exit 0
    ;;
esac
`
	err := fileutil.WriteExecutableFile(dockerPath, []byte(script))
	require.NoError(t, err)
	t.Setenv("PATH", binDir+":"+os.Getenv("PATH"))
}

func TestCycle230_BuildDockerRunArgs(t *testing.T) {
	t.Run("empty ports and environment", func(t *testing.T) {
		args := BuildDockerRunArgs("test-box", "alpine:latest", nil, nil)
		expected := []string{"run", "-d", "--name", "test-box", "alpine:latest"}
		assert.Equal(t, expected, args)
	})

	t.Run("ports and environment and extra args", func(t *testing.T) {
		ports := map[string]string{"8080": "80"}
		env := map[string]string{"ENV_VAR": "value1"}
		extra := []string{"--restart", "always"}
		args := BuildDockerRunArgs("custom-box", "redis:7", ports, env, extra...)
		assert.Contains(t, args, "run")
		assert.Contains(t, args, "-d")
		assert.Contains(t, args, "--name")
		assert.Contains(t, args, "custom-box")
		assert.Contains(t, args, "-p")
		assert.Contains(t, args, "8080:80")
		assert.Contains(t, args, "-e")
		assert.Contains(t, args, "ENV_VAR=value1")
		assert.Contains(t, args, "--restart")
		assert.Contains(t, args, "always")
		assert.Equal(t, "redis:7", args[len(args)-1])
	})
}

func TestCycle230_ServiceManager_VerboseLogging(t *testing.T) {
	t.Run("verbose enabled log output", func(t *testing.T) {
		sm := &ServiceManager{verbose: true}
		sm.log("lifecycle status %s code %d", "operational", 200)
		assert.True(t, sm.verbose)
	})

	t.Run("verbose disabled no-op", func(t *testing.T) {
		sm := &ServiceManager{verbose: false}
		sm.log("suppressed message %s", "quiet")
		assert.False(t, sm.verbose)
	})
}

func TestCycle230_LockStartService_Concurrency(t *testing.T) {
	const workers = 10
	var wg sync.WaitGroup
	results := make([]*sync.Mutex, workers)

	for i := 0; i < workers; i++ {
		workerID := i
		wg.Add(1)
		builder := goroutinelabels.NewGoroutine("testservices_lock_worker", fmt.Sprintf("worker-%d", workerID))
		builder.StartSimple(func() {
			defer wg.Done()
			results[workerID] = lockStartService("shared-test-container")
		})
	}
	wg.Wait()

	baseMutex := results[0]
	require.NotNil(t, baseMutex)
	for i := 1; i < workers; i++ {
		assert.Same(t, baseMutex, results[i], "all workers should receive the same mutex instance")
	}

	distinctMutex := lockStartService("different-test-container")
	assert.NotSame(t, baseMutex, distinctMutex, "different container names must yield distinct mutex instances")
}

func TestCycle230_IsDockerAvailable_MockControl(t *testing.T) {
	setupMockDockerEnvironment(t)
	sm := NewServiceManager()

	t.Run("docker info succeeds", func(t *testing.T) {
		t.Setenv("MOCK_DOCKER_INFO_FAIL", "0")
		assert.True(t, IsDockerAvailable())
		assert.True(t, sm.isDockerAvailable())
	})

	t.Run("docker info fails", func(t *testing.T) {
		t.Setenv("MOCK_DOCKER_INFO_FAIL", "1")
		assert.False(t, IsDockerAvailable())
		assert.False(t, sm.isDockerAvailable())
	})
}

func TestCycle230_IsDockerContainerRunning_MockControl(t *testing.T) {
	setupMockDockerEnvironment(t)
	sm := NewServiceManager()

	t.Run("container is running", func(t *testing.T) {
		t.Setenv("MOCK_DOCKER_RUNNING_CONTAINERS", " cycle230-active-box another-box ")
		assert.True(t, IsDockerContainerRunning("cycle230-active-box"))
		assert.True(t, sm.IsServiceRunning("cycle230-active-box"))
	})

	t.Run("container is not running", func(t *testing.T) {
		t.Setenv("MOCK_DOCKER_RUNNING_CONTAINERS", " other-box ")
		assert.False(t, IsDockerContainerRunning("cycle230-active-box"))
		assert.False(t, sm.IsServiceRunning("cycle230-active-box"))
	})

	t.Run("docker ps command failure", func(t *testing.T) {
		t.Setenv("MOCK_DOCKER_PS_FAIL", "1")
		assert.False(t, IsDockerContainerRunning("cycle230-active-box"))
	})
}

func TestCycle230_HostPortsAlreadyPublished_MockControl(t *testing.T) {
	setupMockDockerEnvironment(t)
	sm := NewServiceManager()

	t.Run("empty ports configuration", func(t *testing.T) {
		cfg := ServiceConfig{Ports: nil}
		assert.False(t, sm.hostPortsAlreadyPublished(cfg))
	})

	t.Run("all ports published", func(t *testing.T) {
		t.Setenv("MOCK_DOCKER_PUBLISHED_PORTS", " 7687 7444 ")
		cfg := ServiceConfig{
			Ports: map[string]string{
				"7687": "7687",
				"7444": "7444",
			},
		}
		assert.True(t, sm.hostPortsAlreadyPublished(cfg))
	})

	t.Run("partial port published", func(t *testing.T) {
		t.Setenv("MOCK_DOCKER_PUBLISHED_PORTS", " 7687 ")
		cfg := ServiceConfig{
			Ports: map[string]string{
				"7687": "7687",
				"7444": "7444",
			},
		}
		assert.False(t, sm.hostPortsAlreadyPublished(cfg))
	})

	t.Run("ps command error", func(t *testing.T) {
		t.Setenv("MOCK_DOCKER_PS_FAIL", "1")
		cfg := ServiceConfig{
			Ports: map[string]string{"7687": "7687"},
		}
		assert.False(t, sm.hostPortsAlreadyPublished(cfg))
	})
}

func TestCycle230_StartService_FailureAndEarlyReturns(t *testing.T) {
	setupMockDockerEnvironment(t)
	sm := NewServiceManager()
	ctx := context.Background()

	t.Run("docker unavailable returns error", func(t *testing.T) {
		t.Setenv("MOCK_DOCKER_INFO_FAIL", "1")
		cfg := ServiceConfig{
			Name:          "unavailable-test",
			ContainerName: "box-unavailable",
		}
		err := sm.StartService(ctx, cfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "docker is not available - required for test services")
	})

	t.Run("service already running returns nil", func(t *testing.T) {
		t.Setenv("MOCK_DOCKER_INFO_FAIL", "0")
		t.Setenv("MOCK_DOCKER_RUNNING_CONTAINERS", " box-already-running ")
		cfg := ServiceConfig{
			Name:          "running-test",
			ContainerName: "box-already-running",
		}
		err := sm.StartService(ctx, cfg)
		assert.NoError(t, err)
	})

	t.Run("host ports already published returns nil", func(t *testing.T) {
		t.Setenv("MOCK_DOCKER_INFO_FAIL", "0")
		t.Setenv("MOCK_DOCKER_RUNNING_CONTAINERS", "")
		t.Setenv("MOCK_DOCKER_PUBLISHED_PORTS", " 9091 ")
		cfg := ServiceConfig{
			Name:          "published-test",
			ContainerName: "box-published",
			Ports:         map[string]string{"9091": "9091"},
		}
		err := sm.StartService(ctx, cfg)
		assert.NoError(t, err)
	})

	t.Run("docker run command invocation failure", func(t *testing.T) {
		t.Setenv("MOCK_DOCKER_INFO_FAIL", "0")
		t.Setenv("MOCK_DOCKER_RUNNING_CONTAINERS", "")
		t.Setenv("MOCK_DOCKER_PUBLISHED_PORTS", "")
		t.Setenv("MOCK_DOCKER_RUN_FAIL", "1")
		cfg := ServiceConfig{
			Name:          "run-fail-test",
			ContainerName: "box-run-fail",
			Image:         "alpine:latest",
		}
		err := sm.StartService(ctx, cfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to start service:")
	})

	t.Run("health check failure triggers teardown cascade", func(t *testing.T) {
		t.Setenv("MOCK_DOCKER_INFO_FAIL", "0")
		t.Setenv("MOCK_DOCKER_RUNNING_CONTAINERS", "")
		t.Setenv("MOCK_DOCKER_PUBLISHED_PORTS", "")
		t.Setenv("MOCK_DOCKER_RUN_FAIL", "0")
		cfg := ServiceConfig{
			Name:          "hc-fail-test",
			ContainerName: "box-hc-fail",
			Image:         "alpine:latest",
			StartTimeout:  40 * time.Millisecond,
			HealthCheck: HealthCheck{
				Type:     "command",
				Command:  []string{"false"},
				Interval: 5 * time.Millisecond,
			},
		}
		err := sm.StartService(ctx, cfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "service failed health check")
	})

	t.Run("successful start with healthy command probe", func(t *testing.T) {
		t.Setenv("MOCK_DOCKER_INFO_FAIL", "0")
		t.Setenv("MOCK_DOCKER_RUNNING_CONTAINERS", "")
		t.Setenv("MOCK_DOCKER_PUBLISHED_PORTS", "")
		t.Setenv("MOCK_DOCKER_RUN_FAIL", "0")
		cfg := ServiceConfig{
			Name:          "success-test",
			ContainerName: "box-success",
			Image:         "alpine:latest",
			StartTimeout:  2 * time.Second,
			HealthCheck: HealthCheck{
				Type:     "command",
				Command:  []string{"true"},
				Interval: 5 * time.Millisecond,
			},
		}
		err := sm.StartService(ctx, cfg)
		assert.NoError(t, err)
	})
}

func TestCycle230_StopService_Branches(t *testing.T) {
	setupMockDockerEnvironment(t)
	sm := NewServiceManager()
	ctx := context.Background()

	t.Run("stop non-running service is no-op", func(t *testing.T) {
		t.Setenv("MOCK_DOCKER_RUNNING_CONTAINERS", "")
		err := sm.StopService(ctx, "non-running-box")
		assert.NoError(t, err)
	})

	t.Run("stop running service failure", func(t *testing.T) {
		t.Setenv("MOCK_DOCKER_RUNNING_CONTAINERS", " running-fail-box ")
		t.Setenv("MOCK_DOCKER_STOP_FAIL", "1")
		err := sm.StopService(ctx, "running-fail-box")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to stop service:")
	})

	t.Run("stop running service success", func(t *testing.T) {
		t.Setenv("MOCK_DOCKER_RUNNING_CONTAINERS", " running-ok-box ")
		t.Setenv("MOCK_DOCKER_STOP_FAIL", "0")
		err := sm.StopService(ctx, "running-ok-box")
		assert.NoError(t, err)
	})
}

func TestCycle230_SetupTestServices_GraphIntegration(t *testing.T) {
	setupMockDockerEnvironment(t)
	ctx := context.Background()

	t.Run("graph enabled start success with port listener", func(t *testing.T) {
		tmpDir := t.TempDir()
		cfgDir := filepath.Join(tmpDir, "config")
		err := fileutil.MkdirAll(cfgDir, paths.DirPerm755)
		require.NoError(t, err)
		configYaml := "storage:\n  graph_enabled: true\n"
		err = fileutil.WriteFile(filepath.Join(cfgDir, "zqk.yaml"), []byte(configYaml), paths.FilePerm644)
		require.NoError(t, err)
		t.Setenv("ZQK_PROJECT_ROOT", tmpDir)

		ln, err := net.Listen("tcp", "127.0.0.1:7687")
		require.NoError(t, err)
		defer func() {
			closeErr := ln.Close()
			assert.NoError(t, closeErr)
		}()

		t.Setenv("MOCK_DOCKER_INFO_FAIL", "0")
		t.Setenv("MOCK_DOCKER_RUNNING_CONTAINERS", "")
		t.Setenv("MOCK_DOCKER_PUBLISHED_PORTS", "")
		t.Setenv("MOCK_DOCKER_RUN_FAIL", "0")

		services, err := SetupTestServices(ctx)
		require.NoError(t, err)
		require.NotNil(t, services)
		assert.Equal(t, "zqk-test-memgraph", services.memgraphContainer)

		cleanupErr := services.Cleanup()
		assert.NoError(t, cleanupErr)
	})

	t.Run("graph enabled start failure", func(t *testing.T) {
		tmpDir := t.TempDir()
		cfgDir := filepath.Join(tmpDir, "config")
		err := fileutil.MkdirAll(cfgDir, paths.DirPerm755)
		require.NoError(t, err)
		configYaml := "storage:\n  graph_enabled: true\n"
		err = fileutil.WriteFile(filepath.Join(cfgDir, "zqk.yaml"), []byte(configYaml), paths.FilePerm644)
		require.NoError(t, err)
		t.Setenv("ZQK_PROJECT_ROOT", tmpDir)

		t.Setenv("MOCK_DOCKER_INFO_FAIL", "0")
		t.Setenv("MOCK_DOCKER_RUN_FAIL", "1")

		services, err := SetupTestServices(ctx)
		require.Error(t, err)
		assert.Nil(t, services)
		assert.Contains(t, err.Error(), "failed to start MemGraph")
	})

	t.Run("cleanup aggregation with stop failure", func(t *testing.T) {
		sm := NewServiceManager()
		ts := &TestServices{
			manager:           sm,
			ctx:               ctx,
			memgraphContainer: "memgraph-clean-err",
		}
		t.Setenv("MOCK_DOCKER_RUNNING_CONTAINERS", " memgraph-clean-err ")
		t.Setenv("MOCK_DOCKER_STOP_FAIL", "1")

		err := ts.Cleanup()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cleanup errors: failed to stop MemGraph:")
	})
}

func TestCycle230_WaitForHealthy_BoundariesAndRetries(t *testing.T) {
	sm := NewServiceManager()

	t.Run("default timeout and interval assignment with immediate cancel", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		cfg := ServiceConfig{
			StartTimeout: 0,
			HealthCheck: HealthCheck{
				Type:     "command",
				Command:  []string{"true"},
				Interval: 0,
			},
		}
		err := sm.waitForHealthy(ctx, cfg)
		require.Error(t, err)
		assert.Equal(t, context.Canceled, err)
	})

	t.Run("error retry loop until timeout", func(t *testing.T) {
		ctx := context.Background()
		cfg := ServiceConfig{
			StartTimeout: 30 * time.Millisecond,
			HealthCheck: HealthCheck{
				Type:     "command",
				Command:  nil, // empty command returns error from checkCommandHealth
				Interval: 5 * time.Millisecond,
			},
		}
		err := sm.waitForHealthy(ctx, cfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "health check timeout after")
	})
}
