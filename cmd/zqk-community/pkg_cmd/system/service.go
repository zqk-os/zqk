package system

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/lanceman/zqk/internal/cli"
	"github.com/lanceman/zqk/pkg/brand"
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/lanceman/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/zqkenv"
	"github.com/spf13/cobra"
)

// NewServiceCmd creates a command for managing services (e.g., MemGraph)
func NewServiceCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Manage external services (e.g., MemGraph)",
		"Manage external services required by ZQK.",
		"",
		"This command allows you to start, stop, and check the status of services",
		"like MemGraph that are used by the graph backend.",
	).
		AddExample("Start MemGraph service", "%s system service start memgraph").
		AddExample("Stop MemGraph service", "%s system service stop memgraph").
		AddExample("Check service status", "%s system service status memgraph").
		AddExample("List all available services", "%s system service list").
		ExcludeCommonFlags()

	serviceCmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemServiceCommandBuilder(

	// Apply help builder to command
	), &cobra.Command{
		Use: "service",
	})

	helpBuilder.ApplyToCommand(serviceCmd)

	serviceCmd.AddCommand(NewServiceStartCmd())
	serviceCmd.AddCommand(NewServiceStopCmd())
	serviceCmd.AddCommand(NewServiceStatusCmd())
	serviceCmd.AddCommand(NewServiceListCmd())

	cli.AddCommonFlags(serviceCmd)
	return serviceCmd
}

// NewServiceStartCmd creates a command to start a service
func NewServiceStartCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Start a service",
		"Start a service required by ZQK.",
		"",
		"Available services:",
		"  - memgraph: Start MemGraph database for graph backend",
	).
		AddExample("Start MemGraph service", "%s system service start memgraph").
		ExcludeCommonFlags()

	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemStartCommandBuilder(), &cobra.Command{
		Use:  "start <service>",
		Args: cobra.ExactArgs(1),
		RunE: runServiceStart,
	})

	// Apply help builder to command
	helpBuilder.ApplyToCommand(cmd)

	cli.AddCommonFlags(cmd)
	return cmd
}

// NewServiceStopCmd creates a command to stop a service
func NewServiceStopCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Stop a service",
		"Stop a running service.",
		"",
		"Available services:",
		"  - memgraph: Stop MemGraph database",
	).
		AddExample("Stop MemGraph service", "%s system service stop memgraph").
		ExcludeCommonFlags()

	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemStopCommandBuilder(), &cobra.Command{
		Use:  "stop <service>",
		Args: cobra.ExactArgs(1),
		RunE: runServiceStop,
	})

	// Apply help builder to command
	helpBuilder.ApplyToCommand(cmd)

	cli.AddCommonFlags(cmd)
	return cmd
}

// NewServiceStatusCmd creates a command to check service status
func NewServiceStatusCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Check service status",
		"Check the status of a service.",
		"",
		"Available services:",
		"  - memgraph: Check MemGraph database status",
	).
		AddExample("Check MemGraph service status", "%s system service status memgraph").
		ExcludeCommonFlags()

	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemStatusCommandBuilder(), &cobra.Command{
		Use:  "status <service>",
		Args: cobra.ExactArgs(1),
		RunE: runServiceStatus,
	})

	// Apply help builder to command
	helpBuilder.ApplyToCommand(cmd)

	cli.AddCommonFlags(cmd)
	return cmd
}

// NewServiceListCmd creates a command to list available services
func NewServiceListCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"List available services",
		"List all available services that can be managed.",
	).
		AddExample("List all available services", "%s system service list").
		ExcludeCommonFlags()

	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemListCommandBuilder(), &cobra.Command{
		Use:  "list",
		Args: cobra.NoArgs,
		RunE: runServiceList,
	})

	// Apply help builder to command
	helpBuilder.ApplyToCommand(cmd)

	cli.AddCommonFlags(cmd)
	return cmd
}

// ServiceManager handles service operations
type ServiceManager struct {
	logger logging.Logger
}

// NewServiceManager creates a new service manager
func NewServiceManager() *ServiceManager {
	return &ServiceManager{
		logger: logging.GetLoggerFromProfile(systemProfileHuman),
	}
}

