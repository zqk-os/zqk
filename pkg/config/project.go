package config

import (
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/stampmemo"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// legacyProjectYAML is keyed by project root (closed set). Stamp is the legacy config file.
var legacyProjectYAML stampmemo.Table[map[string]any]

func legacyProjectConfig(projectRoot string) map[string]any {
	files := []string{
		filepath.Join(projectRoot, paths.ProjectDataDir, paths.ConfigDir, paths.ProjectConfigFile),
		filepath.Join(projectRoot, paths.ProjectDataDir, paths.ProjectConfigFile),
	}
	cfg, _ := legacyProjectYAML.Load(projectRoot, stampmemo.OfAll(files...), func() (map[string]any, error) {
		for _, p := range files {
			data, err := fileutil.ReadFile(p)
			if err != nil {
				continue
			}
			var parsed map[string]any
			if yaml.Unmarshal(data, &parsed) != nil {
				continue
			}
			return parsed, nil
		}
		return nil, nil
	})
	return cfg
}

func loggingMap(cfg map[string]any) map[string]any {
	if cfg == nil {
		return nil
	}
	m, _ := cfg["logging"].(map[string]any)
	return m
}

// GetErrorLogOutput returns the project's logging.error_log_output value ("separate" or "combined").
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
	legacy := legacyProjectConfig(projectRoot)
	if v := errorLogOutputFromMap(loggingMap(legacy)); v != emptyPath {
		return v
	}
	return errorLogOutputFromMap(legacy)
}

func errorLogOutputFromMap(m map[string]any) string {
	s, _ := m["error_log_output"].(string)
	if s == "separate" || s == "combined" {
		return s
	}
	return emptyPath
}

// GetLogLevel returns the project's logging.level value.
func GetLogLevel(projectRoot string) string {
	if projectRoot == emptyPath {
		return emptyPath
	}
	if cfg := LoadForRoot(projectRoot); cfg != nil && cfg.Logging.Level != nil && *cfg.Logging.Level != "" {
		return *cfg.Logging.Level
	}
	if s, _ := loggingMap(legacyProjectConfig(projectRoot))["level"].(string); s != emptyPath {
		return s
	}
	return emptyPath
}
