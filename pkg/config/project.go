package config

import (
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

// GetErrorLogOutput returns the project's logging.error_log_output value ("separate" or "combined").
// Reads from .zqk/config/config.yaml or .zqk/config.yaml. Returns "" when unset or invalid (caller treats as "combined").
func GetErrorLogOutput(projectRoot string) string {
	if projectRoot == emptyPath {
		return emptyPath
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
// Reads from .zqk/config/config.yaml or .zqk/config.yaml. Returns "" when unset or invalid (caller uses default, e.g. InfoLevel).
func GetLogLevel(projectRoot string) string {
	if projectRoot == emptyPath {
		return emptyPath
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