// ServiceConfig holds configuration for a service
type ServiceConfig struct {
	Name          string
	ContainerName string
	Image         string
	Ports         map[string]string // host:container
	Environment   map[string]string
	Volumes       map[string]string // host:container
	RequiresAuth  bool
	DefaultAuth   *ServiceAuth
}

// ServiceAuth holds authentication information
type ServiceAuth struct {
	Username string
	Password string
}

// getServiceConfig returns configuration for a service
func (sm *ServiceManager) getServiceConfig(serviceName string) (*ServiceConfig, error) {
	switch strings.ToLower(serviceName) {
	case "memgraph":
		return &ServiceConfig{
			Name:          "memgraph",
			ContainerName: "zqk-memgraph",
			Image:         "memgraph/memgraph",
			Ports: map[string]string{
				"7687": "7687", // Bolt port
				"7444": "7444", // HTTP port
			},
			Environment:  make(map[string]string),
			Volumes:      make(map[string]string),
			RequiresAuth: false, // MemGraph doesn't require auth by default
			DefaultAuth:  nil,
		}, nil
	default:
		return nil, errfmt.Errorf("unknown service: %s", serviceName)
	}
}

// Start starts a service
func (sm *ServiceManager) Start(serviceName string, auth *ServiceAuth) error {
	config, err := sm.getServiceConfig(serviceName)
	if err != nil {
		return err
	}

	// Check if Docker is available
	if !sm.isDockerAvailable() {
		return errfmt.Errorf("docker is not available, please install and start Docker first")
	}

	// Check if service is already running
	if sm.isServiceRunning(config.ContainerName) {
		logging.Fluent(sm.logger).Info("Service already running").
			String("service", serviceName).
			Log()
		return nil
	}

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

	// Add authentication if provided
	if auth != nil {
		if auth.Username != emptyValue {
			args = append(args, "-e", fmt.Sprintf("MEMGRAPH_USERNAME=%s", auth.Username))
		}
		if auth.Password != emptyValue {
			args = append(args, "-e", fmt.Sprintf("MEMGRAPH_PASSWORD=%s", auth.Password))
		}
	}

	// Add volumes
	for hostPath, containerPath := range config.Volumes {
		args = append(args, "-v", fmt.Sprintf("%s:%s", hostPath, containerPath))
	}

	// Add image
	args = append(args, config.Image)

	// Execute docker run
	cmd := exec.Command("docker", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return errfmt.Newf("failed to start service; output: %s", string(output)).Wrap(err)
	}

	logging.Fluent(sm.logger).Info("Service started successfully").
		String("service", serviceName).
		Log()
	return nil
}

// Stop stops a service
func (sm *ServiceManager) Stop(serviceName string) error {
	config, err := sm.getServiceConfig(serviceName)
	if err != nil {
		return err
	}

	// Check if service is running
	if !sm.isServiceRunning(config.ContainerName) {
		logging.Fluent(sm.logger).Info("Service not running").
			String("service", serviceName).
			Log()
		return nil
	}

	// Stop container
	//nolint:gosec // G204: Docker command with fixed arguments - container name is validated
	cmd := exec.Command("docker", "stop", config.ContainerName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return errfmt.Newf("failed to stop service; output: %s", string(output)).Wrap(err)
	}

	logging.Fluent(sm.logger).Info("Service stopped successfully").
		String("service", serviceName).
		Log()
	return nil
}

// Status checks service status
func (sm *ServiceManager) Status(serviceName string) (bool, error) {
	config, err := sm.getServiceConfig(serviceName)
	if err != nil {
		return false, err
	}

	return sm.isServiceRunning(config.ContainerName), nil
}

// isDockerAvailable checks if Docker is available
func (sm *ServiceManager) isDockerAvailable() bool {
	cmd := exec.Command("docker", "info")
	err := cmd.Run()
	return err == nil
}

// isServiceRunning checks if a service container is running
func (sm *ServiceManager) isServiceRunning(containerName string) bool {
	//nolint:gosec // G204: Docker command with fixed arguments - container name is validated
	cmd := exec.Command("docker", "ps", "--filter", fmt.Sprintf("name=%s", containerName), "--format", "{{.Names}}")
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(output)) == containerName
}

