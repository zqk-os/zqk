package overseer

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/agentfeed"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// DesiredState represents whether an operator/system wants the daemon running.
type DesiredState string

const (
	DesiredStateEnabled  DesiredState = "enabled"
	DesiredStateDisabled DesiredState = "disabled"
)

// ActualState represents the current real-time operating state.
type ActualState string

const (
	ActualStateRunning ActualState = "running"
	ActualStateStopped ActualState = "stopped"
	ActualStateCrashed ActualState = "crashed"
	ActualStateBackoff ActualState = "backoff"
)

// RestartPolicy governs auto-restart conditions.
type RestartPolicy string

const (
	RestartPolicyAlways    RestartPolicy = "always"
	RestartPolicyOnFailure RestartPolicy = "on-failure"
	RestartPolicyNever     RestartPolicy = "never"
)

// DaemonSpec defines the persistent configuration of a managed daemon.
type DaemonSpec struct {
	Name          string            `json:"name"`
	Description   string            `json:"description"`
	Command       []string          `json:"command"`
	DesiredState  DesiredState      `json:"desired_state"`
	RestartPolicy RestartPolicy     `json:"restart_policy"`
	MaxRestarts   int               `json:"max_restarts"`
	BackoffMin    time.Duration     `json:"backoff_min"`
	BackoffMax    time.Duration     `json:"backoff_max"`
	Env           map[string]string `json:"env,omitempty"`
	WorkingDir    string            `json:"working_dir,omitempty"`
}

// DaemonStatus captures real-time runtime state.
type DaemonStatus struct {
	Name         string       `json:"name"`
	DesiredState DesiredState `json:"desired_state"`
	ActualState  ActualState  `json:"actual_state"`
	PID          int          `json:"pid"`
	PGID         int          `json:"pgid"`
	RestartCount int          `json:"restart_count"`
	StartedAt    time.Time    `json:"started_at,omitempty"`
	ExitedAt     time.Time    `json:"exited_at,omitempty"`
	LastError    string       `json:"last_error,omitempty"`
	BackoffUntil time.Time    `json:"backoff_until,omitempty"`
}

// DefaultDaemonSpecs returns the built-in system daemon specifications.
func DefaultDaemonSpecs() []*DaemonSpec {
	return []*DaemonSpec{
		{
			Name:          "scheduler",
			Description:   "ZQK Kernel Task & Event Scheduler Daemon",
			Command:       []string{"scheduler", "start", "--foreground"},
			DesiredState:  DesiredStateEnabled,
			RestartPolicy: RestartPolicyAlways,
			MaxRestarts:   10,
			BackoffMin:    500 * time.Millisecond,
			BackoffMax:    30 * time.Second,
		},
		{
			Name:          "ambient",
			Description:   "ZQK Ambient Filesystem & Heuristics Daemon",
			Command:       []string{"ambient", "daemon"},
			DesiredState:  DesiredStateEnabled,
			RestartPolicy: RestartPolicyAlways,
			MaxRestarts:   10,
			BackoffMin:    500 * time.Millisecond,
			BackoffMax:    30 * time.Second,
		},
		{
			Name:          "privileged-writer",
			Description:   "ZQK Privileged CAS Storage Writer Daemon",
			Command:       []string{"object", "daemon"},
			DesiredState:  DesiredStateEnabled,
			RestartPolicy: RestartPolicyAlways,
			MaxRestarts:   10,
			BackoffMin:    500 * time.Millisecond,
			BackoffMax:    30 * time.Second,
			Env: map[string]string{
				"ZQK_IS_DAEMON": "1",
				"ZQK_API_KEY":   "ACC-SYSTEM",
			},
		},
		{
			Name:          "fswatcher",
			Description:   "ZQK Filesystem Object Watcher Daemon",
			Command:       []string{"ambient", "daemon"},
			DesiredState:  DesiredStateDisabled,
			RestartPolicy: RestartPolicyOnFailure,
			MaxRestarts:   5,
			BackoffMin:    1 * time.Second,
			BackoffMax:    30 * time.Second,
		},
		{
			Name:          "steward",
			Description:   "ZQK Kernel Health & Intake Clustering Steward",
			Command:       []string{"kernel", "steward", "daemon"},
			DesiredState:  DesiredStateEnabled,
			RestartPolicy: RestartPolicyAlways,
			MaxRestarts:   10,
			BackoffMin:    1 * time.Second,
			BackoffMax:    60 * time.Second,
		},
		{
			Name:          "mcp",
			Description:   "Model Context Protocol & Semantic Bridge Daemon",
			Command:       []string{"mcp", "daemon"},
			DesiredState:  DesiredStateDisabled,
			RestartPolicy: RestartPolicyOnFailure,
			MaxRestarts:   5,
			BackoffMin:    1 * time.Second,
			BackoffMax:    30 * time.Second,
		},
		{
			Name:          "seat-worker",
			Description:   "ZQK Autonomous Mesh Agent Seat Worker",
			Command:       []string{"agent", "seat-worker", "--agent-id", "peer-agent-1"},
			DesiredState:  DesiredStateDisabled,
			RestartPolicy: RestartPolicyOnFailure,
			MaxRestarts:   5,
			BackoffMin:    1 * time.Second,
			BackoffMax:    30 * time.Second,
		},
	}
}

// Registry manages the persistence of daemon configurations.
type Registry struct {
	mu       sync.RWMutex
	filePath string
	Daemons  map[string]*DaemonSpec `json:"daemons"`
}

// DefaultRegistryPath returns the canonical path to the overseer registry file.
func DefaultRegistryPath(projectRoot string) string {
	return filepath.Join(projectRoot, paths.ProjectDataDir, paths.StateDir, "daemon", "registry.json")
}

