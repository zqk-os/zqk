package config

import (
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// GetErrorLogOutput returns the project's logging.error_log_output value ("separate" or "combined").
// Reads from config/** (with env/local overrides) or legacy locations. Returns "" when unset or invalid (caller treats as "combined").
func GetErrorLogOutput(projectRoot string) string {
	if projectRoot == emptyPath {
		return emptyPath
	}
	if cfg := LoadForRoot(projectRoot); cfg != nil && cfg.Logging.ErrorLogOutput != nil && *cfg.Logging.ErrorLogOutput != "" {
		v := *cfg.Logging.ErrorLogOutput
		if v == "separate" || v == "combined" {
			return v
		}
	}
	for _, rel := range []string{
		filepath.Join(paths.ProjectDataDir, paths.ConfigDir, paths.ProjectConfigFile),
		filepath.Join(paths.ProjectDataDir, paths.ProjectConfigFile),
	} {
		p := filepath.Join(projectRoot, rel)
		data, err := fileutil.ReadFile(p)
		if err != nil {
			if fileutil.IsNotExist(err) {
				continue
			}
			return emptyPath
		}
		var cfg map[string]any
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			continue
		}
		if m, ok := cfg["logging"].(map[string]any); ok {
			if v := errorLogOutputFromMap(m); v != emptyPath {
				return v
			}
		}
		if v := errorLogOutputFromMap(cfg); v != emptyPath {
			return v
		}
	}
	return emptyPath
}

func errorLogOutputFromMap(m map[string]any) string {
	s, _ := m["error_log_output"].(string)
	if s == "separate" || s == "combined" {
		return s
	}
	return emptyPath
}

// GetLogLevel returns the project's logging.level value ("debug", "info", "warn", "error", "fatal").
// Reads from config/** (with env/local overrides) or legacy locations. Returns "" when unset or invalid (caller uses default, e.g. InfoLevel).
func GetLogLevel(projectRoot string) string {
	if projectRoot == emptyPath {
		return emptyPath
	}
	if cfg := LoadForRoot(projectRoot); cfg != nil && cfg.Logging.Level != nil && *cfg.Logging.Level != "" {
		return *cfg.Logging.Level
	}
	for _, rel := range []string{
		filepath.Join(paths.ProjectDataDir, paths.ConfigDir, paths.ProjectConfigFile),
		filepath.Join(paths.ProjectDataDir, paths.ProjectConfigFile),
	} {
		p := filepath.Join(projectRoot, rel)
		data, err := fileutil.ReadFile(p)
		if err != nil {
			if fileutil.IsNotExist(err) {
				continue
			}
			return emptyPath
		}
		var cfg map[string]any
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			continue
		}
		if m, ok := cfg["logging"].(map[string]any); ok {
			if s, _ := m["level"].(string); s != emptyPath {
				return s
			}
		}
	}
	return emptyPath
}