// ListServices returns a list of available services
func (sm *ServiceManager) ListServices() []string {
	return []string{"memgraph"}
}

// getServiceAuth retrieves authentication for a service from multiple sources
// Priority: Environment variables > Security context > Service defaults
func (sm *ServiceManager) getServiceAuth(serviceName string, config *ServiceConfig) *ServiceAuth {
	// Check environment variables first (highest priority)
	prefix := brand.EnvPrefix()
	username := os.Getenv(fmt.Sprintf("%s_%s_USERNAME", prefix, strings.ToUpper(serviceName)))
	password := os.Getenv(fmt.Sprintf("%s_%s_PASSWORD", prefix, strings.ToUpper(serviceName)))

	// If environment variables are set, use them
	if username != emptyValue || password != emptyValue {
		return &ServiceAuth{
			Username: username,
			Password: password,
		}
	}

	// Check service-specific environment variables (e.g., ZQK_GRAPH_USERNAME)
	if serviceName == "memgraph" {
		graphUsername := os.Getenv(zqkenv.GraphUsername())
		graphPassword := os.Getenv(zqkenv.GraphPassword())
		if graphUsername != emptyValue || graphPassword != emptyValue {
			return &ServiceAuth{
				Username: graphUsername,
				Password: graphPassword,
			}
		}
	}

	// Use service defaults if configured
	if config.DefaultAuth != nil {
		return config.DefaultAuth
	}

	// No authentication required (default for MemGraph)
	return nil
}

// runServiceStart handles the start command
func runServiceStart(cmd *cobra.Command, args []string) error {
	serviceName := args[0]
	logger := logging.GetLoggerFromContext(cmd.Context())

	manager := NewServiceManager()
	config, err := manager.getServiceConfig(serviceName)
	if err != nil {
		return err
	}

	// Get authentication from multiple sources (in order of precedence):
	// 1. Command flags (if added in future)
	// 2. Environment variables
	// 3. Security context (if available)
	// 4. Service defaults
	auth := manager.getServiceAuth(serviceName, config)

	startTime := time.Now()
	err = manager.Start(serviceName, auth)
	duration := time.Since(startTime)

	// Get project root and storage for coordination events
	ctx := cli.GetContext(cmd)
	projectRoot := ""
	var storageProvider storage.ObjectStorageProvider
	if ctx != nil {
		projectRoot = ctx.ProjectRoot
		projectRoot = ProjectRootOrResolve(projectRoot)
		if projectRoot != emptyValue {
			var storageErr error
			storageProvider, storageErr = storage.NewFileObjectStorage(projectRoot)
			if storageErr == nil {
				defer func() { _ = storageProvider.Shutdown(context.Background()) }()
			}
			if storageErr != nil {
				// Best effort - continue without coordinator if storage unavailable
				storageProvider = nil
			}
		}
	}

	if err != nil {
		logging.FluentEvent(logger).Error("Failed to start service", err).
			String("service", serviceName).
			Log()

		// Emit coordination event for service start failure
		if projectRoot != emptyValue {
			profile := systemProfileHuman // Default
			if ctx != nil && ctx.Profile != emptyValue {
				profile = ctx.Profile
			}
			emitServiceOperationEventViaCoordinator(
				cmd.Context(),
				projectRoot,
				storageProvider,
				"start",
				serviceName,
				eventStatusError,
				err,
				duration,
				profile,
			)
		}

		return err
	}

	// Emit coordination event for successful service start
	if projectRoot != emptyValue {
		profile := systemProfileHuman // Default
		if ctx != nil && ctx.Profile != emptyValue {
			profile = ctx.Profile
		}
		emitServiceOperationEventViaCoordinator(
			cmd.Context(),
			projectRoot,
			storageProvider,
			"start",
			serviceName,
			eventStatusComplete,
			nil,
			duration,
			profile,
		)
	}

	switch cli.GetFormat(cmd) {
	case cli.FormatJSON, cli.FormatJSONL, cli.FormatYAML:
		data := map[string]string{"service": serviceName, objects.FieldKeyStatus: objects.ObjectStatusStarted}
		return cli.FormatOutput(cmd, data)
	default:
		msg := fmt.Sprintf("Service %s started successfully\n", serviceName)
		return cli.WriteOutput(cmd, []byte(msg))
	}
}

