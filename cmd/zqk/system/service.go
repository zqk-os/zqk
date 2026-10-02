package system

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/execwrap"
	"github.com/zqk-os/zqk/pkg/zqkenv"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/brand"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/service"
	"github.com/zqk-os/zqk/pkg/storage"
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

func buildServiceSubcmd(bldr *cobra.Command, cmd *cobra.Command, helpBuilder *clipkg.HelpBuilder) *cobra.Command {
	c := clipkg.ApplyBuilder(bldr, cmd)
	helpBuilder.ApplyToCommand(c)
	cli.AddCommonFlags(c)
	return c
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

	return buildServiceSubcmd(bldr_cli_cmd_v1.NewSystemStartCommandBuilder(), &cobra.Command{
		Use:  "start <service>",
		Args: cobra.ExactArgs(1),
		RunE: runServiceStart,
	}, helpBuilder)
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

	return buildServiceSubcmd(bldr_cli_cmd_v1.NewSystemStopCommandBuilder(), &cobra.Command{
		Use:  "stop <service>",
		Args: cobra.ExactArgs(1),
		RunE: runServiceStop,
	}, helpBuilder)
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

	return buildServiceSubcmd(bldr_cli_cmd_v1.NewSystemStatusCommandBuilder(), &cobra.Command{
		Use:  "status <service>",
		Args: cobra.ExactArgs(1),
		RunE: runServiceStatus,
	}, helpBuilder)
}

// NewServiceListCmd creates a command to list available services
func NewServiceListCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"List available services",
		"List all available services that can be managed.",
	).
		AddExample("List all available services", "%s system service list").
		ExcludeCommonFlags()

	return buildServiceSubcmd(bldr_cli_cmd_v1.NewSystemListCommandBuilder(), &cobra.Command{
		Use:  "list",
		Args: cobra.NoArgs,
		RunE: runServiceList,
	}, helpBuilder)
}

var activeServiceManagerFactory = func() *ServiceManager {
	return NewServiceManager()
}

// SetDefaultServiceManagerFactory allows tests to inject custom ServiceManager configurations.
func SetDefaultServiceManagerFactory(f func() *ServiceManager) {
	if f == nil {
		activeServiceManagerFactory = func() *ServiceManager {
			return NewServiceManager()
		}
		return
	}
	activeServiceManagerFactory = f
}

func getActiveServiceManager() *ServiceManager {
	return activeServiceManagerFactory()
}

// ServiceManager handles service operations
type ServiceManager struct {
	logger      logging.Logger
	hostManager *service.Manager
}

var newHostServiceManager = service.NewManager

// NewServiceManager creates a new service manager with auto-detected or provided host service adapters
func NewServiceManager(opts ...service.ManagerOption) *ServiceManager {
	return &ServiceManager{
		logger:      logging.GetLoggerFromProfile(systemProfileHuman),
		hostManager: newHostServiceManager(opts...),
	}
}

