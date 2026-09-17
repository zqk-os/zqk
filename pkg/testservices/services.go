// Package testservices manages optional test-side services (e.g. MemGraph via Docker)
// without pulling in the full pkg/testing surface.
package testservices

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/lanceman/zqk/pkg/config"
	"github.com/lanceman/zqk/pkg/execwrap"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
)

// startServiceLocks serializes docker run/rm per container name across parallel tests
// (each SetupTestServices builds a new ServiceManager).
var startServiceLocks sync.Map // containerName -> *sync.Mutex

func lockStartService(containerName string) *sync.Mutex {
	v, _ := startServiceLocks.LoadOrStore(containerName, &sync.Mutex{})
	return v.(*sync.Mutex)
}

// ServiceManager manages test services (e.g., MemGraph)
type ServiceManager struct {
	verbose bool
}

// NewServiceManager creates a new test service manager
func NewServiceManager() *ServiceManager {
	return &ServiceManager{
		verbose: config.TestingVerbose().OrDefault(false),
	}
}

// log prints a message if verbose mode is enabled
// Routes through logging framework to ensure proper output routing
func (sm *ServiceManager) log(format string, args ...any) {
	if sm.verbose {
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		msg := fmt.Sprintf("[SERVICE] "+format, args...)
		logging.Fluent(logger).Info(msg).Log()
	}
}

// ServiceConfig holds configuration for a test service
type ServiceConfig struct {
	Name          string
	ContainerName string
	Image         string
	Ports         map[string]string // host:container
	Environment   map[string]string
	HealthCheck   HealthCheck
	StartTimeout  time.Duration
	StopTimeout   time.Duration
}

// HealthCheck defines how to check if a service is healthy
type HealthCheck struct {
	Type     string // "port", "http", "command"
	Port     string
	Path     string // for HTTP checks
	Command  []string
	Interval time.Duration
	Timeout  time.Duration
}

// StartService starts a service and waits for it to be healthy
//
//nolint:gocritic // Config passed by value for immutability
func (sm *ServiceManager) StartService(ctx context.Context, config ServiceConfig) error {
	// Check if Docker is available
	if !sm.isDockerAvailable() {
		return errfmt.Errorf("docker is not available - required for test services")
	}

	mu := lockStartService(config.ContainerName)
	mu.Lock()
	defer mu.Unlock()

	// Check if service is already running
	if sm.isServiceRunning(config.ContainerName) {
		sm.log("Service %s already running", config.Name)
		return nil
	}

	// Studio often runs a long-lived MemGraph on the same host ports (e.g. zqk-memgraph).
	// Reuse that listener instead of failing docker run with "port is already allocated".
	if sm.hostPortsAlreadyPublished(config) {
		sm.log("Service %s host ports already published; reusing existing listener", config.Name)
		return nil
	}

	// Clear stale same-name containers (Created/Exited) that block --name reuse.
	_ = execwrap.CommandContext(ctx, "docker", "rm", "-f", config.ContainerName).Run() //nolint:gosec // test container name from config

	// Build docker run command
	args := []string{"run", "-d", "--name", config.ContainerName}

	// Add ports
	for hostPort, containerPort := range config.Ports {
		args = append(args, "-p", fmt.Sprintf("%s:%s", hostPort, containerPort))
	}

	// Add environment variables
	for key, value := range config.Environment {
		args = append(args, "-e", fmt.Sprintf("%s=%s", key, value))
	}

	// Add image
	args = append(args, config.Image)

	// Start container
	sm.log("Starting test service: %s", config.Name)
	cmd := execwrap.CommandContext(ctx, "docker", args...) //nolint:gosec
	output, err := cmd.CombinedOutput()
	if err != nil {
		return errfmt.Errorf("failed to start service: %w\nOutput: %s", err, string(output))
	}

	// Wait for service to be healthy
	sm.log("Waiting for service %s to be healthy", config.Name)
	if err := sm.waitForHealthy(ctx, config); err != nil {
		// Clean up on failure
		//nolint:errcheck // Service cleanup - error acceptable
		_ = sm.StopService(ctx, config.ContainerName)
		return errfmt.Newf("service failed health check").Wrap(err)
	}

	sm.log("Service %s started and healthy", config.Name)
	return nil
}

// StopService stops a service
func (sm *ServiceManager) StopService(ctx context.Context, containerName string) error {
	if !sm.isServiceRunning(containerName) {
		return nil // Already stopped
	}

	sm.log("Stopping test service: %s", containerName)
	cmd := execwrap.CommandContext(ctx, "docker", "stop", containerName) //nolint:gosec
	output, err := cmd.CombinedOutput()
	if err != nil {
		return errfmt.Errorf("failed to stop service: %w\nOutput: %s", err, string(output))
	}

	// Remove container
	sm.log("Removing test service container: %s", containerName)
	cmd = execwrap.CommandContext(ctx, "docker", "rm", containerName)
	//nolint:errcheck // Intentional error ignored
	_ = cmd.Run() // Ignore error if container already removed

	return nil
}

// IsServiceRunning checks if a service is running
func (sm *ServiceManager) IsServiceRunning(containerName string) bool {
	return sm.isServiceRunning(containerName)
}

