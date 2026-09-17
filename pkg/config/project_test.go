package config

import (
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

func TestGetErrorLogOutput(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	configDir := filepath.Join(dir, paths.ProjectDataDir, paths.ConfigDir)
	if err := fileutil.MkdirAll(configDir, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}

	t.Run("missing config returns empty", func(t *testing.T) {
		if got := GetErrorLogOutput(dir); got != emptyPath {
			t.Errorf("GetErrorLogOutput(missing) = %q, want \"\"", got)
		}
	})

	t.Run("logging.error_log_output separate", func(t *testing.T) {
		cfgPath := filepath.Join(configDir, paths.ProjectConfigFile)
		cfg := map[string]any{"logging": map[string]any{"error_log_output": "separate"}}
		data, _ := yaml.Marshal(cfg)
		if err := fileutil.WriteFile(cfgPath, data, paths.FilePerm644); err != nil {
			t.Fatal(err)
		}
		if got := GetErrorLogOutput(dir); got != "separate" {
			t.Errorf("GetErrorLogOutput() = %q, want \"separate\"", got)
		}
	})

	t.Run("logging.error_log_output combined", func(t *testing.T) {
		cfgPath := filepath.Join(configDir, paths.ProjectConfigFile)
		cfg := map[string]any{"logging": map[string]any{"error_log_output": "combined"}}
		data, _ := yaml.Marshal(cfg)
		if err := fileutil.WriteFile(cfgPath, data, paths.FilePerm644); err != nil {
			t.Fatal(err)
		}
		if got := GetErrorLogOutput(dir); got != "combined" {
			t.Errorf("GetErrorLogOutput() = %q, want \"combined\"", got)
		}
	})

	t.Run("empty project root returns empty", func(t *testing.T) {
		if got := GetErrorLogOutput(emptyPath); got != emptyPath {
			t.Errorf("GetErrorLogOutput(\"\") = %q, want \"\"", got)
		}
	})
}