// runServiceStop handles the stop command
func runServiceStop(cmd *cobra.Command, args []string) error {
	serviceName := args[0]
	logger := logging.GetLoggerFromContext(cmd.Context())

	manager := NewServiceManager()

	startTime := time.Now()
	err := manager.Stop(serviceName)
	duration := time.Since(startTime)

	// Get project root and storage for coordination events
	ctx := cli.GetContext(cmd)
	projectRoot := ""
	var storageProvider storage.ObjectStorageProvider
	if ctx != nil {
		projectRoot = ctx.ProjectRoot
		projectRoot = ProjectRootOrResolve(projectRoot)
		if projectRoot != emptyValue {
			var storageErr error
			storageProvider, storageErr = storage.NewFileObjectStorage(projectRoot)
			if storageErr == nil {
				defer func() { _ = storageProvider.Shutdown(context.Background()) }()
			}
			if storageErr != nil {
				// Best effort - continue without coordinator if storage unavailable
				storageProvider = nil
			}
		}
	}

	if err != nil {
		logging.FluentEvent(logger).Error("Failed to stop service", err).
			String("service", serviceName).
			Log()

		// Emit coordination event for service stop failure
		if projectRoot != emptyValue {
			profile := systemProfileHuman // Default
			if ctx != nil && ctx.Profile != emptyValue {
				profile = ctx.Profile
			}
			emitServiceOperationEventViaCoordinator(
				cmd.Context(),
				projectRoot,
				storageProvider,
				"stop",
				serviceName,
				eventStatusError,
				err,
				duration,
				profile,
			)
		}

		return err
	}

	// Emit coordination event for successful service stop
	if projectRoot != emptyValue {
		profile := systemProfileHuman // Default
		if ctx != nil && ctx.Profile != emptyValue {
			profile = ctx.Profile
		}
		emitServiceOperationEventViaCoordinator(
			cmd.Context(),
			projectRoot,
			storageProvider,
			"stop",
			serviceName,
			eventStatusComplete,
			nil,
			duration,
			profile,
		)
	}

	switch cli.GetFormat(cmd) {
	case cli.FormatJSON, cli.FormatJSONL, cli.FormatYAML:
		data := map[string]string{"service": serviceName, objects.FieldKeyStatus: objects.ObjectStatusStopped}
		return cli.FormatOutput(cmd, data)
	default:
		msg := fmt.Sprintf("Service %s stopped successfully\n", serviceName)
		return cli.WriteOutput(cmd, []byte(msg))
	}
}

// runServiceStatus handles the status command
func runServiceStatus(cmd *cobra.Command, args []string) error {
	serviceName := args[0]
	logger := logging.GetLoggerFromContext(cmd.Context())

	manager := NewServiceManager()
	running, err := manager.Status(serviceName)
	if err != nil {
		logging.FluentEvent(logger).Error("Failed to check service status", err).
			String("service", serviceName).
			Log()
		return err
	}

	status := "stopped"
	if running {
		status = "running"
	}

	switch cli.GetFormat(cmd) {
	case cli.FormatJSON, cli.FormatJSONL, cli.FormatYAML:
		data := map[string]string{"service": serviceName, objects.FieldKeyStatus: status}
		return cli.FormatOutput(cmd, data)
	default:
		var msg string
		if running {
			msg = fmt.Sprintf("Service %s is running\n", serviceName)
		} else {
			msg = fmt.Sprintf("Service %s is not running\n", serviceName)
		}
		return cli.WriteOutput(cmd, []byte(msg))
	}
}

// runServiceList handles the list command
func runServiceList(cmd *cobra.Command, args []string) error {
	manager := NewServiceManager()
	services := manager.ListServices()

	switch cli.GetFormat(cmd) {
	case cli.FormatJSON, cli.FormatJSONL, cli.FormatYAML:
		data := map[string][]string{"services": services}
		return cli.FormatOutput(cmd, data)
	default:
		var buf bytes.Buffer
		buf.WriteString("Available services:\n")
		for _, svc := range services {
			fmt.Fprintf(&buf, "  • %s\n", svc)
		}
		return cli.WriteOutput(cmd, buf.Bytes())
	}
}