// waitForHealthy waits for a service to pass its health check
//
//nolint:gocritic // Config passed by value for immutability
func (sm *ServiceManager) waitForHealthy(ctx context.Context, config ServiceConfig) error {
	timeout := config.StartTimeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	deadline := time.Now().Add(timeout)
	interval := config.HealthCheck.Interval
	if interval == 0 {
		interval = 1 * time.Second
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if time.Now().After(deadline) {
				return errfmt.Errorf("health check timeout after %v", timeout)
			}

			healthy, err := sm.checkHealth(ctx, config)
			if err != nil {
				continue // Retry on error
			}
			if healthy {
				return nil
			}
		}
	}
}

// checkHealth performs a health check based on the configured type
//
//nolint:gocritic // Config passed by value for immutability
func (sm *ServiceManager) checkHealth(ctx context.Context, config ServiceConfig) (bool, error) {
	switch config.HealthCheck.Type {
	case "port":
		return sm.checkPortHealth(ctx, config.HealthCheck.Port)
	case "http":
		return sm.checkHTTPHealth(ctx, config.HealthCheck.Port, config.HealthCheck.Path)
	case "command":
		return sm.checkCommandHealth(ctx, config.HealthCheck.Command)
	default:
		// Default: just check if container is running
		return sm.isServiceRunning(config.ContainerName), nil
	}
}

// checkPortHealth checks if a port is listening
func (sm *ServiceManager) checkPortHealth(ctx context.Context, port string) (bool, error) {
	cmd := exec.CommandContext(ctx, "nc", "-z", "localhost", port) //nolint:gosec
	err := cmd.Run()
	return err == nil, nil
}

// checkHTTPHealth checks if an HTTP endpoint responds
func (sm *ServiceManager) checkHTTPHealth(ctx context.Context, port, path string) (bool, error) {
	url := fmt.Sprintf("http://localhost:%s%s", port, path)
	cmd := exec.CommandContext(ctx, "curl", "-f", "-s", url) //nolint:gosec
	err := cmd.Run()
	return err == nil, nil
}

// checkCommandHealth runs a command to check health
func (sm *ServiceManager) checkCommandHealth(ctx context.Context, command []string) (bool, error) {
	if len(command) == 0 {
		return false, errfmt.Errorf("health check command is empty")
	}

	//nolint:gosec // G204: Command execution is intentional - this is a test helper that executes test commands
	cmd := exec.CommandContext(ctx, command[0], command[1:]...)
	err := cmd.Run()
	return err == nil, nil
}

// isDockerAvailable checks if Docker is available
func (sm *ServiceManager) isDockerAvailable() bool {
	cmd := exec.Command("docker", "info")
	err := cmd.Run()
	return err == nil
}

// isServiceRunning checks if a service container is running
func (sm *ServiceManager) isServiceRunning(containerName string) bool {
	cmd := exec.Command("docker", "ps", "--filter", fmt.Sprintf("name=%s", containerName), "--format", "{{.Names}}") //nolint:gosec // Docker command with sanitized container name
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(output)) == containerName
}

// hostPortsAlreadyPublished reports whether every host port in config is already
// published by some running container (so a new bind would fail).
func (sm *ServiceManager) hostPortsAlreadyPublished(config ServiceConfig) bool {
	if len(config.Ports) == 0 {
		return false
	}
	for hostPort := range config.Ports {
		cmd := exec.Command("docker", "ps", "--filter", "publish="+hostPort, "--format", "{{.ID}}") //nolint:gosec // host port from test config
		output, err := cmd.Output()
		if err != nil || strings.TrimSpace(string(output)) == "" {
			return false
		}
	}
	return true
}

// GetMemGraphConfig returns configuration for MemGraph test service
func GetMemGraphConfig() ServiceConfig {
	return ServiceConfig{
		Name:          "memgraph",
		ContainerName: "zqk-test-memgraph",
		Image:         "memgraph/memgraph",
		Ports: map[string]string{
			"7687": "7687", // Bolt port
			"7444": "7444", // HTTP port
		},
		Environment: make(map[string]string),
		HealthCheck: HealthCheck{
			Type:     "port",
			Port:     "7687",
			Interval: 1 * time.Second,
			Timeout:  30 * time.Second,
		},
		StartTimeout: 30 * time.Second,
		StopTimeout:  10 * time.Second,
	}
}

// SetupTestServices sets up all required test services
func SetupTestServices(ctx context.Context) (*TestServices, error) {
	manager := NewServiceManager()
	services := &TestServices{
		manager: manager,
		ctx:     ctx,
	}

	// Start MemGraph if graph backend is enabled
	if config.StorageGraphEnabled().OrDefault(false) {
		memgraphConfig := GetMemGraphConfig()
		if err := manager.StartService(ctx, memgraphConfig); err != nil {
			return nil, errfmt.Newf("failed to start MemGraph").Wrap(err)
		}
		services.memgraphContainer = memgraphConfig.ContainerName
	}

	return services, nil
}

// TestServices holds references to running test services
type TestServices struct {
	manager           *ServiceManager
	ctx               context.Context
	memgraphContainer string
}

// Cleanup stops all test services
func (ts *TestServices) Cleanup() error {
	var errors []string

	if ts.memgraphContainer != "" {
		if err := ts.manager.StopService(ts.ctx, ts.memgraphContainer); err != nil {
			errors = append(errors, fmt.Sprintf("failed to stop MemGraph: %v", err))
		}
	}

	if len(errors) > 0 {
		return errfmt.Errorf("cleanup errors: %s", strings.Join(errors, "; "))
	}

	return nil
}