func projectRootFromRegistryPath(registryPath string) string {
	dir := filepath.Clean(registryPath)
	for i := 0; i < 6; i++ {
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		if fileutil.Exists(filepath.Join(dir, paths.ProjectDataDir)) {
			return dir
		}
		dir = parent
	}
	return paths.ResolveProjectRoot(filepath.Dir(registryPath))
}

// NewRegistry initializes a registry pointing to filePath.
func NewRegistry(filePath string) *Registry {
	return &Registry{
		filePath: filePath,
		Daemons:  make(map[string]*DaemonSpec),
	}
}

// Load reads the registry from disk, seeding defaults if the file does not exist.
func (r *Registry) Load() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	changed := false
	data, err := fileutil.ReadFile(r.filePath)
	if err != nil {
		if !fileutil.IsNotExist(err) {
			return errfmt.Newf("read daemon registry %s", r.filePath).Wrap(err)
		}
		// Seed defaults
		r.Daemons = make(map[string]*DaemonSpec)
		for _, spec := range DefaultDaemonSpecs() {
			r.Daemons[spec.Name] = spec
		}
		changed = true
	} else {
		var stored struct {
			Daemons map[string]*DaemonSpec `json:"daemons"`
		}
		if err := json.Unmarshal(data, &stored); err != nil {
			return errfmt.Newf("unmarshal daemon registry %s", r.filePath).Wrap(err)
		}

		r.Daemons = stored.Daemons
		if r.Daemons == nil {
			r.Daemons = make(map[string]*DaemonSpec)
		}

		// Seed any newly added default daemon specs not present in the stored registry
		for _, spec := range DefaultDaemonSpecs() {
			if _, exists := r.Daemons[spec.Name]; !exists {
				r.Daemons[spec.Name] = spec
				changed = true
			}
		}
	}

	// Dynamically seed seat workers declared in peer_seats.json
	projectRoot := projectRootFromRegistryPath(r.filePath)
	if projectRoot != "" {
		if f, err := agentfeed.LoadPeerSeats(projectRoot); err == nil && len(f.Seats) > 0 {
			seatIDs := make([]string, 0, len(f.Seats))
			for id := range f.Seats {
				seatIDs = append(seatIDs, id)
			}
			sort.Strings(seatIDs)
			for _, seatID := range seatIDs {
				workerName := fmt.Sprintf("seat-worker-%s", seatID)
				if _, exists := r.Daemons[workerName]; !exists {
					r.Daemons[workerName] = &DaemonSpec{
						Name:          workerName,
						Description:   fmt.Sprintf("ZQK Autonomous Mesh Agent Seat Worker (%s)", seatID),
						Command:       []string{"agent", "seat-worker", "--agent-id", seatID},
						DesiredState:  DesiredStateDisabled,
						RestartPolicy: RestartPolicyOnFailure,
						MaxRestarts:   5,
						BackoffMin:    1 * time.Second,
						BackoffMax:    30 * time.Second,
					}
					changed = true
				}
			}
		}
	}

	if changed {
		return r.saveLocked()
	}
	return nil
}

// Save writes the current registry to disk atomically.
func (r *Registry) Save() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.saveLocked()
}

func (r *Registry) saveLocked() error {
	dir := filepath.Dir(r.filePath)
	if err := fileutil.MkdirAll(dir, paths.DirPerm755); err != nil {
		return errfmt.Newf("mkdir daemon registry dir %s", dir).Wrap(err)
	}

	wrapper := struct {
		Daemons map[string]*DaemonSpec `json:"daemons"`
	}{
		Daemons: r.Daemons,
	}

	data, err := json.MarshalIndent(wrapper, "", "  ")
	if err != nil {
		return errfmt.Newf("marshal daemon registry").Wrap(err)
	}

	tmpFile := r.filePath + ".tmp"
	if err := fileutil.WriteFile(tmpFile, data, paths.FilePerm644); err != nil {
		return errfmt.Newf("write temp registry %s", tmpFile).Wrap(err)
	}
	if err := fileutil.Rename(tmpFile, r.filePath); err != nil {
		return errfmt.Newf("atomic rename %s -> %s", tmpFile, r.filePath).Wrap(err)
	}
	return nil
}

// Get returns the spec for a daemon by name.
func (r *Registry) Get(name string) (*DaemonSpec, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	spec, ok := r.Daemons[name]
	if !ok {
		return nil, false
	}
	// Return copy
	specCopy := *spec
	return &specCopy, true
}

// Set adds or updates a daemon specification.
func (r *Registry) Set(spec *DaemonSpec) error {
	if spec == nil || spec.Name == "" {
		return errfmt.Errorf("daemon spec name cannot be empty")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Daemons[spec.Name] = spec
	return r.saveLocked()
}

// SetDesiredState updates the desired state of a daemon.
func (r *Registry) SetDesiredState(name string, state DesiredState) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	spec, ok := r.Daemons[name]
	if !ok {
		return errfmt.Errorf("daemon %q not found in registry", name)
	}
	spec.DesiredState = state
	return r.saveLocked()
}

// List returns a slice of all registered daemon specifications.
func (r *Registry) List() []*DaemonSpec {
	r.mu.RLock()
	defer r.mu.RUnlock()
	specs := make([]*DaemonSpec, 0, len(r.Daemons))
	for _, spec := range r.Daemons {
		specCopy := *spec
		specs = append(specs, &specCopy)
	}
	return specs
}

// Delete removes a daemon specification from the registry.
func (r *Registry) Delete(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.Daemons[name]; !ok {
		return errfmt.Errorf("daemon %q not found in registry", name)
	}
	delete(r.Daemons, name)
	return r.saveLocked()
}