// HostManager returns the underlying decoupled host service manager.
func (sm *ServiceManager) HostManager() *service.Manager {
	if sm.hostManager == nil {
		sm.hostManager = newHostServiceManager()
	}
	return sm.hostManager
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
func (sm *ServiceManager) Start(ctx context.Context, serviceName string, auth *ServiceAuth) error {
	// First: if hostManager has a registered adapter that is a mock or handles the service
	if sm.hostManager != nil {
		adapter := sm.hostManager.Adapter()
		if adapter != nil {
			if _, isMock := adapter.(*service.MockAdapter); isMock {
				return sm.hostManager.Start(ctx, serviceName)
			}
		}
	}

	config, err := sm.getServiceConfig(serviceName)
	if err != nil {
		if sm.hostManager != nil {
			return sm.hostManager.Start(ctx, serviceName)
		}
		return err
	}

	// Check if Docker is available
	if !sm.isDockerAvailable() {
		if sm.hostManager != nil {
			if st, err := sm.hostManager.Status(ctx, serviceName); err == nil && st.State == service.StateRunning {
				return nil
			}
			if err := sm.hostManager.Start(ctx, serviceName); err == nil {
				return nil
			}
		}
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
	cmd := execwrap.Command("docker", args...)
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
func (sm *ServiceManager) Stop(ctx context.Context, serviceName string) error {
	if sm.hostManager != nil {
		adapter := sm.hostManager.Adapter()
		if adapter != nil {
			if _, isMock := adapter.(*service.MockAdapter); isMock {
				return sm.hostManager.Stop(ctx, serviceName)
			}
		}
	}

	config, err := sm.getServiceConfig(serviceName)
	if err != nil {
		if sm.hostManager != nil {
			return sm.hostManager.Stop(ctx, serviceName)
		}
		return err
	}

	// Check if service is running
	if !sm.isServiceRunning(config.ContainerName) {
		if sm.hostManager != nil {
			if err := sm.hostManager.Stop(ctx, serviceName); err == nil {
				return nil
			}
		}
		logging.Fluent(sm.logger).Info("Service not running").
			String("service", serviceName).
			Log()
		return nil
	}

	// Stop container
	//nolint:gosec // G204: Docker command with fixed arguments - container name is validated
	cmd := execwrap.Command("docker", "stop", config.ContainerName)
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
func (sm *ServiceManager) Status(ctx context.Context, serviceName string) (bool, error) {
	if sm.hostManager != nil {
		adapter := sm.hostManager.Adapter()
		if adapter != nil {
			if _, isMock := adapter.(*service.MockAdapter); isMock {
				st, err := sm.hostManager.Status(ctx, serviceName)
				if err != nil {
					return false, err
				}
				return st.State == service.StateRunning, nil
			}
		}
	}

	config, err := sm.getServiceConfig(serviceName)
	if err != nil {
		if sm.hostManager != nil {
			st, statusErr := sm.hostManager.Status(ctx, serviceName)
			if statusErr == nil {
				return st.State == service.StateRunning, nil
			}
		}
		return false, err
	}

	if sm.isServiceRunning(config.ContainerName) {
		return true, nil
	}

	if sm.hostManager != nil {
		st, statusErr := sm.hostManager.Status(ctx, serviceName)
		if statusErr == nil && st.State == service.StateRunning {
			return true, nil
		}
	}

	return false, nil
}

// isDockerAvailable checks if Docker is available
func (sm *ServiceManager) isDockerAvailable() bool {
	cmd := execwrap.Command("docker", "info")
	err := cmd.Run()
	return err == nil
}

// isServiceRunning checks if a service container is running
func (sm *ServiceManager) isServiceRunning(containerName string) bool {
	//nolint:gosec // G204: Docker command with fixed arguments - container name is validated
	cmd := execwrap.Command("docker", "ps", "--filter", fmt.Sprintf("name=%s", containerName), "--format", "{{.Names}}")
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
		graphUsername := zqkenv.GraphUsername().Get()
		graphPassword := zqkenv.GraphPassword().Get()
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

type serviceCoordinationContext struct {
	projectRoot     string
	storageProvider storage.ObjectStorageProvider
	profile         string
	cleanup         func()
}

func resolveServiceCoordinationContext(cmd *cobra.Command) *serviceCoordinationContext {
	ctx, sp, _, _, cleanup, err := openSystemStorageWithContext(cmd)
	projectRoot := ""
	profile := systemProfileHuman
	if ctx != nil {
		projectRoot = ProjectRootOrResolve(ctx.ProjectRoot)
		if ctx.Profile != emptyValue {
			profile = ctx.Profile
		}
	}
	if err != nil {
		return &serviceCoordinationContext{
			projectRoot: projectRoot,
			profile:     profile,
			cleanup:     func() {},
		}
	}
	return &serviceCoordinationContext{
		projectRoot:     projectRoot,
		storageProvider: sp,
		profile:         profile,
		cleanup:         cleanup,
	}
}

func (s *serviceCoordinationContext) emitEvent(ctx context.Context, op, serviceName, status string, err error, duration time.Duration) {
	if s.projectRoot == emptyValue {
		return
	}
	emitServiceOperationEventViaCoordinator(
		ctx,
		s.projectRoot,
		s.storageProvider,
		op,
		serviceName,
		status,
		err,
		duration,
		s.profile,
	)
}

func resolveServiceInvocation(cmd *cobra.Command, args []string) (string, *logging.EventLogger, *ServiceManager) {
	return args[0], logging.GetLoggerFromContext(cmd.Context()), getActiveServiceManager()
}

func executeServiceLifecycleAction(cmd *cobra.Command, args []string, op, status, verb string, action func(manager *ServiceManager, serviceName string) error) error {
	serviceName, logger, manager := resolveServiceInvocation(cmd, args)

	coord := resolveServiceCoordinationContext(cmd)
	defer coord.cleanup()

	startTime := time.Now()
	err := action(manager, serviceName)
	duration := time.Since(startTime)

	if err != nil {
		logging.FluentEvent(logger).Error(fmt.Sprintf("Failed to %s service", op), err).
			String("service", serviceName).
			Log()

		coord.emitEvent(cmd.Context(), op, serviceName, eventStatusError, err, duration)
		return err
	}

	coord.emitEvent(cmd.Context(), op, serviceName, eventStatusComplete, nil, duration)
	return formatServiceResult(cmd, serviceName, status, verb)
}

// runServiceStart handles the start command
func runServiceStart(cmd *cobra.Command, args []string) error {
	return executeServiceLifecycleAction(cmd, args, "start", objects.ObjectStatusStarted, "started", func(manager *ServiceManager, serviceName string) error {
		config, err := manager.getServiceConfig(serviceName)
		if err != nil {
			return err
		}
		auth := manager.getServiceAuth(serviceName, config)
		return manager.Start(cmd.Context(), serviceName, auth)
	})
}

func formatServiceResult(cmd *cobra.Command, serviceName, status, actionVerb string) error {
	switch cli.GetFormat(cmd) {
	case cli.FormatJSON, cli.FormatJSONL, cli.FormatYAML:
		data := map[string]string{"service": serviceName, objects.FieldKeyStatus: status}
		return cli.FormatOutput(cmd, data)
	default:
		msg := fmt.Sprintf("Service %s %s successfully\n", serviceName, actionVerb)
		return cli.WriteOutput(cmd, []byte(msg))
	}
}

// runServiceStop handles the stop command
func runServiceStop(cmd *cobra.Command, args []string) error {
	return executeServiceLifecycleAction(cmd, args, "stop", objects.ObjectStatusStopped, "stopped", func(manager *ServiceManager, serviceName string) error {
		return manager.Stop(cmd.Context(), serviceName)
	})
}

// runServiceStatus handles the status command
func runServiceStatus(cmd *cobra.Command, args []string) error {
	serviceName, logger, manager := resolveServiceInvocation(cmd, args)
	running, err := manager.Status(cmd.Context(), serviceName)
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
	manager := getActiveServiceManager()
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
