package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

// Standard error sentinels for service management operations.
var (
	ErrServiceNotFound      = errors.New("service: service not found")
	ErrServiceAlreadyExists = errors.New("service: service already registered")
	ErrUnsupportedPlatform  = errors.New("service: unsupported platform")
	ErrInvalidSpec          = errors.New("service: invalid service specification")
	ErrOperationTimeout     = errors.New("service: operation timed out")
	ErrServiceNotRunning    = errors.New("service: service is not running")
	ErrServiceAlreadyActive = errors.New("service: service is already running")
)

// ProcessState represents the lifecycle execution state of an OS-level service.
type ProcessState string

const (
	StateRunning ProcessState = "running"
	StateStopped ProcessState = "stopped"
	StateUnknown ProcessState = "unknown"
	StateFailed  ProcessState = "failed"
)

// RestartPolicy indicates how host supervisors should handle unexpected process exits.
type RestartPolicy string

const (
	RestartAlways    RestartPolicy = "always"
	RestartOnFailure RestartPolicy = "on-failure"
	RestartNever     RestartPolicy = "never"
)

// ServiceSpec declares the desired configuration and metadata for an OS-level service.
type ServiceSpec struct {
	// ID is the canonical service name or identifier (e.g. "com.zqk.overseer" or "zqk-overseer").
	ID string `json:"id" yaml:"id"`

	// DisplayName provides a human-readable title for logs and tooling.
	DisplayName string `json:"display_name" yaml:"display_name"`

	// Description explains the daemon's function.
	Description string `json:"description" yaml:"description"`

	// Executable is the absolute path to the binary to execute.
	Executable string `json:"executable" yaml:"executable"`

	// Arguments are the command line arguments passed to Executable.
	Arguments []string `json:"arguments,omitempty" yaml:"arguments,omitempty"`

	// WorkingDir is the working directory for process execution.
	WorkingDir string `json:"working_dir,omitempty" yaml:"working_dir,omitempty"`

	// Environment specifies environment variables (KEY=VALUE).
	Environment map[string]string `json:"environment,omitempty" yaml:"environment,omitempty"`

	// StandardOutPath configures where stdout is redirected.
	StandardOutPath string `json:"stdout_path,omitempty" yaml:"stdout_path,omitempty"`

	// StandardErrorPath configures where stderr is redirected.
	StandardErrorPath string `json:"stderr_path,omitempty" yaml:"stderr_path,omitempty"`

	// RunAtLoad specifies if the service starts automatically when registered/booted.
	RunAtLoad bool `json:"run_at_load" yaml:"run_at_load"`

	// KeepAlive configures supervisor restart policy.
	KeepAlive bool `json:"keep_alive" yaml:"keep_alive"`

	// RestartPolicy provides fine-grained control over supervisor restarts.
	RestartPolicy RestartPolicy `json:"restart_policy,omitempty" yaml:"restart_policy,omitempty"`

	// PreExecCommands are optional prerequisite commands executed prior to starting.
	PreExecCommands [][]string `json:"pre_exec_commands,omitempty" yaml:"pre_exec_commands,omitempty"`
}

// Validate verifies that required invariants of the ServiceSpec are satisfied.
func (s ServiceSpec) Validate() error {
	if strings.TrimSpace(s.ID) == "" {
		return fmt.Errorf("%w: missing required service ID", ErrInvalidSpec)
	}
	if strings.TrimSpace(s.Executable) == "" {
		return fmt.Errorf("%w: missing required executable path", ErrInvalidSpec)
	}
	return nil
}

// ServiceStatus reports the live operational state of a managed service.
type ServiceStatus struct {
	ID        string        `json:"id"`
	PID       int           `json:"pid"`
	State     ProcessState  `json:"state"`
	ExitCode  int           `json:"exit_code,omitempty"`
	Uptime    time.Duration `json:"uptime,omitempty"`
	LastError string        `json:"last_error,omitempty"`
}

// ServiceAdapter represents the pluggable host/vendor OS contract for service management.
// It establishes a zero-coupling interface and DTO specification with zero internal
// kernel dependencies, allowing seamless extraction and multi-platform host adaptation.
// Implementations exist for Darwin (launchd), Linux (systemd), and process supervisors.
type ServiceAdapter interface {
	// Name returns the identifier of this adapter (e.g., "launchd", "systemd", "supervisor").
	Name() string

	// IsAvailable returns true if this adapter can operate on the current host.
	IsAvailable() bool

	// Install creates and registers the service unit/descriptor on the host OS.
	Install(ctx context.Context, spec ServiceSpec) error

	// Uninstall unregisters and removes the service unit/descriptor from the host OS.
	Uninstall(ctx context.Context, id string) error

	// Start requests the host supervisor to start the registered service.
	Start(ctx context.Context, id string) error

	// Stop requests the host supervisor to stop the running service.
	Stop(ctx context.Context, id string) error

	// Restart stops and restarts the service.
	Restart(ctx context.Context, id string) error

	// Status queries the host supervisor for current execution status.
	Status(ctx context.Context, id string) (ServiceStatus, error)

	// CleanupLegacy detects and removes superseded service units matching legacy prefixes.
	CleanupLegacy(ctx context.Context, legacyIDs []string) ([]string, error)
}

func readServiceDirEntries(dir string) ([]os.DirEntry, error) {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return nil, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read dir %s: %w", dir, err)
	}
	return entries, nil
}
