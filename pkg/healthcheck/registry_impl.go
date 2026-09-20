package healthcheck

import (
	"context"
	"encoding/json"
	"path/filepath"
	"sync"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

const (
	configFileName     = "health_monitors.json"
	statusSkip         = "skip"
	statusOK           = "ok"
	statusFail         = "fail"
	statusDegraded     = "degraded"
	summaryMonitorOff  = "monitor disabled"
	summaryNoMonitors  = "no enabled monitors"
	summaryNilResult   = "monitor returned nil result"
	summarySeparator   = "; "
	summaryErrorSuffix = ": error: "
	summaryIDSeparator = ": "
	detailsErrorKey    = "error"
	configDirPerm      = paths.DirPerm755
	configFilePerm     = paths.FilePerm600
	jsonIndentPrefix   = ""
	jsonIndentValue    = "  "
	defaultMonitorOn   = true
	emptyValue         = ""
)

// DefaultRegistry is the global registry used by healthchk and by components that run monitors.
var DefaultRegistry Registry = NewRegistry("")

// NewRegistry returns a registry that persists enabled state to projectRoot/.zqk/config/health_monitors.json.
// If projectRoot is empty, config is in-memory only (for list without a project).
func NewRegistry(projectRoot string) *DefaultRegistryImpl {
	r := &DefaultRegistryImpl{
		projectRoot: projectRoot,
		monitors:    make(map[string]Monitor),
		enabled:     make(map[string]bool),
	}
	if projectRoot != emptyValue {
		_ = r.loadConfig()
	}
	return r
}

// BindProjectRoot sets the project root and reloads config from disk. Use from CLI when project root is resolved.
func (r *DefaultRegistryImpl) BindProjectRoot(projectRoot string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.projectRoot = projectRoot
	return r.loadConfig()
}

// DefaultRegistryImpl is the default in-memory registry with optional file-backed enabled state.
type DefaultRegistryImpl struct {
	mu          sync.RWMutex
	projectRoot string
	monitors    map[string]Monitor
	enabled     map[string]bool
}

func (r *DefaultRegistryImpl) Register(m Monitor) {
	if m == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	id := m.ID()
	r.monitors[id] = m
	if _, set := r.enabled[id]; !set {
		r.enabled[id] = defaultMonitorOn // default enabled
	}
}

func (r *DefaultRegistryImpl) List() []Monitor {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Monitor, 0, len(r.monitors))
	for _, m := range r.monitors {
		out = append(out, m)
	}
	return out
}

func (r *DefaultRegistryImpl) Get(id string) (Monitor, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	m, ok := r.monitors[id]
	return m, ok
}

func (r *DefaultRegistryImpl) IsEnabled(id string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if e, ok := r.enabled[id]; ok {
		return e
	}
	return defaultMonitorOn // default enabled when not in config
}

func (r *DefaultRegistryImpl) SetEnabled(id string, enabled bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.monitors[id]; !ok {
		return fileutil.ErrNotExist
	}
	r.enabled[id] = enabled
	return r.saveConfig()
}

func (r *DefaultRegistryImpl) Run(ctx context.Context, projectRoot string, id string) (*Result, error) {
	if projectRoot == emptyValue {
		projectRoot = r.projectRoot
	}
	r.mu.RLock()
	var m Monitor
	var ok bool
	if id != emptyValue {
		m, ok = r.monitors[id]
		if !ok {
			r.mu.RUnlock()
			return nil, fileutil.ErrNotExist
		}
		if !r.enabled[m.ID()] {
			r.mu.RUnlock()
			return &Result{Status: statusSkip, Summary: summaryMonitorOff}, nil
		}
		r.mu.RUnlock()
		return m.Run(ctx, projectRoot)
	}
	// Run all enabled and combine worst status
	list := make([]Monitor, 0, len(r.monitors))
	for _, mon := range r.monitors {
		if r.enabled[mon.ID()] {
			list = append(list, mon)
		}
	}
	r.mu.RUnlock()
	if len(list) == 0 {
		return &Result{Status: statusSkip, Summary: summaryNoMonitors}, nil
	}
	var worst string
	var summaries []string
	details := make(map[string]any)
	for _, m := range list {
		res, err := m.Run(ctx, projectRoot)
		if err != nil {
			summaries = append(summaries, m.ID()+summaryErrorSuffix+err.Error())
			details[m.ID()] = map[string]any{detailsErrorKey: err.Error()}
			worst = statusFail
			continue
		}
		if res == nil {
			// TRACK: BLI-CEF-R2-OBS-HEALTHCHECK-FALSEGREEN — nil Result is not ok.
			summaries = append(summaries, m.ID()+summaryIDSeparator+summaryNilResult)
			details[m.ID()] = map[string]any{detailsErrorKey: summaryNilResult}
			worst = statusFail
			continue
		}
		summaries = append(summaries, m.ID()+summaryIDSeparator+res.Summary)
		details[m.ID()] = res
		switch res.Status {
		case statusFail:
			worst = statusFail
		case statusDegraded:
			if worst != statusFail {
				worst = statusDegraded
			}
		case statusOK:
			if worst == emptyValue {
				worst = statusOK
			}
		default:
			// Unknown or empty status is not a pass.
			if worst != statusFail {
				worst = statusDegraded
			}
		}
	}
	if worst == emptyValue {
		worst = statusFail
	}
	summary := emptyValue
	for i, s := range summaries {
		if i > 0 {
			summary += summarySeparator
		}
		summary += s
	}
	return &Result{Status: worst, Summary: summary, Details: details}, nil
}

func (r *DefaultRegistryImpl) configPath() string {
	return filepath.Join(r.projectRoot, paths.ProjectDataDir, paths.ConfigDir, configFileName)
}

func (r *DefaultRegistryImpl) loadConfig() error {
	path := r.configPath()
	data, err := fileutil.ReadFile(path)
	if err != nil {
		if fileutil.IsNotExist(err) {
			return nil
		}
		return err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return err
	}
	for id, c := range cfg.Monitors {
		r.enabled[id] = c.Enabled
	}
	return nil
}

func (r *DefaultRegistryImpl) saveConfig() error {
	if r.projectRoot == emptyValue {
		return nil
	}
	path := r.configPath()
	dir := filepath.Dir(path)
	if err := fileutil.MkdirAll(dir, configDirPerm); err != nil {
		return err
	}
	cfg := Config{Monitors: make(map[string]MonitorConfig)}
	for id, e := range r.enabled {
		cfg.Monitors[id] = MonitorConfig{Enabled: e}
	}
	data, err := json.MarshalIndent(cfg, jsonIndentPrefix, jsonIndentValue)
	if err != nil {
		return err
	}
	return fileutil.WriteFile(path, data, configFilePerm)
}
